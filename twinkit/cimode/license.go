package cimode

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// License is the WonderTwin license file format. It is signed by the
// issuer's ed25519 private key over the canonical JSON of the license
// with the Signature field omitted; verifiers re-marshal in the same
// shape and check the signature.
//
// The Reason strings emitted by ValidateLicense are stable enums (see
// the Reason* constants in this package). Telemetry payloads and the
// `wt license status` output depend on those values.
type License struct {
	// Key is the opaque license identifier issued to the customer.
	Key string `json:"key"`

	// OrgID is the customer organization identifier. The issuer hashes
	// this before insertion; verifiers treat it as opaque.
	OrgID string `json:"org_id"`

	// TwinScope lists the twin names this license covers. ["*"]
	// authorizes all twins.
	TwinScope []string `json:"twin_scope"`

	// IssuedAt is the issuance timestamp.
	IssuedAt time.Time `json:"issued_at"`

	// NotAfter is the hard expiry timestamp.
	NotAfter time.Time `json:"not_after"`

	// Signature is the base64-standard-encoded ed25519 signature over
	// the canonical JSON of this license with Signature omitted.
	Signature string `json:"signature"`
}

// SigningBytes returns the deterministic JSON encoding of the license
// with the Signature field cleared. Issuers sign these bytes; verifiers
// re-derive them and check the signature against the License.Signature.
func (l License) SigningBytes() ([]byte, error) {
	clone := l
	clone.Signature = ""
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(&clone); err != nil {
		return nil, fmt.Errorf("encode license for signing: %w", err)
	}
	// json.Encoder.Encode appends a trailing newline; drop it so the
	// signing input matches a callers-side bytes.Buffer that doesn't
	// add one.
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

// SignatureBytes decodes License.Signature from base64. Returns an
// error if the field is empty or malformed.
func (l License) SignatureBytes() ([]byte, error) {
	if l.Signature == "" {
		return nil, fmt.Errorf("license signature is empty")
	}
	sig, err := base64.StdEncoding.DecodeString(l.Signature)
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	return sig, nil
}

// SetSignature base64-encodes raw and stores it on the license. Used
// by issuance code and tests.
func (l *License) SetSignature(raw []byte) {
	l.Signature = base64.StdEncoding.EncodeToString(raw)
}
