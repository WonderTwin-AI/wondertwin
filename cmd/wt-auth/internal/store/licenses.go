package store

import (
	"database/sql"
	"fmt"
	"time"
)

// LicenseStore handles license CRUD operations.
type LicenseStore struct {
	db *sql.DB
}

// NewLicenseStore creates a new LicenseStore.
func NewLicenseStore(db *sql.DB) *LicenseStore {
	return &LicenseStore{db: db}
}

// CreateOrg creates a new organization.
func (s *LicenseStore) CreateOrg(org *Org) error {
	_, err := s.db.Exec(
		`INSERT INTO orgs (id, name, tier, created_at, active) VALUES (?, ?, ?, ?, ?)`,
		org.ID, org.Name, org.Tier, org.CreatedAt.Unix(), boolToInt(org.Active),
	)
	return err
}

// GetOrg returns an org by ID.
func (s *LicenseStore) GetOrg(id string) (*Org, error) {
	row := s.db.QueryRow(`SELECT id, name, tier, created_at, active FROM orgs WHERE id = ?`, id)
	var org Org
	var createdAt int64
	var active int
	if err := row.Scan(&org.ID, &org.Name, &org.Tier, &createdAt, &active); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("org %q not found", id)
		}
		return nil, err
	}
	org.CreatedAt = time.Unix(createdAt, 0)
	org.Active = active == 1
	return &org, nil
}

// CreateLicense creates a new license.
func (s *LicenseStore) CreateLicense(lic *License) error {
	var expiresAt *int64
	if lic.ExpiresAt != nil {
		v := lic.ExpiresAt.Unix()
		expiresAt = &v
	}
	_, err := s.db.Exec(
		`INSERT INTO licenses (id, org_id, tier, twin_name, twin_class, billing_period, active, issued_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lic.ID, lic.OrgID, lic.Tier, lic.TwinName, lic.TwinClass, lic.BillingPeriod,
		boolToInt(lic.Active), lic.IssuedAt.Unix(), expiresAt,
	)
	return err
}

// GetLicense returns a license by ID.
func (s *LicenseStore) GetLicense(id string) (*License, error) {
	row := s.db.QueryRow(
		`SELECT id, org_id, tier, twin_name, twin_class, billing_period, active, issued_at, expires_at, revoked_at, revoke_reason
		 FROM licenses WHERE id = ?`, id)
	return scanLicense(row)
}

// FindLicense finds an active license for an org + twin combination.
func (s *LicenseStore) FindLicense(orgID, twinName string) (*License, error) {
	row := s.db.QueryRow(
		`SELECT id, org_id, tier, twin_name, twin_class, billing_period, active, issued_at, expires_at, revoked_at, revoke_reason
		 FROM licenses WHERE org_id = ? AND twin_name = ? AND active = 1 AND revoked_at IS NULL
		 ORDER BY issued_at DESC LIMIT 1`, orgID, twinName)
	return scanLicense(row)
}

// ListLicenses returns all licenses for an org.
func (s *LicenseStore) ListLicenses(orgID string) ([]*License, error) {
	rows, err := s.db.Query(
		`SELECT id, org_id, tier, twin_name, twin_class, billing_period, active, issued_at, expires_at, revoked_at, revoke_reason
		 FROM licenses WHERE org_id = ? ORDER BY issued_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var licenses []*License
	for rows.Next() {
		lic, err := scanLicenseRow(rows)
		if err != nil {
			return nil, err
		}
		licenses = append(licenses, lic)
	}
	return licenses, rows.Err()
}

// RevokeLicense revokes a license immediately.
func (s *LicenseStore) RevokeLicense(id, reason string) error {
	now := time.Now().Unix()
	result, err := s.db.Exec(
		`UPDATE licenses SET active = 0, revoked_at = ?, revoke_reason = ? WHERE id = ?`,
		now, reason, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("license %q not found", id)
	}
	return nil
}

func scanLicense(row *sql.Row) (*License, error) {
	var lic License
	var active int
	var issuedAt int64
	var expiresAt, revokedAt sql.NullInt64
	var revokeReason sql.NullString
	err := row.Scan(&lic.ID, &lic.OrgID, &lic.Tier, &lic.TwinName, &lic.TwinClass,
		&lic.BillingPeriod, &active, &issuedAt, &expiresAt, &revokedAt, &revokeReason)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("license not found")
		}
		return nil, err
	}
	lic.Active = active == 1
	lic.IssuedAt = time.Unix(issuedAt, 0)
	if expiresAt.Valid {
		t := time.Unix(expiresAt.Int64, 0)
		lic.ExpiresAt = &t
	}
	if revokedAt.Valid {
		t := time.Unix(revokedAt.Int64, 0)
		lic.RevokedAt = &t
	}
	if revokeReason.Valid {
		lic.RevokeReason = revokeReason.String
	}
	return &lic, nil
}

func scanLicenseRow(rows *sql.Rows) (*License, error) {
	var lic License
	var active int
	var issuedAt int64
	var expiresAt, revokedAt sql.NullInt64
	var revokeReason sql.NullString
	err := rows.Scan(&lic.ID, &lic.OrgID, &lic.Tier, &lic.TwinName, &lic.TwinClass,
		&lic.BillingPeriod, &active, &issuedAt, &expiresAt, &revokedAt, &revokeReason)
	if err != nil {
		return nil, err
	}
	lic.Active = active == 1
	lic.IssuedAt = time.Unix(issuedAt, 0)
	if expiresAt.Valid {
		t := time.Unix(expiresAt.Int64, 0)
		lic.ExpiresAt = &t
	}
	if revokedAt.Valid {
		t := time.Unix(revokedAt.Int64, 0)
		lic.RevokedAt = &t
	}
	if revokeReason.Valid {
		lic.RevokeReason = revokeReason.String
	}
	return &lic, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
