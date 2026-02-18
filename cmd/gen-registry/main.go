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
)

// Registry mirrors internal/registry.Registry for JSON serialisation.
type Registry struct {
	SchemaVersion int                  `json:"schema_version"`
	Twins         map[string]TwinEntry `json:"twins"`
}

// TwinEntry mirrors internal/registry.TwinEntry.
type TwinEntry struct {
	Description string             `json:"description"`
	Repo        string             `json:"repo"`
	Category    string             `json:"category"`
	Author      string             `json:"author"`
	Latest      string             `json:"latest"`
	Versions    map[string]Version `json:"versions"`
}

// Version mirrors internal/registry.Version.
type Version struct {
	Released   string            `json:"released"`
	SDKPackage string            `json:"sdk_package"`
	SDKVersion string            `json:"sdk_version"`
	Tier       string            `json:"tier"`
	Checksums  map[string]string `json:"checksums"`
	BinaryURLs map[string]string `json:"binary_urls"`
}

// TwinManifest represents the relevant fields from twin-manifest.json.
type TwinManifest struct {
	Twin        string `json:"twin"`
	Description string `json:"description"`
	Category    string `json:"category"`
	SDKTarget   struct {
		Primary struct {
			Package string `json:"package"`
			Version string `json:"version"`
		} `json:"primary"`
	} `json:"sdk_target"`
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

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *twin == "" || *version == "" || *checksumsFile == "" || *registryFile == "" {
		return fmt.Errorf("--twin, --version, --checksums-file, and --registry-file are all required")
	}

	// 1. Read twin manifest
	manifest, err := readManifest(*twin)
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

	// 4. Build version entry
	ver := buildVersion(*twin, *version, *repo, manifest, checksums)

	// 5. Upsert into registry
	upsert(reg, *twin, *version, manifest, ver)

	// 6. Write back
	if err := writeRegistry(*registryFile, reg); err != nil {
		return fmt.Errorf("writing registry: %w", err)
	}

	fmt.Printf("Updated registry: %s v%s (%d platforms)\n", *twin, *version, len(checksums))
	return nil
}

func readManifest(twin string) (*TwinManifest, error) {
	path := filepath.Join(fmt.Sprintf("twin-%s", twin), "twin-manifest.json")
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

func buildVersion(twin, version, repo string, manifest *TwinManifest, checksums map[string]string) Version {
	platforms := []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"}
	binaryURLs := make(map[string]string, len(platforms))
	for _, p := range platforms {
		binaryURLs[p] = fmt.Sprintf(
			"https://github.com/%s/releases/download/twin-%s-v%s/twin-%s-%s",
			repo, twin, version, twin, p,
		)
	}

	return Version{
		Released:   nowFunc().UTC().Format("2006-01-02"),
		SDKPackage: manifest.SDKTarget.Primary.Package,
		SDKVersion: manifest.SDKTarget.Primary.Version,
		Tier:       "free",
		Checksums:  checksums,
		BinaryURLs: binaryURLs,
	}
}

func upsert(reg *Registry, twin, version string, manifest *TwinManifest, ver Version) {
	entry, exists := reg.Twins[twin]
	if !exists {
		entry = TwinEntry{
			Description: manifest.Description,
			Repo:        "https://github.com/wondertwin-ai/wondertwin",
			Category:    manifest.Category,
			Author:      "WonderTwin",
			Versions:    make(map[string]Version),
		}
	}
	entry.Latest = version
	entry.Versions[version] = ver
	reg.Twins[twin] = entry
}

func writeRegistry(path string, reg *Registry) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
