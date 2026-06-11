package main

import (
	"strings"
	"testing"
)

func TestValidateIngestKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"empty", "", true},
		{"too-short-31", strings.Repeat("a", 31), true},
		{"exact-32", strings.Repeat("a", 32), false},
		{"long-64", strings.Repeat("a", 64), false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateIngestKey(tc.key)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateIngestKey(len=%d) err=%v wantErr=%v", len(tc.key), err, tc.wantErr)
			}
		})
	}
}
