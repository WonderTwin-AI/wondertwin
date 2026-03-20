package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// RawEvent is a telemetry event as received from the emitter.
type RawEvent struct {
	EventType   string `json:"event_type"`
	Twin        string `json:"twin"`
	TwinVersion string `json:"twin_version"`
	OrgID       string `json:"org_id"`
	Timestamp   int64  `json:"timestamp"`
	Payload     any    `json:"payload"`
}

// EventStore handles telemetry event persistence.
type EventStore struct {
	db *sql.DB
}

// NewEventStore creates a new EventStore.
func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db}
}

// InsertBatch stores a batch of events. Append-only — no updates, no deletes.
func (s *EventStore) InsertBatch(events []RawEvent) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO events (id, org_id, twin, twin_version, event_type, payload, received_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Unix()
	inserted := 0

	for _, ev := range events {
		if ev.OrgID == "" || ev.Twin == "" || ev.TwinVersion == "" || ev.EventType == "" {
			continue // skip invalid events
		}

		payloadJSON, err := json.Marshal(ev.Payload)
		if err != nil {
			continue
		}

		id := generateEventID()
		if _, err := stmt.Exec(id, ev.OrgID, ev.Twin, ev.TwinVersion, ev.EventType, string(payloadJSON), now); err != nil {
			continue // skip duplicates or errors, don't fail the batch
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return inserted, nil
}

// Count returns the total number of stored events.
func (s *EventStore) Count() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	return count, err
}

// CountByTwin returns event counts grouped by twin.
func (s *EventStore) CountByTwin() (map[string]int64, error) {
	rows, err := s.db.Query("SELECT twin, COUNT(*) FROM events GROUP BY twin")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int64)
	for rows.Next() {
		var twin string
		var count int64
		if err := rows.Scan(&twin, &count); err != nil {
			return nil, err
		}
		result[twin] = count
	}
	return result, rows.Err()
}

func generateEventID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "evt_" + hex.EncodeToString(b)
}
