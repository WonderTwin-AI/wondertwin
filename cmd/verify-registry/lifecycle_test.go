package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func registryWith(v versionDef) registrySchema {
	r := validRegistry()
	cur := r.Twins["stripe"].Versions["0.1.0"]
	v.Released = cur.Released
	v.SDKPackage = cur.SDKPackage
	v.SDKVersion = cur.SDKVersion
	v.Tier = cur.Tier
	v.Checksums = cur.Checksums
	v.BinaryURLs = cur.BinaryURLs
	r.Twins["stripe"].Versions["0.1.0"] = v
	return r
}

func runRegistry(t *testing.T, reg registrySchema) []checkResult {
	t.Helper()
	binaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(binaryServer.Close)
	v := reg.Twins["stripe"].Versions["0.1.0"]
	for _, p := range requiredPlatforms {
		v.BinaryURLs[p] = binaryServer.URL + "/" + p
	}
	reg.Twins["stripe"].Versions["0.1.0"] = v

	data, _ := json.Marshal(reg)
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(data)
	}))
	t.Cleanup(regServer.Close)
	return run(regServer.URL)
}

func failedCheck(results []checkResult, name string) *checkResult {
	for i := range results {
		if results[i].Name == name && !results[i].Passed {
			return &results[i]
		}
	}
	return nil
}

func TestVerifyAcceptsV1WithoutLifecycle(t *testing.T) {
	results := runRegistry(t, validRegistry())
	for _, r := range results {
		if !r.Passed {
			t.Errorf("v1 fixture should pass cleanly; %s failed: %s", r.Name, r.Detail)
		}
	}
}

func TestVerifyAcceptsValidV2(t *testing.T) {
	reg := registryWith(versionDef{
		ReleaseType:       "beta",
		MaintenanceStatus: "security_only",
		MaintenanceUntil:  "2027-01-01",
		BreakingChanges:   []string{"removed legacy auth"},
	})
	reg.SchemaVersion = 2
	results := runRegistry(t, reg)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("valid v2 fixture should pass cleanly; %s failed: %s", r.Name, r.Detail)
		}
	}
}

func TestVerifyRejectsInvalidReleaseType(t *testing.T) {
	reg := registryWith(versionDef{ReleaseType: "experimental"})
	results := runRegistry(t, reg)
	if failedCheck(results, "[stripe@0.1.0] release_type") == nil {
		t.Error("expected release_type check to fail")
	}
}

func TestVerifyRejectsInvalidMaintenanceStatus(t *testing.T) {
	reg := registryWith(versionDef{MaintenanceStatus: "yolo"})
	results := runRegistry(t, reg)
	if failedCheck(results, "[stripe@0.1.0] maintenance_status") == nil {
		t.Error("expected maintenance_status check to fail")
	}
}

func TestVerifyRejectsInvalidDate(t *testing.T) {
	reg := registryWith(versionDef{MaintenanceUntil: "next-week"})
	results := runRegistry(t, reg)
	if failedCheck(results, "[stripe@0.1.0] maintenance_until") == nil {
		t.Error("expected maintenance_until check to fail")
	}
}

func TestVerifyRejectsBSLOnFreeTier(t *testing.T) {
	reg := registryWith(versionDef{BSLExpiryDate: "2030-01-01"})
	v := reg.Twins["stripe"].Versions["0.1.0"]
	v.Tier = "community"
	reg.Twins["stripe"].Versions["0.1.0"] = v
	results := runRegistry(t, reg)
	if failedCheck(results, "[stripe@0.1.0] bsl_expiry_date") == nil {
		t.Error("expected bsl_expiry_date check to fail on community tier")
	}
}

func TestVerifyRejectsEmptyBreakingChange(t *testing.T) {
	reg := registryWith(versionDef{BreakingChanges: []string{"   "}})
	results := runRegistry(t, reg)
	if failedCheck(results, "[stripe@0.1.0] breaking_changes[0]") == nil {
		t.Error("expected breaking_changes[0] check to fail on whitespace")
	}
}

func TestVerifyRejectsSchemaV3(t *testing.T) {
	reg := validRegistry()
	reg.SchemaVersion = 3
	results := runRegistry(t, reg)
	if failedCheck(results, "Schema version") == nil {
		t.Error("expected schema version check to fail for v3")
	}
}
