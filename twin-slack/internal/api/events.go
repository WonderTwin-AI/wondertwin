package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
	"github.com/wondertwin-ai/wondertwin/twinkit/twincore"
	"github.com/wondertwin-ai/wondertwin/twinkit/webhook"
)

// Events API HTTP delivery, as apis/events-api.md describes it: a write the
// Web API reports as an event is POSTed to the app's Request URL as an
// event_callback, signed with the signing secret, and retried on failure.
const (
	// eventAckTimeout is how long Slack waits for a 2xx before it counts the
	// attempt as failed (http_timeout).
	eventAckTimeout = 3 * time.Second

	// DefaultSigningSecret is the signing secret when none is configured.
	DefaultSigningSecret = "slack-emulator-signing-secret"
	// DefaultVerificationToken fills the deprecated token field.
	DefaultVerificationToken = "slack-emulator-verification-token"
)

// defaultRetryDelays is Slack's schedule: the first retry nearly at once, the
// second after a minute, the third after five.
var defaultRetryDelays = []time.Duration{0, time.Minute, 5 * time.Minute}

// EventsConfig is the app's Event Subscriptions settings. Slack sets these in
// the app's settings, not through the Web API, so the app emulator takes them
// at startup and from POST /admin/events/config.
type EventsConfig struct {
	RequestURL        string
	SigningSecret     string
	VerificationToken string
	// RetryDelays are the waits before each retry. There is one retry per
	// entry, so the default three entries give four attempts.
	RetryDelays []time.Duration
}

// EventDelivery records one delivery attempt.
type EventDelivery struct {
	EventID     string `json:"event_id"`
	EventType   string `json:"event_type"`
	URL         string `json:"url"`
	RetryNum    int    `json:"retry_num"`
	RetryReason string `json:"retry_reason,omitempty"`
	StatusCode  int    `json:"status_code"`
	Error       string `json:"error,omitempty"`
}

// slackSigner signs a body the way Slack signs requests to an app:
// X-Slack-Signature is v0= and the hex HMAC-SHA256 of v0:{timestamp}:{body}
// keyed with the signing secret. The timestamp is the wall clock, because
// receivers reject one more than five minutes from their own.
type slackSigner struct{}

func (slackSigner) Sign(payload []byte, secret string) map[string]string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	return map[string]string{
		"X-Slack-Request-Timestamp": ts,
		"X-Slack-Signature":         slackSignature(secret, ts, payload),
	}
}

func slackSignature(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// attempt is what the dispatcher callbacks need for the attempt in flight.
type attempt struct {
	body        []byte
	retryNum    int
	retryReason string
}

type eventJob struct {
	id, typ string
	body    []byte
	gen     int
}

// eventBus delivers events in order, one at a time, through a twinkit
// dispatcher. The dispatcher makes each attempt; the bus owns Slack's retry
// schedule, because each retry carries its own x-slack-retry-num and
// x-slack-retry-reason.
type eventBus struct {
	mu         sync.Mutex
	cfg        EventsConfig
	d          *webhook.Dispatcher
	inflight   map[string]attempt // dispatcher event id -> attempt
	deliveries []EventDelivery
	counter    int
	// gen counts resets. A job queued before a reset is dropped rather than
	// delivered into the next test's log, and resetCh, closed on each reset,
	// cuts short a retry wait so it does not hold up later events.
	gen     int
	resetCh chan struct{}
	jobs    chan eventJob
	pending sync.WaitGroup
}

func newEventBus() *eventBus {
	b := &eventBus{
		cfg: EventsConfig{
			SigningSecret:     DefaultSigningSecret,
			VerificationToken: DefaultVerificationToken,
			RetryDelays:       defaultRetryDelays,
		},
		inflight: map[string]attempt{},
		jobs:     make(chan eventJob, 1024),
		resetCh:  make(chan struct{}),
	}
	b.d = webhook.NewDispatcher(webhook.Config{
		Signer:      slackSigner{},
		MaxRetries:  1,
		Timeout:     eventAckTimeout,
		EventPrefix: "attempt",
		Encode:      b.encode,
		Headers:     b.headers,
	})
	go b.run()
	return b
}

func (b *eventBus) encode(evt webhook.Event) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inflight[evt.ID].body, nil
}

func (b *eventBus) headers(evt webhook.Event) map[string]string {
	b.mu.Lock()
	a := b.inflight[evt.ID]
	b.mu.Unlock()
	if a.retryNum == 0 {
		return nil
	}
	return map[string]string{
		"X-Slack-Retry-Num":    strconv.Itoa(a.retryNum),
		"X-Slack-Retry-Reason": a.retryReason,
	}
}

func (b *eventBus) config() EventsConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cfg
}

// configure applies settings and points the dispatcher at them.
func (b *eventBus) configure(cfg EventsConfig) {
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
	b.d.SetURL(cfg.RequestURL)
	b.d.SetSecret(cfg.SigningSecret)
}

// publish queues one event for delivery. With no Request URL configured,
// the dispatcher sends nothing, as Slack sends nothing to an app without
// Event Subscriptions.
func (b *eventBus) publish(h *Handler, event map[string]any) {
	cfg := b.config()
	b.mu.Lock()
	b.counter++
	n, gen := b.counter, b.gen
	b.mu.Unlock()

	team := h.store.Team.ID
	id := fmt.Sprintf("Ev%08d", n)
	envelope := map[string]any{
		"token":      cfg.VerificationToken,
		"team_id":    team,
		"api_app_id": appID,
		"event":      event,
		"type":       "event_callback",
		"event_id":   id,
		"event_time": h.store.Clock.Now().Unix(),
		"authorizations": []map[string]any{{
			"enterprise_id":         nil,
			"team_id":               team,
			"user_id":               store.DefaultBotUserID,
			"is_bot":                true,
			"is_enterprise_install": false,
		}},
		"event_context":         fmt.Sprintf("EC%08d", n),
		"is_ext_shared_channel": false,
		"context_team_id":       team,
		"context_enterprise_id": nil,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	b.pending.Add(1)
	b.jobs <- eventJob{id: id, typ: fmt.Sprint(event["type"]), body: body, gen: gen}
}

func (b *eventBus) run() {
	for job := range b.jobs {
		b.deliver(job)
		b.pending.Done()
	}
}

// deliver makes the first attempt and Slack's retries, until one succeeds.
func (b *eventBus) deliver(job eventJob) {
	delays := b.config().RetryDelays
	reason := ""
	for retry := 0; retry <= len(delays); retry++ {
		if retry > 0 && !b.wait(job, delays[retry-1]) {
			return
		}
		if !b.current(job) {
			return
		}
		rec, ok := b.attempt(job, retry, reason)
		if ok {
			return
		}
		reason = retryReason(rec)
	}
}

// attempt makes one delivery through the dispatcher and records it.
func (b *eventBus) attempt(job eventJob, retry int, reason string) (EventDelivery, bool) {
	evt := b.d.Enqueue(job.typ, nil)
	b.mu.Lock()
	b.inflight[evt.ID] = attempt{body: job.body, retryNum: retry, retryReason: reason}
	b.mu.Unlock()

	err := b.d.Flush()

	rec := EventDelivery{EventID: job.id, EventType: job.typ, URL: b.config().RequestURL, RetryNum: retry}
	if retry > 0 {
		rec.RetryReason = reason
	}
	sent, noRetry := false, false
	for _, d := range b.d.Deliveries() {
		if d.EventID == evt.ID {
			rec.StatusCode, rec.Error, sent = d.StatusCode, d.Error, true
			noRetry = d.Header.Get("X-Slack-No-Retry") == "1"
		}
	}
	b.mu.Lock()
	delete(b.inflight, evt.ID)
	if sent && job.gen == b.gen {
		b.deliveries = append(b.deliveries, rec)
	}
	b.mu.Unlock()
	// An attempt the dispatcher did not send (no Request URL is configured)
	// ends the delivery rather than waiting out the retry schedule, and so
	// does a failure the receiver answered with x-slack-no-retry: 1, which
	// asks Slack not to redeliver this event.
	return rec, !sent || noRetry || (err == nil && rec.StatusCode >= 200 && rec.StatusCode < 300)
}

// wait sleeps before a retry. It returns false if a reset came first.
func (b *eventBus) wait(job eventJob, d time.Duration) bool {
	b.mu.Lock()
	ch := b.resetCh
	stale := job.gen != b.gen
	b.mu.Unlock()
	if stale {
		return false
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ch:
		return false
	}
}

// current reports whether a job was queued since the last reset.
func (b *eventBus) current(job eventJob) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return job.gen == b.gen
}

// reset forgets every event and delivery, and the event counter, as
// POST /admin/reset does for the rest of the app emulator. The Event
// Subscriptions settings stay: they are the app's configuration, set at
// startup or through /admin/events/config, not workspace state, which is also
// why neither they nor the delivery log is part of a state snapshot.
func (b *eventBus) reset() {
	b.mu.Lock()
	b.gen++
	b.counter = 0
	b.deliveries = nil
	close(b.resetCh)
	b.resetCh = make(chan struct{})
	b.mu.Unlock()
	b.d.Reset()
}

// retryReason names a failed attempt with the x-slack-retry-reason value the
// docs give for it.
func retryReason(d EventDelivery) string {
	switch {
	case d.StatusCode != 0:
		return "http_error"
	case strings.Contains(d.Error, "Timeout") || strings.Contains(d.Error, "deadline exceeded"):
		return "http_timeout"
	case d.Error != "":
		return "connection_failed"
	}
	return "unknown_error"
}

// FlushWebhooks waits until every queued event has been delivered or has run
// out of retries. It implements the admin handler's webhook flusher.
func (b *eventBus) FlushWebhooks() error {
	b.pending.Wait()
	return nil
}

func (b *eventBus) log() []EventDelivery {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]EventDelivery{}, b.deliveries...)
}

// ConfigureEvents sets the app's Event Subscriptions settings.
func (h *Handler) ConfigureEvents(cfg EventsConfig) {
	h.events.configure(withEventDefaults(cfg))
}

// withEventDefaults fills the settings a caller left empty.
func withEventDefaults(cfg EventsConfig) EventsConfig {
	if cfg.SigningSecret == "" {
		cfg.SigningSecret = DefaultSigningSecret
	}
	if cfg.VerificationToken == "" {
		cfg.VerificationToken = DefaultVerificationToken
	}
	if cfg.RetryDelays == nil {
		cfg.RetryDelays = defaultRetryDelays
	}
	return cfg
}

// adminState is the app emulator's state as POST /admin/reset, GET and
// POST /admin/state see it: the store, plus the event bus on reset.
type adminState struct {
	*store.MemoryStore
	events *eventBus
}

func (s adminState) Reset() {
	s.MemoryStore.Reset()
	s.events.reset()
}

// AdminState is the state the admin handler manages. Resetting it also
// clears the Events API delivery log and drops queued events.
func (h *Handler) AdminState() interface {
	Snapshot() any
	LoadState(data []byte) error
	Reset()
} {
	return adminState{MemoryStore: h.store, events: h.events}
}

// EventsFlusher is what the admin handler's /admin/webhooks/flush waits on.
func (h *Handler) EventsFlusher() interface{ FlushWebhooks() error } { return h.events }

// channelType names a conversation the way message events do.
func channelType(ch store.Channel) string {
	switch {
	case ch.IsIM:
		return "im"
	case ch.IsMPIM:
		return "mpim"
	case ch.IsPrivate:
		return "group"
	}
	return "channel"
}

// emitMessage reports a posted message as a message event.
func (h *Handler) emitMessage(msg store.Message) {
	ch, _ := h.store.Channels.Get(msg.Channel)
	event := map[string]any{
		"type":         "message",
		"channel":      msg.Channel,
		"user":         msg.User,
		"text":         msg.Text,
		"ts":           msg.TS,
		"event_ts":     msg.TS,
		"channel_type": channelType(ch),
		"team":         msg.Team,
	}
	if msg.ThreadTS != "" {
		event["thread_ts"] = msg.ThreadTS
	}
	if msg.BotID != "" {
		event["bot_id"] = msg.BotID
	}
	if msg.AppID != "" {
		event["app_id"] = msg.AppID
	}
	if msg.BotProfile != nil {
		event["bot_profile"] = msg.BotProfile
	}
	if msg.Blocks != nil {
		event["blocks"] = msg.Blocks
	}
	h.events.publish(h, event)
}

// emitMessageDeleted reports a deleted message, in the hidden message_deleted
// shape the message event docs show.
func (h *Handler) emitMessageDeleted(channel, deletedTS string) {
	ch, _ := h.store.Channels.Get(channel)
	ts := h.store.NextTS()
	h.events.publish(h, map[string]any{
		"type":         "message",
		"subtype":      "message_deleted",
		"hidden":       true,
		"channel":      channel,
		"ts":           ts,
		"deleted_ts":   deletedTS,
		"event_ts":     ts,
		"channel_type": channelType(ch),
	})
}

// emitReactionAdded reports a reaction, in the shape the Events API docs show.
func (h *Handler) emitReactionAdded(user, reaction string, msg store.Message) {
	ch, _ := h.store.Channels.Get(msg.Channel)
	h.events.publish(h, map[string]any{
		"type":     "reaction_added",
		"user":     user,
		"reaction": reaction,
		"item": map[string]any{
			"type":         "message",
			"channel":      msg.Channel,
			"ts":           msg.TS,
			"channel_type": channelType(ch),
		},
		"item_user": msg.User,
		"event_ts":  h.store.NextTS(),
	})
}

// memberChannelType names a conversation for member_joined_channel and
// member_left_channel: C for a public channel, G for a private one. This is
// unverified, since Slack's reference page for those events is not in the
// captured docs (see the Events entry in divergences.json).
func memberChannelType(ch store.Channel) string {
	if ch.IsPrivate || ch.IsMPIM {
		return "G"
	}
	return "C"
}

// emitMemberJoined reports a member joining a channel. inviter is the user
// who invited them, and is left out when they joined on their own.
func (h *Handler) emitMemberJoined(ch store.Channel, user, inviter string) {
	event := map[string]any{
		"type":         "member_joined_channel",
		"user":         user,
		"channel":      ch.ID,
		"channel_type": memberChannelType(ch),
		"team":         h.store.Team.ID,
		"event_ts":     h.store.NextTS(),
	}
	if inviter != "" {
		event["inviter"] = inviter
	}
	h.events.publish(h, event)
}

// emitMemberLeft reports a member leaving, or being removed from, a channel.
func (h *Handler) emitMemberLeft(ch store.Channel, user string) {
	h.events.publish(h, map[string]any{
		"type":         "member_left_channel",
		"user":         user,
		"channel":      ch.ID,
		"channel_type": memberChannelType(ch),
		"team":         h.store.Team.ID,
		"event_ts":     h.store.NextTS(),
	})
}

// verifyRequestURL makes the url_verification handshake Slack makes when a
// Request URL is entered in the app's settings: a signed POST carrying a
// challenge, which the app must echo with HTTP 200 as plain text, as a
// challenge form field, or as JSON (reference/events/url_verification.md).
// Slack keeps a URL that fails out of the settings.
func verifyRequestURL(cfg EventsConfig) error {
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	challenge := hex.EncodeToString(nonce)
	body, _ := json.Marshal(map[string]string{
		"token":     cfg.VerificationToken,
		"challenge": challenge,
		"type":      "url_verification",
	})
	req, err := http.NewRequest(http.MethodPost, cfg.RequestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range (slackSigner{}).Sign(body, cfg.SigningSecret) {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: eventAckTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the Request URL answered HTTP %d", resp.StatusCode)
	}
	if echoed(answer) != challenge {
		return fmt.Errorf("the Request URL did not echo the challenge")
	}
	return nil
}

// echoed reads the challenge back from a url_verification answer in any of
// the three forms the docs accept.
func echoed(answer []byte) string {
	var j struct {
		Challenge string `json:"challenge"`
	}
	if json.Unmarshal(answer, &j) == nil && j.Challenge != "" {
		return j.Challenge
	}
	text := strings.TrimSpace(string(answer))
	if v, err := url.ParseQuery(text); err == nil && v.Get("challenge") != "" {
		return v.Get("challenge")
	}
	return text
}

// AdminEventsConfig handles GET and POST /admin/events/config. POST takes any
// of request_url, signing_secret, verification_token and retry_delays_ms, and
// leaves the others as they are. A non-empty request_url must pass Slack's
// url_verification handshake, or nothing changes. The --webhook-url flag sets
// the Request URL at startup without one, since the app may not be up yet.
func (h *Handler) AdminEventsConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.events.config()
	if r.Method == http.MethodPost {
		var req struct {
			RequestURL        *string `json:"request_url"`
			SigningSecret     *string `json:"signing_secret"`
			VerificationToken *string `json:"verification_token"`
			RetryDelaysMS     []int   `json:"retry_delays_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			twincore.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.RequestURL != nil {
			cfg.RequestURL = *req.RequestURL
		}
		if req.SigningSecret != nil {
			cfg.SigningSecret = *req.SigningSecret
		}
		if req.VerificationToken != nil {
			cfg.VerificationToken = *req.VerificationToken
		}
		if req.RetryDelaysMS != nil {
			delays := make([]time.Duration, len(req.RetryDelaysMS))
			for i, ms := range req.RetryDelaysMS {
				if ms < 0 {
					twincore.Error(w, http.StatusBadRequest, "retry_delays_ms must not be negative")
					return
				}
				delays[i] = time.Duration(ms) * time.Millisecond
			}
			cfg.RetryDelays = delays
		}
		// The handshake is signed with the settings later deliveries use.
		cfg = withEventDefaults(cfg)
		if req.RequestURL != nil && cfg.RequestURL != "" {
			if err := verifyRequestURL(cfg); err != nil {
				twincore.Error(w, http.StatusBadRequest, "url_verification failed: "+err.Error())
				return
			}
		}
		h.ConfigureEvents(cfg)
		cfg = h.events.config()
	}
	delays := make([]int64, len(cfg.RetryDelays))
	for i, d := range cfg.RetryDelays {
		delays[i] = d.Milliseconds()
	}
	twincore.JSON(w, http.StatusOK, map[string]any{
		"request_url":        cfg.RequestURL,
		"signing_secret":     cfg.SigningSecret,
		"verification_token": cfg.VerificationToken,
		"retry_delays_ms":    delays,
	})
}

// AdminEventDeliveries handles GET /admin/events/deliveries: every attempt,
// in order.
func (h *Handler) AdminEventDeliveries(w http.ResponseWriter, r *http.Request) {
	twincore.JSON(w, http.StatusOK, map[string]any{"deliveries": h.events.log()})
}
