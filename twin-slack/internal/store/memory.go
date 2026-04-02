package store

import (
	"encoding/json"
	"fmt"
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
	Clock             *pkgstate.Clock

	// Team info (singleton)
	Team Team

	// Thread tracking: maps channel+thread_ts to reply count
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
	var result []Message
	// Iterate in reverse (newest first)
	for i := len(all) - 1; i >= 0; i-- {
		msg := all[i]
		if msg.Channel == channel && !msg.IsDeleted && msg.ThreadTS == "" {
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
	var result []Message
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

type stateSnapshot struct {
	Channels          map[string]Channel          `json:"channels,omitempty"`
	Messages          map[string]Message          `json:"messages,omitempty"`
	Users             map[string]User             `json:"users,omitempty"`
	Files             map[string]File             `json:"files,omitempty"`
	ScheduledMessages map[string]ScheduledMessage `json:"scheduled_messages,omitempty"`
	Team              *Team                       `json:"team,omitempty"`
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Channels:          s.Channels.Snapshot(),
		Messages:          s.Messages.Snapshot(),
		Users:             s.Users.Snapshot(),
		Files:             s.Files.Snapshot(),
		ScheduledMessages: s.ScheduledMessages.Snapshot(),
		Team:              &s.Team,
	}
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
		s.Messages.LoadSnapshot(snap.Messages)
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
	if snap.Team != nil {
		s.Team = *snap.Team
	}
	return nil
}

func (s *MemoryStore) Reset() {
	s.Channels.Reset()
	s.Messages.Reset()
	s.Users.Reset()
	s.Pins.Reset()
	s.Files.Reset()
	s.ScheduledMessages.Reset()
	s.Clock.Reset()
	s.tsCounter.Store(0)
	s.Team = Team{ID: "T0001", Name: "WonderTwin", Domain: "wondertwin"}
}
