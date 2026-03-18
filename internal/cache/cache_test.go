package cache

import (
	"bytes"
	"testing"
	"time"
)

func TestPutGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "wt_com_acme_abcdef_xx")

	payload := []byte(`{"quirks": []}`)
	if err := store.Put("stripe", "0.1.0", payload, 24); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, meta, err := store.Get("stripe", "0.1.0")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if !bytes.Equal(payload, got) {
		t.Fatalf("payload mismatch: got %q, want %q", got, payload)
	}

	if meta.Twin != "stripe" || meta.Version != "0.1.0" {
		t.Fatalf("meta mismatch: %+v", meta)
	}

	if meta.IsExpired() {
		t.Fatal("cache should not be expired yet")
	}
}

func TestIsValid(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "test-key")

	if store.IsValid("stripe", "0.1.0") {
		t.Fatal("should not be valid before Put")
	}

	if err := store.Put("stripe", "0.1.0", []byte("data"), 24); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if !store.IsValid("stripe", "0.1.0") {
		t.Fatal("should be valid after Put")
	}
}

func TestExpiredCache(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "test-key")

	if err := store.Put("stripe", "0.1.0", []byte("data"), 24); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Manually expire the cache by rewriting meta
	meta, _ := ReadMeta(store.metaPath("stripe", "0.1.0"))
	meta.ExpiresAt = time.Now().Add(-1 * time.Hour)
	WriteMeta(store.metaPath("stripe", "0.1.0"), meta)

	// Get should still return stale data
	got, retMeta, err := store.Get("stripe", "0.1.0")
	if err != nil {
		t.Fatalf("Get should succeed for expired cache: %v", err)
	}
	if got == nil {
		t.Fatal("expected stale payload")
	}
	if !retMeta.IsExpired() {
		t.Fatal("meta should report expired")
	}

	if store.IsValid("stripe", "0.1.0") {
		t.Fatal("IsValid should return false for expired cache")
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "test-key")

	store.Put("stripe", "0.1.0", []byte("a"), 24)
	store.Put("stripe", "0.2.0", []byte("b"), 24)
	store.Put("twilio", "0.1.0", []byte("c"), 24)

	// Clear specific version
	store.Clear("stripe", "0.1.0")
	if store.IsValid("stripe", "0.1.0") {
		t.Fatal("stripe 0.1.0 should be cleared")
	}
	if !store.IsValid("stripe", "0.2.0") {
		t.Fatal("stripe 0.2.0 should still exist")
	}

	// Clear all versions of a twin
	store.Clear("stripe", "")
	if store.IsValid("stripe", "0.2.0") {
		t.Fatal("stripe 0.2.0 should be cleared")
	}
	if !store.IsValid("twilio", "0.1.0") {
		t.Fatal("twilio should still exist")
	}

	// Clear all
	store.Clear("", "")
	if store.IsValid("twilio", "0.1.0") {
		t.Fatal("everything should be cleared")
	}
}
