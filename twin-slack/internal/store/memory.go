package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Slack twin state in memory.
type MemoryStore struct {
	Channels          *pkgstate.Store[Channel]
	Messages          *pkgstate.Store[Message]
	Users             *pkgstate.Store[User]
	Pins              *pkgstate.Store[Pin]
	Files             *pkgstate.Store[File]
	ScheduledMessages *pkgstate.Store[ScheduledMessage]
	Bookmarks         *pkgstate.Store[Bookmark]
	Usergroups        *pkgstate.Store[Usergroup]
	Tokens            *pkgstate.Store[Token]
	Views             *pkgstate.Store[ViewRecord]
	Presences         *pkgstate.Store[Presence]
	Bots              *pkgstate.Store[Bot]
	// Emoji are the workspace's custom emoji: a name maps to an image URL, or
	// to "alias:" and the name of another emoji.
	Emoji *pkgstate.Store[string]
	// ReadStates are each user's read cursor and open state per conversation.
	ReadStates *pkgstate.Store[ReadState]
	Clock      *pkgstate.Clock

	// Team info (singleton)
	Team Team

	tsCounter atomic.Int64
}

// New creates a new MemoryStore with empty state and default team.
func New() *MemoryStore {
	s := &MemoryStore{
		Channels:          pkgstate.New[Channel]("C"),
		Messages:          pkgstate.New[Message]("msg"),
		Users:             pkgstate.New[User]("U"),
		Pins:              pkgstate.New[Pin]("pin"),
		Files:             pkgstate.New[File]("F"),
		ScheduledMessages: pkgstate.New[ScheduledMessage]("Q"),
		Bookmarks:         pkgstate.New[Bookmark]("Bk"),
		Usergroups:        pkgstate.New[Usergroup]("S"),
		Tokens:            pkgstate.New[Token]("tok"),
		Views:             pkgstate.New[ViewRecord]("V"),
		Presences:         pkgstate.New[Presence]("P"),
		Bots:              pkgstate.New[Bot]("B"),
		Emoji:             pkgstate.New[string]("E"),
		ReadStates:        pkgstate.New[ReadState]("R"),
		Clock:             pkgstate.NewClock(),
		Team:              defaultTeam(),
	}
	s.ensureDefaultPrincipals()
	return s
}

// DefaultAppID is the app the default bot belongs to.
const DefaultAppID = "A_SIM"

func defaultTeam() Team {
	return Team{ID: "T0001", Name: "WonderTwin", Domain: "wondertwin", Icon: TeamIcon{ImageDefault: true}}
}

// ensureDefaultPrincipals records the users and bot that an unseeded token
// speaks for, so the API can look them up like any other: the bot user, the
// user a user token defaults to, and the bot's app identity. Seeded records
// with the same IDs are kept.
func (s *MemoryStore) ensureDefaultPrincipals() {
	if _, ok := s.Users.Get(DefaultBotUserID); !ok {
		s.Users.Set(DefaultBotUserID, User{
			ID: DefaultBotUserID, TeamID: s.Team.ID, Name: "bot", RealName: "bot", IsBot: true,
			Profile: UserProfile{DisplayName: "bot", DisplayNameNorm: "bot", RealName: "bot", RealNameNorm: "bot"},
		})
	}
	if _, ok := s.Users.Get(DefaultUserID); !ok {
		s.Users.Set(DefaultUserID, User{
			ID: DefaultUserID, TeamID: s.Team.ID, Name: "user", RealName: "user",
			Profile: UserProfile{DisplayName: "user", DisplayNameNorm: "user", RealName: "user", RealNameNorm: "user"},
		})
	}
	if _, ok := s.Bots.Get(DefaultBotID); !ok {
		s.Bots.Set(DefaultBotID, Bot{ID: DefaultBotID, Name: "bot", AppID: DefaultAppID, UserID: DefaultBotUserID})
	}
}

// BotProfileFor is the bot identity a message posted by the bot carries, or
// nil for a bot the workspace has no record of.
func (s *MemoryStore) BotProfileFor(botID string) *BotProfile {
	b, ok := s.Bots.Get(botID)
	if !ok {
		return nil
	}
	return &BotProfile{ID: b.ID, Deleted: b.Deleted, Name: b.Name, Updated: b.Updated, AppID: b.AppID, UserID: b.UserID, TeamID: s.Team.ID}
}

// NextTS generates a Slack-compatible message timestamp (unique, monotonically increasing).
func (s *MemoryStore) NextTS() string {
	n := s.tsCounter.Add(1)
	sec := s.Clock.Now().Unix()
	return fmt.Sprintf("%d.%06d", sec, n)
}

// GetMessageByTS looks up a message by channel and timestamp.
func (s *MemoryStore) GetMessageByTS(channel, ts string) (*Message, string, bool) {
	ids, msgs := s.Messages.FilterWithIDs(func(id string, msg Message) bool {
		return msg.Channel == channel && msg.TS == ts && !msg.IsDeleted
	})
	if len(ids) == 0 {
		return nil, "", false
	}
	return &msgs[0], ids[0], true
}

// GetChannelMessages returns messages in a channel, newest first.
func (s *MemoryStore) GetChannelMessages(channel string, limit int) []Message {
	all := s.Messages.List()
	result := []Message{}
	// Iterate in reverse (newest first)
	for i := len(all) - 1; i >= 0; i-- {
		msg := all[i]
		if msg.Channel == channel && !msg.IsDeleted && (msg.ThreadTS == "" || msg.Subtype == "thread_broadcast") {
			result = append(result, msg)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result
}

// GetThreadReplies returns messages in a thread.
func (s *MemoryStore) GetThreadReplies(channel, threadTS string, limit int) []Message {
	all := s.Messages.List()
	result := []Message{}
	for _, msg := range all {
		if msg.Channel == channel && !msg.IsDeleted &&
			(msg.TS == threadTS || msg.ThreadTS == threadTS) {
			result = append(result, msg)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result
}

// GetChannelByName looks up a channel by name.
func (s *MemoryStore) GetChannelByName(name string) (*Channel, string, bool) {
	ids, chs := s.Channels.FilterWithIDs(func(id string, ch Channel) bool {
		return ch.Name == name
	})
	if len(ids) == 0 {
		return nil, "", false
	}
	return &chs[0], ids[0], true
}

// GetUserByEmail looks up a user by email.
func (s *MemoryStore) GetUserByEmail(email string) (*User, bool) {
	if email == "" {
		return nil, false
	}
	for _, u := range s.Users.List() {
		if u.Profile.Email == email {
			return &u, true
		}
	}
	return nil, false
}

// KnownUser reports whether id is a user of the workspace: a stored user, or
// a user that a token, seeded or default, speaks for.
func (s *MemoryStore) KnownUser(id string) bool {
	if id == "" {
		return false
	}
	if _, ok := s.Users.Get(id); ok {
		return true
	}
	if id == DefaultBotUserID || id == DefaultUserID {
		return true
	}
	return len(s.Tokens.Filter(func(_ string, t Token) bool { return t.UserID == id })) > 0
}

type stateSnapshot struct {
	Channels          map[string]Channel          `json:"channels"`
	Messages          map[string]Message          `json:"messages"`
	Users             map[string]User             `json:"users"`
	Files             map[string]File             `json:"files"`
	ScheduledMessages map[string]ScheduledMessage `json:"scheduled_messages"`
	Tokens            map[string]Token            `json:"tokens"`
	Pins              map[string]Pin              `json:"pins"`
	Bookmarks         map[string]Bookmark         `json:"bookmarks"`
	Usergroups        map[string]Usergroup        `json:"usergroups"`
	Views             map[string]ViewRecord       `json:"views"`
	Presences         map[string]Presence         `json:"presences"`
	Bots              map[string]Bot              `json:"bots"`
	Emoji             map[string]string           `json:"emoji"`
	ReadStates        map[string]ReadState        `json:"read_states"`
	Team              *Team                       `json:"team,omitempty"`
	// Clock is the simulated clock's offset and pin, so a restored emulator
	// keeps the time it was moved to.
	Clock *clockState `json:"clock,omitempty"`
	// DeletedMessages names the messages chat.delete removed. A message's
	// deleted flag is not part of its Slack shape, so it travels here.
	DeletedMessages []string `json:"deleted_messages,omitempty"`
	// TSCounter is the last message timestamp sequence number, so a restored
	// emulator keeps issuing timestamps after the ones it already holds.
	TSCounter *int64 `json:"ts_counter,omitempty"`
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Channels:          s.Channels.Snapshot(),
		Messages:          s.Messages.Snapshot(),
		Users:             s.Users.Snapshot(),
		Files:             s.Files.Snapshot(),
		ScheduledMessages: s.ScheduledMessages.Snapshot(),
		Tokens:            s.Tokens.Snapshot(),
		Pins:              s.Pins.Snapshot(),
		Bookmarks:         s.Bookmarks.Snapshot(),
		Usergroups:        s.Usergroups.Snapshot(),
		Views:             s.Views.Snapshot(),
		Presences:         s.Presences.Snapshot(),
		Bots:              s.Bots.Snapshot(),
		Emoji:             s.Emoji.Snapshot(),
		ReadStates:        s.ReadStates.Snapshot(),
		Team:              &s.Team,
		Clock:             snapshotClock(s.Clock),
		DeletedMessages:   s.deletedMessages(),
		TSCounter:         ptr(s.tsCounter.Load()),
	}
}

// deletedMessages lists the keys of deleted messages, sorted.
func (s *MemoryStore) deletedMessages() []string {
	var keys []string
	for key, msg := range s.Messages.Snapshot() {
		if msg.IsDeleted {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// LoadState replaces state from a snapshot. Every store the snapshot names is
// replaced, even when it is empty, so loading a snapshot taken from the
// emulator restores it exactly. A store the snapshot leaves out is kept, which
// lets a seed file name only the stores it fills. The default principals are
// put back if the snapshot dropped them.
func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	for _, key := range snap.DeletedMessages {
		if msg, ok := snap.Messages[key]; ok {
			msg.IsDeleted = true
			snap.Messages[key] = msg
		}
	}
	load(s.Channels, snap.Channels)
	load(s.Messages, snap.Messages)
	load(s.Users, snap.Users)
	load(s.Files, snap.Files)
	load(s.ScheduledMessages, snap.ScheduledMessages)
	load(s.Tokens, snap.Tokens)
	load(s.Pins, snap.Pins)
	load(s.Bookmarks, snap.Bookmarks)
	load(s.Usergroups, snap.Usergroups)
	load(s.Views, snap.Views)
	load(s.Presences, snap.Presences)
	load(s.Bots, snap.Bots)
	load(s.Emoji, snap.Emoji)
	load(s.ReadStates, snap.ReadStates)
	if snap.Team != nil {
		s.Team = *snap.Team
	}
	if snap.Clock != nil {
		restoreClock(s.Clock, snap.Clock)
	}
	// The timestamp sequence continues after the highest one held, so a
	// snapshot written by hand without ts_counter cannot reissue a ts.
	next := highestTSSequence(s.Messages.Snapshot())
	if snap.TSCounter != nil {
		next = max(next, *snap.TSCounter)
	} else {
		next = max(next, s.tsCounter.Load())
	}
	s.tsCounter.Store(next)
	// twinkit/state does not restore a store's ID counter, so a restored
	// store would issue IDs it already holds and overwrite those items.
	advancePast(s.Channels)
	advancePast(s.Messages)
	advancePast(s.Users)
	advancePast(s.Files)
	advancePast(s.ScheduledMessages)
	advancePast(s.Tokens)
	advancePast(s.Pins)
	advancePast(s.Bookmarks)
	advancePast(s.Usergroups)
	advancePast(s.Views)
	advancePast(s.Bots)
	s.ensureDefaultPrincipals()
	return nil
}

// load replaces a store with the snapshot's items when the snapshot names
// the store, and leaves it alone when it does not.
func load[T any](st *pkgstate.Store[T], items map[string]T) {
	if items != nil {
		st.LoadSnapshot(items)
	}
}

func ptr[T any](v T) *T { return &v }

// highestTSSequence is the largest sequence number in the fraction of any
// message ts, the part NextTS counts up.
func highestTSSequence(msgs map[string]Message) int64 {
	var highest int64
	for _, m := range msgs {
		if _, frac, ok := strings.Cut(m.TS, "."); ok {
			if n, err := strconv.ParseInt(frac, 10, 64); err == nil && n > highest {
				highest = n
			}
		}
	}
	return highest
}

// clockState is the simulated clock in a snapshot.
type clockState struct {
	OffsetSeconds float64    `json:"offset_seconds"`
	PinnedAt      *time.Time `json:"pinned_at,omitempty"`
}

func snapshotClock(c *pkgstate.Clock) *clockState {
	st := &clockState{OffsetSeconds: c.Offset().Seconds()}
	if c.Pinned() {
		at := c.Now().Add(-c.Offset())
		st.PinnedAt = &at
	}
	return st
}

func restoreClock(c *pkgstate.Clock, st *clockState) {
	c.Reset()
	if st == nil {
		return
	}
	if st.PinnedAt != nil {
		c.Pin(*st.PinnedAt)
	}
	c.Advance(time.Duration(st.OffsetSeconds * float64(time.Second)))
}

func (s *MemoryStore) Reset() {
	s.Channels.Reset()
	s.Messages.Reset()
	s.Users.Reset()
	s.Pins.Reset()
	s.Files.Reset()
	s.ScheduledMessages.Reset()
	s.Bookmarks.Reset()
	s.Usergroups.Reset()
	s.Tokens.Reset()
	s.Views.Reset()
	s.Presences.Reset()
	s.Bots.Reset()
	s.Emoji.Reset()
	s.ReadStates.Reset()
	s.Clock.Reset()
	s.tsCounter.Store(0)
	s.Team = defaultTeam()
	s.ensureDefaultPrincipals()
}

// advancePast moves a store's ID counter past the highest numbered ID it
// holds, so the next NextID is one it does not already hold.
func advancePast[T any](st *pkgstate.Store[T]) {
	highest := 0
	for _, id := range st.ListIDs() {
		if n := idNumber(id); n > highest {
			highest = n
		}
	}
	for highest > 0 && idNumber(st.NextID()) < highest {
	}
}

// idNumber is the number after the last underscore in a NextID-style ID, or 0.
func idNumber(id string) int {
	i := strings.LastIndexByte(id, '_')
	if i < 0 {
		return 0
	}
	n, err := strconv.Atoi(id[i+1:])
	if err != nil {
		return 0
	}
	return n
}
