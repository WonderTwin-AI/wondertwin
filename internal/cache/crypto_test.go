package cache

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := DeriveKey("wt_com_acme_abcdef_xx", "stripe", "0.1.0")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}

	plaintext := []byte(`{"quirks": [{"id": "STRIPE-Q-001"}]}`)
	enc, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	dec, err := Decrypt(key, enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, dec) {
		t.Fatalf("round-trip mismatch: got %q, want %q", dec, plaintext)
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	key1, _ := DeriveKey("key-one", "stripe", "0.1.0")
	key2, _ := DeriveKey("key-two", "stripe", "0.1.0")

	enc, err := Encrypt(key1, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = Decrypt(key2, enc)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
}

func TestDeriveKeyDeterministic(t *testing.T) {
	k1, _ := DeriveKey("license", "twin", "1.0.0")
	k2, _ := DeriveKey("license", "twin", "1.0.0")

	if !bytes.Equal(k1, k2) {
		t.Fatal("DeriveKey is not deterministic")
	}
}

func TestDeriveKeyDifferentInputs(t *testing.T) {
	k1, _ := DeriveKey("license", "stripe", "1.0.0")
	k2, _ := DeriveKey("license", "twilio", "1.0.0")

	if bytes.Equal(k1, k2) {
		t.Fatal("different twins should produce different keys")
	}
}
