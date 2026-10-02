package api

import (
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"

	"github.com/wondertwin-ai/wondertwin/twin-slack/internal/store"
)

// appID is the app the emulator's bot belongs to, as bots.info reports it.
const appID = "A_SIM"

// viewInput is the view payload a caller sends. Slack documents it as a
// JSON-encoded string; the node SDK sends it that way in a form field, and the
// python SDK sends it as an object inside a JSON body. Both decode here.
type viewInput struct {
	Type            string          `json:"type"`
	Title           json.RawMessage `json:"title"`
	Close           json.RawMessage `json:"close"`
	Submit          json.RawMessage `json:"submit"`
	Blocks          json.RawMessage `json:"blocks"`
	PrivateMetadata string          `json:"private_metadata"`
	CallbackID      string          `json:"callback_id"`
	ExternalID      string          `json:"external_id"`
	ClearOnClose    bool            `json:"clear_on_close"`
	NotifyOnClose   bool            `json:"notify_on_close"`
	SubmitDisabled  bool            `json:"submit_disabled"`
}

// decodeView reads a view argument, which the argument decoder hands over as
// a decoded object or as the string it was sent as.
func decodeView(v any) (viewInput, bool) {
	var raw []byte
	switch t := v.(type) {
	case nil:
		return viewInput{}, false
	case string:
		raw = []byte(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return viewInput{}, false
		}
		raw = b
	}
	var in viewInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return viewInput{}, false
	}
	return in, true
}

// invalidArgument answers invalid_arguments with the argument named, the way
// the views.publish docs show it.
func invalidArgument(w http.ResponseWriter, name string) {
	slackErrorWith(w, "invalid_arguments", map[string]any{
		"response_metadata": map[string]any{"messages": []string{"invalid `" + name + "`"}},
	})
}

// buildView turns a caller's view into the stored view with the given id.
// Each save gets a fresh hash, so a caller holding an older one is refused
// with hash_conflict.
func (h *Handler) buildView(r *http.Request, id, typ string, in viewInput) store.View {
	botID := principal(r).BotID
	if botID == "" {
		botID = store.DefaultBotID
	}
	return store.View{
		ID:              id,
		TeamID:          h.store.Team.ID,
		Type:            typ,
		Title:           in.Title,
		Close:           in.Close,
		Submit:          in.Submit,
		Blocks:          withBlockIDs(id, in.Blocks),
		PrivateMetadata: in.PrivateMetadata,
		CallbackID:      in.CallbackID,
		State:           store.ViewState{Values: map[string]any{}},
		Hash:            h.store.NextTS(),
		ClearOnClose:    in.ClearOnClose,
		NotifyOnClose:   in.NotifyOnClose,
		SubmitDisabled:  in.SubmitDisabled,
		RootViewID:      id,
		AppID:           appID,
		ExternalID:      in.ExternalID,
		BotID:           botID,
	}
}

// withBlockIDs gives every block that has no block_id one, as the published
// view in the docs example shows ("block_id": "2WGp9"). Whether Slack does
// this for every block is not verified.
func withBlockIDs(viewID string, raw json.RawMessage) json.RawMessage {
	var blocks []map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &blocks) != nil {
		return json.RawMessage("[]")
	}
	for i, b := range blocks {
		if id, _ := b["block_id"].(string); id == "" {
			b["block_id"] = blockID(viewID, i)
		}
	}
	out, err := json.Marshal(blocks)
	if err != nil {
		return json.RawMessage("[]")
	}
	return out
}

// blockID derives a five-character alphanumeric id, stable for a block's
// position in a view, so a republish does not churn the ids.
func blockID(viewID string, i int) string {
	sum := sha256.Sum256([]byte(viewID + "/" + strconv.Itoa(i)))
	s := new(big.Int).SetBytes(sum[:8]).Text(62)
	for len(s) < 5 {
		s = "0" + s
	}
	return s[:5]
}

// externalIDTaken reports whether another view already uses externalID. Slack
// requires it to be unique across a team's views.
func (h *Handler) externalIDTaken(externalID, exceptID string) bool {
	if externalID == "" {
		return false
	}
	return len(h.store.Views.Filter(func(id string, v store.ViewRecord) bool {
		return id != exceptID && v.View.ExternalID == externalID
	})) > 0
}

// homeView returns the Home tab view published to a user, if there is one.
func (h *Handler) homeView(userID string) (store.ViewRecord, bool) {
	_, recs := h.store.Views.FilterWithIDs(func(_ string, v store.ViewRecord) bool {
		return v.UserID == userID && v.View.Type == "home"
	})
	if len(recs) == 0 {
		return store.ViewRecord{}, false
	}
	return recs[0], true
}

// ViewsPublish handles /api/views.publish. A user has one Home tab per app,
// so publishing again replaces the view and keeps its id.
func (h *Handler) ViewsPublish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		View   any    `json:"view"`
		Hash   string `json:"hash"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	if !h.store.KnownUser(req.UserID) {
		invalidArgument(w, "user_id")
		return
	}
	in, ok := decodeView(req.View)
	if !ok {
		invalidArgument(w, "view")
		return
	}
	// Only Home tab views are published here. Slack also publishes profile
	// views, which this emulator does not serve.
	if in.Type != "home" {
		invalidArgument(w, "view")
		return
	}
	prev, exists := h.homeView(req.UserID)
	if exists && req.Hash != "" && req.Hash != prev.View.Hash {
		slackError(w, "hash_conflict")
		return
	}
	id := prev.View.ID
	if !exists {
		id = h.store.Views.NextID()
	}
	if h.externalIDTaken(in.ExternalID, id) {
		slackError(w, "duplicate_external_id")
		return
	}
	view := h.buildView(r, id, "home", in)
	h.store.Views.Set(id, store.ViewRecord{UserID: req.UserID, View: view})
	slackOK(w, map[string]any{"view": view})
}

// ViewsUpdate handles /api/views.update. The view is found by view_id, else
// by external_id, and keeps its id and type.
func (h *Handler) ViewsUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ViewID     string `json:"view_id"`
		ExternalID string `json:"external_id"`
		View       any    `json:"view"`
		Hash       string `json:"hash"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	in, ok := decodeView(req.View)
	if !ok {
		invalidArgument(w, "view")
		return
	}
	if req.ViewID == "" && req.ExternalID == "" {
		invalidArgument(w, "view_id")
		return
	}
	id, rec, found := req.ViewID, store.ViewRecord{}, false
	if id != "" {
		rec, found = h.store.Views.Get(id)
	} else {
		ids, recs := h.store.Views.FilterWithIDs(func(_ string, v store.ViewRecord) bool {
			return v.View.ExternalID == req.ExternalID
		})
		if len(ids) > 0 {
			id, rec, found = ids[0], recs[0], true
		}
	}
	if !found {
		slackError(w, "not_found")
		return
	}
	if req.Hash != "" && req.Hash != rec.View.Hash {
		slackError(w, "hash_conflict")
		return
	}
	if in.ExternalID == "" {
		in.ExternalID = rec.View.ExternalID
	}
	if h.externalIDTaken(in.ExternalID, id) {
		slackError(w, "duplicate_external_id")
		return
	}
	view := h.buildView(r, id, rec.View.Type, in)
	view.RootViewID = rec.View.RootViewID
	view.PreviousViewID = rec.View.PreviousViewID
	h.store.Views.Set(id, store.ViewRecord{UserID: rec.UserID, View: view})
	slackOK(w, map[string]any{"view": view})
}

// ViewsOpen handles /api/views.open. The view is stored so views.update can
// find it. trigger_id is not checked: the emulator issues no triggers until
// interactivity is served.
func (h *Handler) ViewsOpen(w http.ResponseWriter, r *http.Request) {
	h.openModal(w, r)
}

// ViewsPush handles /api/views.push, which stacks a modal on the one the
// trigger came from. With no triggers to follow, the pushed view is its own
// root, and the two-push limit is not enforced.
func (h *Handler) ViewsPush(w http.ResponseWriter, r *http.Request) {
	h.openModal(w, r)
}

// openModal stores a modal from views.open or views.push.
func (h *Handler) openModal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TriggerID string `json:"trigger_id"`
		View      any    `json:"view"`
	}
	if err := parseJSON(r, &req); err != nil {
		slackArgsError(w, err)
		return
	}
	in, ok := decodeView(req.View)
	if !ok {
		invalidArgument(w, "view")
		return
	}
	if h.externalIDTaken(in.ExternalID, "") {
		slackError(w, "duplicate_external_id")
		return
	}
	typ := in.Type
	if typ == "" {
		typ = "modal"
	}
	id := h.store.Views.NextID()
	view := h.buildView(r, id, typ, in)
	h.store.Views.Set(id, store.ViewRecord{View: view})
	slackOK(w, map[string]any{"view": view})
}
