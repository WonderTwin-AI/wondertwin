package store

import (
	"database/sql"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS events (
    id          TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL,
    twin        TEXT NOT NULL,
    twin_version TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    payload     TEXT NOT NULL,
    received_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_twin ON events(twin, twin_version);
CREATE INDEX IF NOT EXISTS idx_events_org ON events(org_id);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type, received_at);
CREATE INDEX IF NOT EXISTS idx_events_agent ON events(org_id, twin, received_at);
`

// Open opens (or creates) the collector database and runs migrations.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
