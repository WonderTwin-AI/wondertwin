// Command gen-registry updates a registry.json file with a single twin release.
// It is called by CI after GoReleaser produces binaries and checksums.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wondertwin-ai/wondertwin/internal/registry"
)

// Registry mirrors internal/registry.Registry for JSON serialisation.
type Registry struct {
	SchemaVersion int                  `json:"schema_version"`
	Twins         map[string]TwinEntry `json:"twins"`
}

// TwinEntry mirrors internal/registry.TwinEntry.
type TwinEntry struct {
	Description  string             `json:"description"`
	Repo         string             `json:"repo"`
	Category     string             `json:"category"`
	Author       string             `json:"author"`
	Tier         string             `json:"tier"`
	DownloadAuth string             `json:"download_auth,omitempty"`
	Latest       string             `json:"latest"`
	Versions     map[string]Version `json:"versions"`
}

// Version mirrors internal/registry.Version, including schema-v2
// lifecycle fields. All lifecycle fields are omitempty so emitted
// entries remain compatible with v1 consumers.
type Version struct {
	Released          string            `json:"released"`
	SDKPackage        string            `json:"sdk_package"`
	SDKVersion        string            `json:"sdk_version"`
	APIVersion        string            `json:"api_version,omitempty"`
	Tier              string            `json:"tier"`
	Checksums         map[string]string `json:"checksums"`
	BinaryURLs        map[string]string `json:"binary_urls"`
	ReleaseType       string            `json:"release_type,omitempty"`
	MaintenanceStatus string            `json:"maintenance_status,omitempty"`
	MaintenanceUntil  string            `json:"maintenance_until,omitempty"`
	BSLExpiryDate     string            `json:"bsl_expiry_date,omitempty"`
	Changelog         string            `json:"changelog,omitempty"`
	BreakingChanges   []string          `json:"breaking_changes,omitempty"`
}

// stringSliceFlag accumulates repeated --breaking-change values.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error { *s = append(*s, v); return nil }

// TwinManifest represents the relevant fields from twin-manifest.json.
//
// The Lifecycle block is optional; when present its values seed the
// emitted Version's lifecycle metadata. CLI flags override anything
// supplied here.
type TwinManifest struct {
	Twin        string `json:"twin"`
	Description string `json:"description"`
	Category    string `json:"category"`
	SDKTarget   struct {
		Primary struct {
			Package    string `json:"package"`
			Version    string `json:"version"`
			APIVersion string `json:"api_version"`
		} `json:"primary"`
	} `json:"sdk_target"`
	Lifecycle *struct {
		ReleaseType       string   `json:"release_type"`
		MaintenanceStatus string   `json:"maintenance_status"`
		MaintenanceUntil  string   `json:"maintenance_until"`
		BSLExpiryDate     string   `json:"bsl_expiry_date"`
		Changelog         string   `json:"changelog"`
		BreakingChanges   []string `json:"breaking_changes"`
	} `json:"lifecycle,omitempty"`
}

// lifecycleInputs aggregates resolved lifecycle metadata from manifest
// + CLI flags after validation.
type lifecycleInputs struct {
	releaseType       string
	maintenanceStatus string
	maintenanceUntil  string
	bslExpiryDate     string
	changelog         string
	breakingChanges   []string
}

// nowFunc is overridden in tests to produce deterministic dates.
var nowFunc = time.Now

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gen-registry: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("gen-registry", flag.ContinueOnError)
	twin := fs.String("twin", "", "twin name (e.g. stripe)")
	version := fs.String("version", "", "version string (e.g. 0.1.0)")
	checksumsFile := fs.String("checksums-file", "", "path to checksums file")
	registryFile := fs.String("registry-file", "", "path to registry.json")
	repo := fs.String("repo", "wondertwin-ai/registry", "GitHub repo for download URLs")
	prerelease := fs.Bool("prerelease", false, "add version without updating latest")
	tier := fs.String("tier", "community", "twin tier: community or commercial")
	manifestFile := fs.String("manifest-file", "", "explicit path to twin-manifest.json (default: twin-<name>/twin-manifest.json)")

	releaseType := fs.String("release-type", "", "release stream: stable, beta, or rc (overrides manifest)")
	maintenanceStatus := fs.String("maintenance-status", "", "maintenance status: active, security_only, or eol (overrides manifest)")
	maintenanceUntil := fs.String("maintenance-until", "", "maintenance end date in ISO 8601 (YYYY-MM-DD) (overrides manifest)")
	bslExpiryDate := fs.String("bsl-expiry-date", "", "BSL expiry date in ISO 8601 (commercial tier only) (overrides manifest)")
	changelogFlag := fs.String("changelog", "", "changelog text or @path/to/file (overrides manifest)")
	var breakingChanges stringSliceFlag
	fs.Var(&breakingChanges, "breaking-change", "breaking change description; may be repeated (overrides manifest)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *twin == "" || *version == "" || *checksumsFile == "" || *registryFile == "" {
		return fmt.Errorf("--twin, --version, --checksums-file, and --registry-file are all required")
	}

	if *tier != "community" && *tier != "commercial" {
		return fmt.Errorf("--tier must be 'community' or 'commercial', got %q", *tier)
	}

	// 1. Read twin manifest
	manifest, err := readManifestFrom(*twin, *manifestFile)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	// 2. Parse checksums
	checksums, err := parseChecksums(*checksumsFile, *twin)
	if err != nil {
		return fmt.Errorf("parsing checksums: %w", err)
	}

	// 3. Load existing registry
	reg, err := loadRegistry(*registryFile)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	// 4. Resolve lifecycle metadata from manifest + flag overrides.
	lc, err := resolveLifecycle(manifest, *tier, lifecycleFlags{
		releaseType:       *releaseType,
		maintenanceStatus: *maintenanceStatus,
		maintenanceUntil:  *maintenanceUntil,
		bslExpiryDate:     *bslExpiryDate,
		changelog:         *changelogFlag,
		breakingChanges:   []string(breakingChanges),
	})
	if err != nil {
		return fmt.Errorf("invalid lifecycle metadata: %w", err)
	}

	// 5. Build version entry
	ver := buildVersion(*twin, *version, *repo, *tier, manifest, checksums, lc)

	// 6. Upsert into registry
	upsert(reg, *twin, *version, *tier, manifest, ver, *prerelease)

	// 6. Write back
	if err := writeRegistry(*registryFile, reg); err != nil {
		return fmt.Errorf("writing registry: %w", err)
	}

	fmt.Printf("Updated registry: %s v%s (%d platforms)\n", *twin, *version, len(checksums))
	return nil
}

func readManifestFrom(twin, manifestFile string) (*TwinManifest, error) {
	path := manifestFile
	if path == "" {
		path = filepath.Join(fmt.Sprintf("twin-%s", twin), "twin-manifest.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m TwinManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &m, nil
}

// parseChecksums reads a checksums file in `<sha256hex>  <filename>` format.
// It extracts the platform from filenames matching twin-{name}-{os}-{arch}.
func parseChecksums(path, twin string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	prefix := fmt.Sprintf("twin-%s-", twin)
	checksums := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Format: <hex>  <filename>  (two spaces between)
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			// Also try single space (some tools use one space)
			parts = strings.SplitN(line, " ", 2)
			if len(parts) != 2 {
				continue
			}
		}
		hex := strings.TrimSpace(parts[0])
		filename := strings.TrimSpace(parts[1])

		// Extract platform from filename
		if !strings.HasPrefix(filename, prefix) {
			continue
		}
		platform := strings.TrimPrefix(filename, prefix)
		checksums[platform] = fmt.Sprintf("sha256:%s", hex)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(checksums) == 0 {
		return nil, fmt.Errorf("no checksums found for twin %q", twin)
	}
	return checksums, nil
}

func loadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing registry: %w", err)
	}
	if reg.Twins == nil {
		reg.Twins = make(map[string]TwinEntry)
	}
	return &reg, nil
}

func buildVersion(twin, version, repo, tier string, manifest *TwinManifest, checksums map[string]string, lc lifecycleInputs) Version {
	platforms := []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"}
	binaryURLs := make(map[string]string, len(platforms))
	for _, p := range platforms {
		binaryURLs[p] = fmt.Sprintf(
			"https://github.com/%s/releases/download/twin-%s-v%s/twin-%s-%s",
			repo, twin, version, twin, p,
		)
	}

	return Version{
		Released:          nowFunc().UTC().Format("2006-01-02"),
		SDKPackage:        manifest.SDKTarget.Primary.Package,
		SDKVersion:        manifest.SDKTarget.Primary.Version,
		APIVersion:        manifest.SDKTarget.Primary.APIVersion,
		Tier:              tier,
		Checksums:         checksums,
		BinaryURLs:        binaryURLs,
		ReleaseType:       lc.releaseType,
		MaintenanceStatus: lc.maintenanceStatus,
		MaintenanceUntil:  lc.maintenanceUntil,
		BSLExpiryDate:     lc.bslExpiryDate,
		Changelog:         lc.changelog,
		BreakingChanges:   lc.breakingChanges,
	}
}

// lifecycleFlags carries CLI-supplied lifecycle overrides into
// resolveLifecycle.
type lifecycleFlags struct {
	releaseType       string
	maintenanceStatus string
	maintenanceUntil  string
	bslExpiryDate     string
	changelog         string
	breakingChanges   []string
}

// resolveLifecycle reconciles manifest-supplied lifecycle metadata with
// CLI overrides and validates the result. Manifest values are used
// where flags are absent; flag values override manifest values when
// non-empty.
func resolveLifecycle(manifest *TwinManifest, tier string, flags lifecycleFlags) (lifecycleInputs, error) {
	var lc lifecycleInputs
	if manifest != nil && manifest.Lifecycle != nil {
		lc.releaseType = manifest.Lifecycle.ReleaseType
		lc.maintenanceStatus = manifest.Lifecycle.MaintenanceStatus
		lc.maintenanceUntil = manifest.Lifecycle.MaintenanceUntil
		lc.bslExpiryDate = manifest.Lifecycle.BSLExpiryDate
		lc.changelog = manifest.Lifecycle.Changelog
		lc.breakingChanges = append([]string(nil), manifest.Lifecycle.BreakingChanges...)
	}
	if flags.releaseType != "" {
		lc.releaseType = flags.releaseType
	}
	if flags.maintenanceStatus != "" {
		lc.maintenanceStatus = flags.maintenanceStatus
	}
	if flags.maintenanceUntil != "" {
		lc.maintenanceUntil = flags.maintenanceUntil
	}
	if flags.bslExpiryDate != "" {
		lc.bslExpiryDate = flags.bslExpiryDate
	}
	if flags.changelog != "" {
		resolved, err := loadChangelog(flags.changelog)
		if err != nil {
			return lifecycleInputs{}, err
		}
		lc.changelog = resolved
	}
	if len(flags.breakingChanges) > 0 {
		lc.breakingChanges = append([]string(nil), flags.breakingChanges...)
	}

	if err := validateLifecycle(lc, tier); err != nil {
		return lifecycleInputs{}, err
	}
	return lc, nil
}

func validateLifecycle(lc lifecycleInputs, tier string) error {
	switch lc.releaseType {
	case "", registry.ReleaseTypeStable, registry.ReleaseTypeBeta, registry.ReleaseTypeRC:
	default:
		return fmt.Errorf("release_type %q must be one of stable, beta, rc", lc.releaseType)
	}
	switch lc.maintenanceStatus {
	case "", registry.MaintenanceActive, registry.MaintenanceSecurityOnly, registry.MaintenanceEOL:
	default:
		return fmt.Errorf("maintenance_status %q must be one of active, security_only, eol", lc.maintenanceStatus)
	}
	if lc.maintenanceUntil != "" {
		if _, err := time.Parse("2006-01-02", lc.maintenanceUntil); err != nil {
			return fmt.Errorf("maintenance_until %q is not ISO 8601 (YYYY-MM-DD)", lc.maintenanceUntil)
		}
	}
	if lc.bslExpiryDate != "" {
		if _, err := time.Parse("2006-01-02", lc.bslExpiryDate); err != nil {
			return fmt.Errorf("bsl_expiry_date %q is not ISO 8601 (YYYY-MM-DD)", lc.bslExpiryDate)
		}
		if tier != "commercial" {
			return fmt.Errorf("bsl_expiry_date is only meaningful for commercial-tier twins (got tier=%q)", tier)
		}
	}
	for i, bc := range lc.breakingChanges {
		if strings.TrimSpace(bc) == "" {
			return fmt.Errorf("breaking_changes[%d] is empty", i)
		}
	}
	return nil
}

// loadChangelog reads a changelog string. A leading "@" treats the rest
// of the value as a path to read from.
func loadChangelog(s string) (string, error) {
	if !strings.HasPrefix(s, "@") {
		return s, nil
	}
	path := strings.TrimPrefix(s, "@")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading changelog file: %w", err)
	}
	return string(data), nil
}

func upsert(reg *Registry, twin, version, tier string, manifest *TwinManifest, ver Version, prerelease bool) {
	entry, exists := reg.Twins[twin]
	if !exists {
		repo := "https://github.com/wondertwin-ai/wondertwin"
		if tier == "commercial" {
			repo = "https://github.com/wondertwin-ai/wondertwin-pro"
		}
		entry = TwinEntry{
			Description: manifest.Description,
			Repo:        repo,
			Category:    manifest.Category,
			Author:      "WonderTwin",
			Versions:    make(map[string]Version),
		}
	}

	// Refresh manifest-derived fields on every release, not just on
	// entry creation. A twin's description and category track what it
	// actually covers, and both drift as the twin grows — twin-stripe
	// went from 6 resources to 44 while its registry description still
	// advertised the Connect-and-Payouts surface it shipped with at
	// v0.1.0. Leaving these write-once meant the registry, and the
	// /docs/twins pages generated from it, described the first release
	// forever.
	entry.Description = manifest.Description
	entry.Category = manifest.Category

	entry.Tier = tier
	if tier == "commercial" {
		entry.DownloadAuth = "required"
	}

	// Update latest unless --prerelease is set.
	// Exception: if there's no previous latest (first release), set it even for prereleases.
	if !prerelease || entry.Latest == "" {
		entry.Latest = version
	}
	entry.Versions[version] = ver
	reg.Twins[twin] = entry
}

func writeRegistry(path string, reg *Registry) error {
	reg.SchemaVersion = registry.CurrentSchemaVersion
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	//nolint:gosec // G306: registry.json is the public, published index — version,
	// checksum and URL metadata only — and release-pipeline steps that serve or
	// upload it often run as a different user than the one that generated it.
	return os.WriteFile(path, data, 0o644)
}
