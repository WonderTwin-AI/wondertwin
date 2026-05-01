package cimode

import "crypto/ed25519"

// embeddedPubKey is the WonderTwin license verification key, embedded
// at build time. The matching private key lives in wondertwin-app and
// is used by the license issuer service.
//
// To rotate: generate a new ed25519 keypair, replace the bytes below,
// and distribute the new private key to the issuer service. Existing
// licenses signed with the old key will fail verification once this
// file is replaced — plan rotation alongside a re-issuance window so
// customers' NotAfter horizons cover the swap.
var embeddedPubKey = ed25519.PublicKey{
	0x1b, 0xef, 0x64, 0xad, 0xc1, 0x0e, 0x26, 0xe3,
	0x61, 0xc1, 0x2d, 0xfa, 0x59, 0xa5, 0x11, 0x5d,
	0xc0, 0x00, 0x20, 0x9b, 0xf6, 0x24, 0x03, 0x0d,
	0x04, 0x33, 0xfe, 0x7a, 0x3a, 0x4f, 0x77, 0xcd,
}
