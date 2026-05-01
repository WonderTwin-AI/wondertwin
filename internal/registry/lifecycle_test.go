package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSchemaV1Cleanly(t *testing.T) {
	body := `{
  "schema_version": 1,
  "twins": {
    "stripe": {
      "description": "Stripe twin",
      "latest": "0.4.0",
      "versions": {
        "0.4.0": {
          "released": "2025-01-15",
          "sdk_package": "github.com/stripe/stripe-go",
          "sdk_version": "v76",
          "tier": "community",
          "checksums": {},
          "binary_urls": {}
        }
      }
    }
  }
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	reg, err := FetchRegistry(srv.URL+"/registry.json", "")
	if err != nil {
		t.Fatalf("FetchRegistry returned error on v1 fixture: %v", err)
	}
	v := reg.Twins["stripe"].Versions["0.4.0"]
	if v.EffectiveReleaseType() != ReleaseTypeStable {
		t.Errorf("EffectiveReleaseType = %q, want stable", v.EffectiveReleaseType())
	}
	if v.EffectiveMaintenanceStatus() != MaintenanceActive {
		t.Errorf("EffectiveMaintenanceStatus = %q, want active", v.EffectiveMaintenanceStatus())
	}
	if !v.IsActive() {
		t.Error("v1 entry should be IsActive")
	}
	if v.IsEOL() {
		t.Error("v1 entry should not be IsEOL")
	}
	if _, ok := v.MaintenanceUntilTime(); ok {
		t.Error("MaintenanceUntilTime should be ok=false for unset field")
	}
}

func TestParseSchemaV2Round(t *testing.T) {
	src := Registry{
		SchemaVersion: 2,
		Twins: map[string]TwinEntry{
			"stripe": {
				Description: "Stripe",
				Latest:      "1.0.0",
				Versions: map[string]Version{
					"1.0.0": {
						Released:          "2026-04-01",
						SDKPackage:        "github.com/stripe/stripe-go",
						SDKVersion:        "v80",
						Tier:              "commercial",
						Checksums:         map[string]string{},
						BinaryURLs:        map[string]string{},
						ReleaseType:       ReleaseTypeBeta,
						MaintenanceStatus: MaintenanceSecurityOnly,
						MaintenanceUntil:  "2027-01-01",
						BSLExpiryDate:     "2030-01-01",
						Changelog:         "initial release",
						BreakingChanges:   []string{"removed legacy auth"},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(src)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	reg, err := FetchRegistry(srv.URL+"/registry.json", "")
	if err != nil {
		t.Fatalf("FetchRegistry returned error on v2 fixture: %v", err)
	}
	v := reg.Twins["stripe"].Versions["1.0.0"]
	if v.EffectiveReleaseType() != ReleaseTypeBeta {
		t.Errorf("ReleaseType = %q, want beta", v.EffectiveReleaseType())
	}
	if v.EffectiveMaintenanceStatus() != MaintenanceSecurityOnly {
		t.Errorf("MaintenanceStatus = %q, want security_only", v.EffectiveMaintenanceStatus())
	}
	if !v.IsActive() {
		t.Error("security_only must count as active")
	}
	if _, ok := v.MaintenanceUntilTime(); !ok {
		t.Error("MaintenanceUntilTime should parse 2027-01-01")
	}
	if _, ok := v.BSLExpiryTime(); !ok {
		t.Error("BSLExpiryTime should parse 2030-01-01")
	}
	if got := len(v.BreakingChanges); got != 1 {
		t.Errorf("BreakingChanges len = %d, want 1", got)
	}
}

func TestParseSchemaV3Rejected(t *testing.T) {
	body := `{"schema_version": 3, "twins": {"x": {"latest": "1.0.0", "versions": {"1.0.0": {}}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	_, err := FetchRegistry(srv.URL+"/registry.json", "")
	if err == nil {
		t.Fatal("expected error for schema_version=3")
	}
	if !strings.Contains(err.Error(), "schema version 3") {
		t.Errorf("error %q should mention schema version 3", err)
	}
}

func TestEOLAndIsActive(t *testing.T) {
	cases := []struct {
		status string
		active bool
		eol    bool
	}{
		{"", true, false},
		{MaintenanceActive, true, false},
		{MaintenanceSecurityOnly, true, false},
		{MaintenanceEOL, false, true},
	}
	for _, tc := range cases {
		v := Version{MaintenanceStatus: tc.status}
		if v.IsActive() != tc.active {
			t.Errorf("IsActive(%q) = %v, want %v", tc.status, v.IsActive(), tc.active)
		}
		if v.IsEOL() != tc.eol {
			t.Errorf("IsEOL(%q) = %v, want %v", tc.status, v.IsEOL(), tc.eol)
		}
	}
}

func TestParseISODateInvalid(t *testing.T) {
	v := Version{MaintenanceUntil: "not-a-date"}
	if _, ok := v.MaintenanceUntilTime(); ok {
		t.Error("MaintenanceUntilTime should be ok=false for unparseable input")
	}
}
