package main

import (
	"fmt"
	"testing"
)

func TestIsValidLicenseKey(t *testing.T) {
	// Generate a valid key: wt_com_acme_{random}_{check}
	// payload = "wt_com_acme_abcdef" => sum mod 256 as hex
	validKey := makeTestKey("com", "acme", "abcdef")

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"valid key", validKey, true},
		{"empty", "", false},
		{"too few parts", "wt_com_acme", false},
		{"bad prefix", "xx_com_acme_abcdef_00", false},
		{"bad tier", "wt_xxx_acme_abcdef_00", false},
		{"empty org", "wt_com__abcdef_00", false},
		{"short random", "wt_com_acme_abc_00", false},
		{"bad checksum", "wt_com_acme_abcdef_ff", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidLicenseKey(tt.key)
			if got != tt.want {
				t.Errorf("isValidLicenseKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestIsValidLicenseKeyEnterprise(t *testing.T) {
	key := makeTestKey("ent", "bigcorp", "xyzxyzxyz")
	if !isValidLicenseKey(key) {
		t.Errorf("expected enterprise key %q to be valid", key)
	}
}

// makeTestKey generates a structurally valid license key.
func makeTestKey(tier, org, random string) string {
	payload := "wt_" + tier + "_" + org + "_" + random
	var sum byte
	for _, b := range []byte(payload) {
		sum += b
	}
	return payload + "_" + fmt.Sprintf("%02x", sum)
}
