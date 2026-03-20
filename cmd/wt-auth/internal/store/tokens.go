package store

import (
	"database/sql"
	"fmt"
	"time"
)

// TokenStore handles token issuance and validation.
type TokenStore struct {
	db *sql.DB
}

// NewTokenStore creates a new TokenStore.
func NewTokenStore(db *sql.DB) *TokenStore {
	return &TokenStore{db: db}
}

// CreateToken records a newly issued token.
func (s *TokenStore) CreateToken(tok *Token) error {
	_, err := s.db.Exec(
		`INSERT INTO tokens (id, license_id, org_id, twin_name, issued_at, expires_at, revoked)
		 VALUES (?, ?, ?, ?, ?, ?, 0)`,
		tok.ID, tok.LicenseID, tok.OrgID, tok.TwinName, tok.IssuedAt.Unix(), tok.ExpiresAt.Unix(),
	)
	return err
}

// GetToken returns a token by ID.
func (s *TokenStore) GetToken(id string) (*Token, error) {
	row := s.db.QueryRow(
		`SELECT id, license_id, org_id, twin_name, issued_at, expires_at, revoked
		 FROM tokens WHERE id = ?`, id)
	var tok Token
	var issuedAt, expiresAt int64
	var revoked int
	err := row.Scan(&tok.ID, &tok.LicenseID, &tok.OrgID, &tok.TwinName, &issuedAt, &expiresAt, &revoked)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token not found")
		}
		return nil, err
	}
	tok.IssuedAt = time.Unix(issuedAt, 0)
	tok.ExpiresAt = time.Unix(expiresAt, 0)
	tok.Revoked = revoked == 1
	return &tok, nil
}

// IsTokenValid checks if a token exists, is not expired, and is not revoked.
func (s *TokenStore) IsTokenValid(id, twinName string) (*Token, error) {
	tok, err := s.GetToken(id)
	if err != nil {
		return nil, err
	}
	if tok.Revoked {
		return nil, fmt.Errorf("token revoked")
	}
	if time.Now().After(tok.ExpiresAt) {
		return nil, fmt.Errorf("token expired")
	}
	if tok.TwinName != twinName {
		return nil, fmt.Errorf("token not valid for twin %q", twinName)
	}
	return tok, nil
}
