package api

import "testing"

// TestSvixSignature checks the signature against Svix's published example
// (docs.svix.com, verifying payloads manually).
func TestSvixSignature(t *testing.T) {
	got := svixSignature("whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", "msg_p5jXN8AQM9LWM0D4loKWxJek", "1614265330",
		[]byte(`{"test": 2432232314}`))
	if want := "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="; got != want {
		t.Errorf("signature %s, want %s", got, want)
	}
}
