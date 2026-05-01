package cimode

import (
	"crypto/ed25519"
	"sync"
)

var (
	verifyMu     sync.RWMutex
	activeVerify = embeddedPubKey
)

// verifyKey returns the active verification key. Tests use
// setVerificationKey to swap it out.
func verifyKey() ed25519.PublicKey {
	verifyMu.RLock()
	defer verifyMu.RUnlock()
	return activeVerify
}

// setVerificationKey replaces the active verification key with pk and
// returns a closure that restores the previous value.
func setVerificationKey(pk ed25519.PublicKey) (restore func()) {
	verifyMu.Lock()
	prev := activeVerify
	activeVerify = pk
	verifyMu.Unlock()
	return func() {
		verifyMu.Lock()
		activeVerify = prev
		verifyMu.Unlock()
	}
}

// SetVerificationKeyForTest is exposed only for use by tests in
// dependent packages (e.g. twinkit/twincore's license-banner tests
// that must mint a synthetic license without the production private
// key). It MUST NOT be called by production code.
func SetVerificationKeyForTest(pk ed25519.PublicKey) (restore func()) {
	return setVerificationKey(pk)
}

// verifySignature returns true when lic.Signature is a valid ed25519
// signature over lic.SigningBytes() under the active verification
// key. Any error in deriving the signing bytes or decoding the
// signature is treated as an invalid signature (returns false).
func verifySignature(lic License) bool {
	msg, err := lic.SigningBytes()
	if err != nil {
		return false
	}
	sig, err := lic.SignatureBytes()
	if err != nil {
		return false
	}
	return ed25519.Verify(verifyKey(), msg, sig)
}
