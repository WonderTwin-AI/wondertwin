package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS orgs (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    tier        TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    active      INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS licenses (
    id              TEXT PRIMARY KEY,
    org_id          TEXT NOT NULL REFERENCES orgs(id),
    tier            TEXT NOT NULL,
    twin_name       TEXT NOT NULL,
    twin_class      TEXT NOT NULL,
    billing_period  TEXT NOT NULL,
    active          INTEGER NOT NULL DEFAULT 1,
    issued_at       INTEGER NOT NULL,
    expires_at      INTEGER,
    revoked_at      INTEGER,
    revoke_reason   TEXT
);

CREATE TABLE IF NOT EXISTS tokens (
    id              TEXT PRIMARY KEY,
    license_id      TEXT NOT NULL REFERENCES licenses(id),
    org_id          TEXT NOT NULL,
    twin_name       TEXT NOT NULL,
    issued_at       INTEGER NOT NULL,
    expires_at      INTEGER NOT NULL,
    revoked         INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_licenses_org ON licenses(org_id);
CREATE INDEX IF NOT EXISTS idx_licenses_twin ON licenses(twin_name);
CREATE INDEX IF NOT EXISTS idx_tokens_license ON tokens(license_id);
`

// Open opens (or creates) the SQLite database and runs migrations.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Enable WAL mode for concurrent reads.
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
