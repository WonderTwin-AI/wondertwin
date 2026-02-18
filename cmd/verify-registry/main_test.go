package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func validRegistry() registrySchema {
	return registrySchema{
		SchemaVersion: 1,
		Twins: map[string]twinEntry{
			"stripe": {
				Description: "Stripe twin",
				Latest:      "0.1.0",
				Versions: map[string]versionDef{
					"0.1.0": {
						Released:   "2026-02-01",
						SDKPackage: "github.com/stripe/stripe-go",
						SDKVersion: "81.0.0",
						Checksums: map[string]string{
							"darwin-amd64": "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
							"darwin-arm64": "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
							"linux-amd64":  "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
							"linux-arm64":  "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
						},
						BinaryURLs: map[string]string{
							"darwin-amd64": "", // replaced per test
							"darwin-arm64": "",
							"linux-amd64":  "",
							"linux-arm64":  "",
						},
					},
				},
			},
		},
	}
}

func TestValidChecksum(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", true},
		{"sha256:0000000000000000000000000000000000000000000000000000000000000000", true},
		{"sha256:short", false},
		{"md5:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", false},
		{"sha256:ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789", false}, // uppercase
		{"", false},
		{"sha256:", false},
	}
	for _, tc := range tests {
		if got := ValidChecksum(tc.input); got != tc.want {
			t.Errorf("ValidChecksum(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestRunFullValidRegistry(t *testing.T) {
	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer binaryServer.Close()

	reg := validRegistry()
	v := reg.Twins["stripe"].Versions["0.1.0"]
	for _, p := range requiredPlatforms {
		v.BinaryURLs[p] = binaryServer.URL + "/twin-stripe-" + p
	}
	reg.Twins["stripe"].Versions["0.1.0"] = v

	data, _ := json.Marshal(reg)
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer regServer.Close()

	results := run(regServer.URL)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("check %q failed: %s", r.Name, r.Detail)
		}
	}
}

func TestRunLatestMissing(t *testing.T) {
	reg := validRegistry()
	entry := reg.Twins["stripe"]
	entry.Latest = "9.9.9"
	reg.Twins["stripe"] = entry

	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer binaryServer.Close()

	v := reg.Twins["stripe"].Versions["0.1.0"]
	for _, p := range requiredPlatforms {
		v.BinaryURLs[p] = binaryServer.URL + "/" + p
	}
	reg.Twins["stripe"].Versions["0.1.0"] = v

	data, _ := json.Marshal(reg)
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer regServer.Close()

	results := run(regServer.URL)
	foundFail := false
	for _, r := range results {
		if r.Name == "[stripe] latest exists in versions" && !r.Passed {
			foundFail = true
		}
	}
	if !foundFail {
		t.Error("expected latest-exists-in-versions check to fail")
	}
}

func TestRunMissingPlatform(t *testing.T) {
	reg := validRegistry()
	v := reg.Twins["stripe"].Versions["0.1.0"]
	delete(v.BinaryURLs, "linux-arm64")
	delete(v.Checksums, "linux-arm64")
	reg.Twins["stripe"].Versions["0.1.0"] = v

	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer binaryServer.Close()

	for _, p := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64"} {
		v.BinaryURLs[p] = binaryServer.URL + "/" + p
	}
	reg.Twins["stripe"].Versions["0.1.0"] = v

	data, _ := json.Marshal(reg)
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer regServer.Close()

	results := run(regServer.URL)
	foundFail := false
	for _, r := range results {
		if !r.Passed && r.Detail == "missing: linux-arm64" {
			foundFail = true
		}
	}
	if !foundFail {
		t.Error("expected platform check to fail for missing linux-arm64")
	}
}

func TestRunBinaryUnreachable(t *testing.T) {
	reg := validRegistry()
	v := reg.Twins["stripe"].Versions["0.1.0"]

	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer goodServer.Close()

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer badServer.Close()

	v.BinaryURLs["darwin-amd64"] = goodServer.URL + "/ok"
	v.BinaryURLs["darwin-arm64"] = goodServer.URL + "/ok"
	v.BinaryURLs["linux-amd64"] = goodServer.URL + "/ok"
	v.BinaryURLs["linux-arm64"] = badServer.URL + "/missing"
	reg.Twins["stripe"].Versions["0.1.0"] = v

	data, _ := json.Marshal(reg)
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer regServer.Close()

	results := run(regServer.URL)
	foundFail := false
	for _, r := range results {
		if r.Name == "[stripe@0.1.0] reachable linux-arm64" && !r.Passed {
			foundFail = true
		}
	}
	if !foundFail {
		t.Error("expected reachability check to fail for linux-arm64")
	}
}

func TestRunBadChecksum(t *testing.T) {
	reg := validRegistry()
	v := reg.Twins["stripe"].Versions["0.1.0"]
	v.Checksums["darwin-amd64"] = "md5:badformat"

	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer binaryServer.Close()

	for _, p := range requiredPlatforms {
		v.BinaryURLs[p] = binaryServer.URL + "/" + p
	}
	reg.Twins["stripe"].Versions["0.1.0"] = v

	data, _ := json.Marshal(reg)
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer regServer.Close()

	results := run(regServer.URL)
	foundFail := false
	for _, r := range results {
		if !r.Passed && r.Name == "[stripe@0.1.0] checksum format darwin-amd64" {
			foundFail = true
		}
	}
	if !foundFail {
		t.Error("expected checksum format check to fail")
	}
}
