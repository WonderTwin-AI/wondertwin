package store

import (
	"encoding/json"
	"sync/atomic"

	"github.com/wondertwin-ai/wondertwin/twinkit/search"
	pkgstate "github.com/wondertwin-ai/wondertwin/twinkit/state"
)

// MemoryStore holds all Algolia twin state in memory.
// The search engine handles indices and records; this store handles
// Algolia-specific resources (synonyms, rules, API keys).
type MemoryStore struct {
	Engine   *search.Engine
	Synonyms *pkgstate.Store[Synonym]  // keyed by indexName:objectID
	Rules    *pkgstate.Store[Rule]     // keyed by indexName:objectID
	APIKeys  *pkgstate.Store[APIKey]
	Clock    *pkgstate.Clock

	taskCounter atomic.Int64
}

func New(engine *search.Engine) *MemoryStore {
	return &MemoryStore{
		Engine:   engine,
		Synonyms: pkgstate.New[Synonym]("syn"),
		Rules:    pkgstate.New[Rule]("rule"),
		APIKeys:  pkgstate.New[APIKey]("key"),
		Clock:    pkgstate.NewClock(),
	}
}

func (s *MemoryStore) NextTaskID() int64 {
	return s.taskCounter.Add(1)
}

func (s *MemoryStore) Now() string {
	return s.Clock.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// TaskResult returns an Algolia async task response.
func (s *MemoryStore) TaskResult(indexName string) map[string]any {
	return map[string]any{
		"taskID":    s.NextTaskID(),
		"updatedAt": s.Now(),
	}
}

// ListIndexSynonyms returns synonyms for a specific index.
func (s *MemoryStore) ListIndexSynonyms(indexName string) []Synonym {
	return s.Synonyms.Filter(func(id string, syn Synonym) bool {
		// Keys are stored as indexName:objectID
		return len(id) > len(indexName)+1 && id[:len(indexName)+1] == indexName+":"
	})
}

// ListIndexRules returns rules for a specific index.
func (s *MemoryStore) ListIndexRules(indexName string) []Rule {
	return s.Rules.Filter(func(id string, rule Rule) bool {
		return len(id) > len(indexName)+1 && id[:len(indexName)+1] == indexName+":"
	})
}

// SynonymKey builds a store key for a synonym.
func SynonymKey(indexName, objectID string) string {
	return indexName + ":" + objectID
}

// RuleKey builds a store key for a rule.
func RuleKey(indexName, objectID string) string {
	return indexName + ":" + objectID
}

type stateSnapshot struct {
	Synonyms map[string]Synonym `json:"synonyms,omitempty"`
	Rules    map[string]Rule    `json:"rules,omitempty"`
	APIKeys  map[string]APIKey  `json:"api_keys,omitempty"`
}

func (s *MemoryStore) Snapshot() any {
	return stateSnapshot{
		Synonyms: s.Synonyms.Snapshot(),
		Rules:    s.Rules.Snapshot(),
		APIKeys:  s.APIKeys.Snapshot(),
	}
}

func (s *MemoryStore) LoadState(data []byte) error {
	var snap stateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Synonyms != nil {
		s.Synonyms.LoadSnapshot(snap.Synonyms)
	}
	if snap.Rules != nil {
		s.Rules.LoadSnapshot(snap.Rules)
	}
	if snap.APIKeys != nil {
		s.APIKeys.LoadSnapshot(snap.APIKeys)
	}
	return nil
}

func (s *MemoryStore) Reset() {
	s.Synonyms.Reset()
	s.Rules.Reset()
	s.APIKeys.Reset()
	s.Clock.Reset()
	s.taskCounter.Store(0)
	// Note: Engine indices are NOT reset here — the engine is shared infrastructure.
	// Twin-level reset should call engine methods separately if needed.
}
