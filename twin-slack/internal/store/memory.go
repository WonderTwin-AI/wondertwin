package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

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
	Clock             *pkgstate.Clock

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
		Clock:             pkgstate.NewClock(),
		Team: Team{
			ID:     "T0001",
			Name:   "WonderTwin",
			Domain: "wondertwin",
		},
	}
	return s
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
	Channels          map[string]Channel          `json:"channels,omitempty"`
	Messages          map[string]Message          `json:"messages,omitempty"`
	Users             map[string]User             `json:"users,omitempty"`
	Files             map[string]File             `json:"files,omitempty"`
	ScheduledMessages map[string]ScheduledMessage `json:"scheduled_messages,omitempty"`
	Tokens            map[string]Token            `json:"tokens,omitempty"`
	Pins              map[string]Pin              `json:"pins,omitempty"`
	Bookmarks         map[string]Bookmark         `json:"bookmarks,omitempty"`
	Usergroups        map[string]Usergroup        `json:"usergroups,omitempty"`
	Views             map[string]ViewRecord       `json:"views,omitempty"`
	Presences         map[string]Presence         `json:"presences,omitempty"`
	Team              *Team                       `json:"team,omitempty"`
	// DeletedMessages names the messages chat.delete removed. A message's
	// deleted flag is not part of its Slack shape, so it travels here.
	DeletedMessages []string `json:"deleted_messages,omitempty"`
	// TSCounter is the last message timestamp sequence number, so a restored
	// emulator keeps issuing timestamps after the ones it already holds.
	TSCounter int64 `json:"ts_counter,omitempty"`
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
		Team:              &s.Team,
		DeletedMessages:   s.deletedMessages(),
		TSCounter:         s.tsCounter.Load(),
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

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Channels != nil {
		s.Channels.LoadSnapshot(snap.Channels)
	}
	if snap.Messages != nil {
		for _, key := range snap.DeletedMessages {
			if msg, ok := snap.Messages[key]; ok {
				msg.IsDeleted = true
				snap.Messages[key] = msg
			}
		}
		s.Messages.LoadSnapshot(snap.Messages)
	}
	if snap.TSCounter > 0 {
		s.tsCounter.Store(snap.TSCounter)
	}
	if snap.Users != nil {
		s.Users.LoadSnapshot(snap.Users)
	}
	if snap.Files != nil {
		s.Files.LoadSnapshot(snap.Files)
	}
	if snap.ScheduledMessages != nil {
		s.ScheduledMessages.LoadSnapshot(snap.ScheduledMessages)
	}
	if snap.Tokens != nil {
		s.Tokens.LoadSnapshot(snap.Tokens)
	}
	if snap.Pins != nil {
		s.Pins.LoadSnapshot(snap.Pins)
	}
	if snap.Bookmarks != nil {
		s.Bookmarks.LoadSnapshot(snap.Bookmarks)
	}
	if snap.Usergroups != nil {
		s.Usergroups.LoadSnapshot(snap.Usergroups)
	}
	if snap.Views != nil {
		s.Views.LoadSnapshot(snap.Views)
	}
	if snap.Presences != nil {
		s.Presences.LoadSnapshot(snap.Presences)
	}
	if snap.Team != nil {
		s.Team = *snap.Team
	}
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
	return nil
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
	s.Clock.Reset()
	s.tsCounter.Store(0)
	s.Team = Team{ID: "T0001", Name: "WonderTwin", Domain: "wondertwin"}
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
