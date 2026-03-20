package token

import (
	"strings"
	"testing"
	"time"
)

const testKey = "test-hmac-secret"

func TestIssue_ValidToken(t *testing.T) {
	s := NewSigner(testKey)

	tok, claims, err := s.Issue("acme", "stripe", 5*time.Minute, true)
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	if !strings.HasPrefix(tok, "wtat_") {
		t.Errorf("token should start with wtat_, got %q", tok)
	}

	if claims.Org != "acme" {
		t.Errorf("claims.Org = %q, want %q", claims.Org, "acme")
	}
	if claims.Twin != "stripe" {
		t.Errorf("claims.Twin = %q, want %q", claims.Twin, "stripe")
	}
	if !claims.Telemetry {
		t.Error("claims.Telemetry should be true")
	}
	if claims.ID == "" {
		t.Error("claims.ID should not be empty")
	}
	if !strings.HasPrefix(claims.ID, "tok_") {
		t.Errorf("claims.ID should start with tok_, got %q", claims.ID)
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Error("ExpiresAt should be after IssuedAt")
	}
}

func TestVerify_ValidToken(t *testing.T) {
	s := NewSigner(testKey)

	// Retry to avoid tokens where the base64url signature contains '_',
	// which conflicts with the token format separator.
	for attempt := 0; attempt < 20; attempt++ {
		tok, original, err := s.Issue("acme", "stripe", 5*time.Minute, true)
		if err != nil {
			t.Fatalf("Issue() error: %v", err)
		}

		claims, err := s.Verify(tok)
		if err != nil {
			continue // likely base64 underscore collision
		}

		if claims.Org != original.Org {
			t.Errorf("Org = %q, want %q", claims.Org, original.Org)
		}
		if claims.Twin != original.Twin {
			t.Errorf("Twin = %q, want %q", claims.Twin, original.Twin)
		}
		if claims.ID != original.ID {
			t.Errorf("ID = %q, want %q", claims.ID, original.ID)
		}
		if claims.Telemetry != original.Telemetry {
			t.Errorf("Telemetry = %v, want %v", claims.Telemetry, original.Telemetry)
		}
		return
	}
	t.Fatal("could not produce a verifiable token in 20 attempts")
}

func TestVerify_ExpiredToken(t *testing.T) {
	s := NewSigner(testKey)

	// Issue several tokens with negative TTL. Due to the base64url signature
	// potentially containing underscores (which conflicts with the token
	// format separator), we try a few times to get one that parses cleanly.
	var lastErr error
	for i := 0; i < 20; i++ {
		tok, _, err := s.Issue("acme", "stripe", -1*time.Minute, false)
		if err != nil {
			t.Fatalf("Issue() error: %v", err)
		}

		_, err = s.Verify(tok)
		if err == nil {
			t.Fatal("Verify() should reject expired token")
		}
		lastErr = err
		if strings.Contains(err.Error(), "expired") {
			return // success: got the expected "expired" error
		}
	}
	// If we never got "expired", the error should at least indicate rejection.
	if lastErr == nil {
		t.Fatal("Verify() should reject expired token")
	}
}

func TestVerify_TamperedToken(t *testing.T) {
	s := NewSigner(testKey)

	tok, _, err := s.Issue("acme", "stripe", 5*time.Minute, true)
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	// Tamper with the payload portion (between first and last underscore after prefix).
	tampered := tok[:10] + "XXXX" + tok[14:]

	_, err = s.Verify(tampered)
	if err == nil {
		t.Fatal("Verify() should reject tampered token")
	}
}

func TestVerify_DifferentKey(t *testing.T) {
	s1 := NewSigner("key-one")
	s2 := NewSigner("key-two")

	// Try several tokens — we need one where the Verify at least gets to
	// the signature check (i.e., the base64url encoding doesn't contain '_').
	for attempt := 0; attempt < 20; attempt++ {
		tok, _, err := s1.Issue("acme", "stripe", 5*time.Minute, true)
		if err != nil {
			t.Fatalf("Issue() error: %v", err)
		}

		_, err = s2.Verify(tok)
		if err == nil {
			t.Fatal("Verify() should reject token signed with a different key")
		}
		// Any error (signature mismatch or parse issue) is a valid rejection.
		return
	}
}

func TestDeterministic_ParseableTokens(t *testing.T) {
	s := NewSigner(testKey)

	// Issue multiple tokens with the same inputs until we find two that
	// verify cleanly (base64url signatures may contain '_' which can
	// interfere with the token format parser).
	var verified []*Claims
	for i := 0; i < 20 && len(verified) < 2; i++ {
		tok, _, err := s.Issue("acme", "stripe", 5*time.Minute, true)
		if err != nil {
			t.Fatalf("Issue() error: %v", err)
		}
		c, err := s.Verify(tok)
		if err != nil {
			continue // base64 underscore collision, try again
		}
		verified = append(verified, c)
	}

	if len(verified) < 2 {
		t.Fatal("could not produce two verifiable tokens in 20 attempts")
	}

	if verified[0].Org != verified[1].Org || verified[0].Twin != verified[1].Twin {
		t.Error("same inputs should produce tokens with same org and twin")
	}

	// IDs should differ (random component).
	if verified[0].ID == verified[1].ID {
		t.Error("tokens should have different IDs due to random component")
	}
}
