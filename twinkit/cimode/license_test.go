package cimode

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// signedLicense returns a License signed by priv. Tests use this
// helper to avoid replicating signing wiring.
func signedLicense(t *testing.T, priv ed25519.PrivateKey, lic License) License {
	t.Helper()
	bytes, err := lic.SigningBytes()
	if err != nil {
		t.Fatalf("SigningBytes: %v", err)
	}
	sig := ed25519.Sign(priv, bytes)
	lic.SetSignature(sig)
	return lic
}

func newTestKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

func TestValidateRoundTrip(t *testing.T) {
	pub, priv := newTestKeypair(t)
	restore := setVerificationKey(pub)
	defer restore()

	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	lic := signedLicense(t, priv, License{
		Key:       "lic_test",
		OrgID:     "org_acme",
		TwinScope: []string{"stripe"},
		IssuedAt:  now,
		NotAfter:  now.Add(90 * 24 * time.Hour),
	})

	st := ValidateLicense(&lic, "stripe", now)
	if !st.Valid {
		t.Fatalf("expected valid; reason=%q", st.Reason)
	}
	if st.ExpiresIn <= 0 {
		t.Errorf("ExpiresIn = %v, want positive", st.ExpiresIn)
	}
}

func TestValidateNilLicense(t *testing.T) {
	st := ValidateLicense(nil, "stripe", time.Now())
	if st.Valid || st.Reason != ReasonNoLicense || st.ExpiresIn != 0 {
		t.Errorf("nil license status = %+v", st)
	}
}

func TestValidateTamperedPayload(t *testing.T) {
	pub, priv := newTestKeypair(t)
	restore := setVerificationKey(pub)
	defer restore()

	now := time.Now().UTC()
	lic := signedLicense(t, priv, License{
		Key:       "lic_test",
		OrgID:     "org_a",
		TwinScope: []string{"*"},
		IssuedAt:  now,
		NotAfter:  now.Add(time.Hour),
	})
	lic.OrgID = "org_evil" // tamper after signing

	st := ValidateLicense(&lic, "stripe", now)
	if st.Valid || st.Reason != ReasonInvalidSig {
		t.Errorf("tampered status = %+v, want invalid signature", st)
	}
}

func TestValidateWrongKey(t *testing.T) {
	_, priv := newTestKeypair(t)
	otherPub, _ := newTestKeypair(t)
	restore := setVerificationKey(otherPub)
	defer restore()

	now := time.Now().UTC()
	lic := signedLicense(t, priv, License{
		Key: "lic", OrgID: "o", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(time.Hour),
	})
	st := ValidateLicense(&lic, "stripe", now)
	if st.Valid || st.Reason != ReasonInvalidSig {
		t.Errorf("wrong-key status = %+v", st)
	}
}

func TestValidateExpired(t *testing.T) {
	pub, priv := newTestKeypair(t)
	restore := setVerificationKey(pub)
	defer restore()

	issued := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	expires := issued.Add(24 * time.Hour)
	lic := signedLicense(t, priv, License{
		Key: "lic", OrgID: "o", TwinScope: []string{"*"},
		IssuedAt: issued, NotAfter: expires,
	})
	now := expires.Add(time.Hour)
	st := ValidateLicense(&lic, "stripe", now)
	if st.Valid || st.Reason != ReasonExpired {
		t.Errorf("expired status = %+v", st)
	}
	if st.ExpiresIn >= 0 {
		t.Errorf("ExpiresIn = %v, want negative", st.ExpiresIn)
	}
}

func TestValidateTwinScope(t *testing.T) {
	pub, priv := newTestKeypair(t)
	restore := setVerificationKey(pub)
	defer restore()

	now := time.Now().UTC()
	lic := signedLicense(t, priv, License{
		Key: "lic", OrgID: "o", TwinScope: []string{"stripe"},
		IssuedAt: now, NotAfter: now.Add(time.Hour),
	})
	stOK := ValidateLicense(&lic, "stripe", now)
	if !stOK.Valid {
		t.Errorf("stripe should validate; status=%+v", stOK)
	}
	stMiss := ValidateLicense(&lic, "twilio", now)
	if stMiss.Valid || stMiss.Reason != ReasonTwinOutOfScope {
		t.Errorf("twilio status = %+v", stMiss)
	}
}

func TestValidateWildcardScope(t *testing.T) {
	pub, priv := newTestKeypair(t)
	restore := setVerificationKey(pub)
	defer restore()

	now := time.Now().UTC()
	lic := signedLicense(t, priv, License{
		Key: "lic", OrgID: "o", TwinScope: []string{"*"},
		IssuedAt: now, NotAfter: now.Add(time.Hour),
	})
	for _, name := range []string{"stripe", "twilio", "anything"} {
		st := ValidateLicense(&lic, name, now)
		if !st.Valid {
			t.Errorf("wildcard scope failed for %q: %+v", name, st)
		}
	}
}

func TestLoadLicenseFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "license.json")
	want := License{Key: "k", OrgID: "o", TwinScope: []string{"*"}}
	data, _ := json.Marshal(want)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLicenseFromPath(path)
	if err != nil {
		t.Fatalf("LoadLicenseFromPath: %v", err)
	}
	if got.Key != "k" {
		t.Errorf("Key = %q", got.Key)
	}
}

func TestLoadLicenseExplicitMissing(t *testing.T) {
	t.Setenv(EnvLicenseFile, "/nonexistent/path/license.json")
	if _, err := LoadLicense(); err == nil {
		t.Error("explicit missing path should error")
	}
}

func TestLoadLicenseDefaultMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvLicenseFile, "")
	t.Setenv(EnvHome, dir)
	got, err := LoadLicense()
	if err != nil {
		t.Fatalf("expected nil error for missing default; got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil license; got %+v", got)
	}
}

func TestLoadLicenseHomeOverride(t *testing.T) {
	dir := t.TempDir()
	want := License{Key: "k", OrgID: "o", TwinScope: []string{"*"}}
	data, _ := json.Marshal(want)
	os.WriteFile(filepath.Join(dir, DefaultLicenseFilename), data, 0o600)

	t.Setenv(EnvLicenseFile, "")
	t.Setenv(EnvHome, dir)
	got, err := LoadLicense()
	if err != nil {
		t.Fatalf("LoadLicense: %v", err)
	}
	if got == nil || got.Key != "k" {
		t.Errorf("got = %+v", got)
	}
}

func TestSigningBytesIsDeterministic(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	lic := License{Key: "k", OrgID: "o", TwinScope: []string{"a", "b"}, IssuedAt: now, NotAfter: now}
	a, _ := lic.SigningBytes()
	b, _ := lic.SigningBytes()
	if string(a) != string(b) {
		t.Errorf("SigningBytes non-deterministic:\nA: %s\nB: %s", a, b)
	}
	// Signature field must not appear in signing bytes.
	lic.Signature = "spurious"
	c, _ := lic.SigningBytes()
	if string(a) != string(c) {
		t.Errorf("Signature field leaked into SigningBytes:\nbase:    %s\nwithSig: %s", a, c)
	}
}
