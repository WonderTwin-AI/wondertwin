// Package registry fetches the WonderTwin twin registry and resolves
// twin versions for download.
package registry

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/wondertwin-ai/wondertwin/internal/httpclient"
	"github.com/wondertwin-ai/wondertwin/internal/httpio"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultRegistryURL is the canonical location of the WonderTwin registry.
// FetchRegistry will try the JSON variant first and fall back to YAML.
const DefaultRegistryURL = "https://raw.githubusercontent.com/wondertwin-ai/registry/main/registry.yaml"

// CurrentSchemaVersion is the schema version emitted by gen-registry
// going forward. Parsers accept any schema_version <= CurrentSchemaVersion.
// Schema v1 entries continue to parse cleanly because all v2 fields are
// declared with `omitempty`.
const CurrentSchemaVersion = 2

// Registry represents the top-level registry manifest.
type Registry struct {
	SchemaVersion int                  `yaml:"schema_version" json:"schema_version"`
	Twins         map[string]TwinEntry `yaml:"twins" json:"twins"`
}

// TwinEntry describes a twin available in the registry.
type TwinEntry struct {
	Description string             `yaml:"description" json:"description"`
	Repo        string             `yaml:"repo" json:"repo"`
	Category    string             `yaml:"category" json:"category"`
	Author      string             `yaml:"author" json:"author"`
	Latest      string             `yaml:"latest" json:"latest"`
	Versions    map[string]Version `yaml:"versions" json:"versions"`
}

// Release-type constants. ReleaseType on a Version is one of these or
// empty; consumers should prefer Version.EffectiveReleaseType, which
// treats an empty value as ReleaseTypeStable.
const (
	ReleaseTypeStable = "stable"
	ReleaseTypeBeta   = "beta"
	ReleaseTypeRC     = "rc"
)

// Maintenance-status constants. MaintenanceStatus on a Version is one
// of these or empty; consumers should prefer
// Version.EffectiveMaintenanceStatus, which treats an empty value as
// MaintenanceActive.
const (
	MaintenanceActive       = "active"
	MaintenanceSecurityOnly = "security_only"
	MaintenanceEOL          = "eol"
)

// Version describes a specific release of a twin.
//
// The lifecycle fields (ReleaseType, MaintenanceStatus, MaintenanceUntil,
// BSLExpiryDate, Changelog, BreakingChanges) are optional schema-v2
// additions. Use the EffectiveX accessors to read them: a v1 entry with
// no lifecycle fields is interpreted as an active stable release with
// no expiry.
type Version struct {
	Released   string            `yaml:"released" json:"released"`
	SDKPackage string            `yaml:"sdk_package" json:"sdk_package"`
	SDKVersion string            `yaml:"sdk_version" json:"sdk_version"`
	APIVersion string            `yaml:"api_version,omitempty" json:"api_version,omitempty"`
	Tier       string            `yaml:"tier" json:"tier"`
	Checksums  map[string]string `yaml:"checksums" json:"checksums"`
	BinaryURLs map[string]string `yaml:"binary_urls" json:"binary_urls"`

	// ReleaseType identifies the release stream: stable, beta, or rc.
	// Empty values default to stable via EffectiveReleaseType.
	ReleaseType string `yaml:"release_type,omitempty" json:"release_type,omitempty"`

	// MaintenanceStatus is the current maintenance commitment: active,
	// security_only, or eol. Empty values default to active via
	// EffectiveMaintenanceStatus.
	MaintenanceStatus string `yaml:"maintenance_status,omitempty" json:"maintenance_status,omitempty"`

	// MaintenanceUntil is an ISO 8601 date marking when maintenance
	// ends. Parse via MaintenanceUntilTime.
	MaintenanceUntil string `yaml:"maintenance_until,omitempty" json:"maintenance_until,omitempty"`

	// BSLExpiryDate is the ISO 8601 date a BSL-licensed pro twin
	// converts to MIT. Only meaningful for commercial-tier versions.
	// Parse via BSLExpiryTime.
	BSLExpiryDate string `yaml:"bsl_expiry_date,omitempty" json:"bsl_expiry_date,omitempty"`

	// Changelog is human-readable release notes for this version,
	// typically markdown. Optional.
	Changelog string `yaml:"changelog,omitempty" json:"changelog,omitempty"`

	// BreakingChanges enumerates breaking changes introduced in this
	// version. Each entry should be a complete, non-empty sentence.
	BreakingChanges []string `yaml:"breaking_changes,omitempty" json:"breaking_changes,omitempty"`
}

// EffectiveReleaseType returns ReleaseType when set, else
// ReleaseTypeStable. Use this in consumer code so v1 entries (which
// lack the field) are treated as stable releases.
func (v Version) EffectiveReleaseType() string {
	if v.ReleaseType == "" {
		return ReleaseTypeStable
	}
	return v.ReleaseType
}

// EffectiveMaintenanceStatus returns MaintenanceStatus when set, else
// MaintenanceActive. A v1 registry entry with no lifecycle fields is
// treated as an active release.
func (v Version) EffectiveMaintenanceStatus() string {
	if v.MaintenanceStatus == "" {
		return MaintenanceActive
	}
	return v.MaintenanceStatus
}

// IsActive reports whether the version is currently maintained — that
// is, its EffectiveMaintenanceStatus is active or security_only.
func (v Version) IsActive() bool {
	switch v.EffectiveMaintenanceStatus() {
	case MaintenanceActive, MaintenanceSecurityOnly:
		return true
	}
	return false
}

// IsEOL reports whether the version has reached end-of-life.
func (v Version) IsEOL() bool {
	return v.EffectiveMaintenanceStatus() == MaintenanceEOL
}

// MaintenanceUntilTime parses MaintenanceUntil into a time.Time.
// Returns ok=false when the field is empty or unparseable.
func (v Version) MaintenanceUntilTime() (time.Time, bool) {
	return parseISODate(v.MaintenanceUntil)
}

// BSLExpiryTime parses BSLExpiryDate into a time.Time. Returns
// ok=false when the field is empty or unparseable.
func (v Version) BSLExpiryTime() (time.Time, bool) {
	return parseISODate(v.BSLExpiryDate)
}

func parseISODate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// FetchRegistry downloads and parses the registry from the given URL.
// If the URL ends in .yaml or .yml, it tries a .json variant first and
// falls back to the original URL. If token is non-empty, it is sent as
// a Bearer authorization header.
func FetchRegistry(url, token string) (*Registry, error) {
	// Try JSON variant first if URL points to a YAML file
	jsonURL := toJSONURL(url)
	if jsonURL != "" {
		reg, err := fetchAndParse(jsonURL, token)
		if err == nil {
			return reg, nil
		}
		// Only fall back to YAML on HTTP errors (e.g. 404).
		// If JSON was fetched successfully but failed to parse,
		// that's a real error — don't silently swallow it.
		if !isHTTPError(err) {
			return nil, err
		}
	}

	return fetchAndParse(url, token)
}

// httpError is returned when the registry server responds with a non-200 status.
type httpError struct {
	statusCode int
}

func (e *httpError) Error() string {
	return fmt.Sprintf("registry returned HTTP %d", e.statusCode)
}

// isHTTPError returns true if the error is an HTTP-level error (non-200 status).
func isHTTPError(err error) bool {
	_, ok := err.(*httpError)
	return ok
}

// toJSONURL converts a .yaml/.yml URL to its .json equivalent.
// Returns empty string if the URL does not end in .yaml or .yml.
func toJSONURL(url string) string {
	if strings.HasSuffix(url, ".yaml") {
		return strings.TrimSuffix(url, ".yaml") + ".json"
	}
	if strings.HasSuffix(url, ".yml") {
		return strings.TrimSuffix(url, ".yml") + ".json"
	}
	return ""
}

// fetchAndParse downloads a registry file and parses it based on the URL extension.
// JSON is used for .json URLs, YAML for everything else.
func fetchAndParse(url, token string) (*Registry, error) {
	client := httpclient.New(httpclient.WithTimeout(30 * time.Second))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating registry request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{statusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, httpio.MaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading registry response: %w", err)
	}

	var reg Registry
	if strings.HasSuffix(url, ".json") {
		if err := json.Unmarshal(body, &reg); err != nil {
			return nil, fmt.Errorf("parsing registry JSON: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(body, &reg); err != nil {
			return nil, fmt.Errorf("parsing registry YAML: %w", err)
		}
	}

	if reg.SchemaVersion > CurrentSchemaVersion {
		return nil, fmt.Errorf("registry schema version %d not supported (max %d)", reg.SchemaVersion, CurrentSchemaVersion)
	}

	if reg.Twins == nil {
		return nil, fmt.Errorf("registry contains no twins")
	}

	return &reg, nil
}

// ResolveVersion looks up a twin in the registry and resolves the version spec.
// The versionSpec may be:
//   - "latest" — resolves to the entry's latest version
//   - "0.4.0"  — exact match
//   - "sdk:github.com/stripe/stripe-go/v76" — newest version targeting this SDK package
func (r *Registry) ResolveVersion(twinName, versionSpec string) (string, Version, error) {
	entry, ok := r.Twins[twinName]
	if !ok {
		return "", Version{}, fmt.Errorf("twin %q not found in registry", twinName)
	}

	// SDK resolution: find newest version matching an SDK package
	if strings.HasPrefix(versionSpec, "sdk:") {
		sdkPackage := strings.TrimPrefix(versionSpec, "sdk:")
		return resolveBySDK(twinName, entry, sdkPackage)
	}

	resolvedVersion := versionSpec
	if versionSpec == "latest" || versionSpec == "" {
		if entry.Latest == "" {
			return "", Version{}, fmt.Errorf("twin %q has no latest version defined", twinName)
		}
		resolvedVersion = entry.Latest
	}

	ver, ok := entry.Versions[resolvedVersion]
	if !ok {
		return "", Version{}, fmt.Errorf("twin %q version %q not found in registry", twinName, resolvedVersion)
	}

	return resolvedVersion, ver, nil
}

// resolveBySDK finds the newest version of a twin targeting the given SDK package.
func resolveBySDK(twinName string, entry TwinEntry, sdkPackage string) (string, Version, error) {
	var bestVersion string
	var bestVer Version

	for v, ver := range entry.Versions {
		if ver.SDKPackage != sdkPackage {
			continue
		}
		if bestVersion == "" || compareSemver(v, bestVersion) > 0 {
			bestVersion = v
			bestVer = ver
		}
	}

	if bestVersion == "" {
		return "", Version{}, fmt.Errorf("twin %q has no version targeting SDK %q", twinName, sdkPackage)
	}

	return bestVersion, bestVer, nil
}

// compareSemver does a simple lexicographic comparison of dotted version strings.
// Returns >0 if a > b, <0 if a < b, 0 if equal.
func compareSemver(a, b string) int {
	aParts := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bParts := strings.Split(strings.TrimPrefix(b, "v"), ".")

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var aNum, bNum int
		if i < len(aParts) {
			fmt.Sscanf(aParts[i], "%d", &aNum)
		}
		if i < len(bParts) {
			fmt.Sscanf(bParts[i], "%d", &bNum)
		}
		if aNum != bNum {
			return aNum - bNum
		}
	}
	return 0
}
