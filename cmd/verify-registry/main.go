// Command verify-registry validates the live WonderTwin twin registry.
//
// It fetches registry.json, parses it, and runs a series of checks to
// ensure all entries are well-formed and all binary downloads are reachable.
//
// Usage:
//
//	go run ./cmd/verify-registry
//	go run ./cmd/verify-registry --registry-url https://raw.githubusercontent.com/wondertwin-ai/registry/main/registry.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const defaultRegistryURL = "https://raw.githubusercontent.com/wondertwin-ai/registry/main/registry.json"

var requiredPlatforms = []string{
	"darwin-amd64",
	"darwin-arm64",
	"linux-amd64",
	"linux-arm64",
}

// checksumRe matches the expected format: sha256:<64 hex chars>
var checksumRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// registrySchema mirrors internal/registry types for standalone parsing.
type registrySchema struct {
	SchemaVersion int                  `json:"schema_version"`
	Twins         map[string]twinEntry `json:"twins"`
}

type twinEntry struct {
	Description string                `json:"description"`
	Repo        string                `json:"repo"`
	Category    string                `json:"category"`
	Author      string                `json:"author"`
	Latest      string                `json:"latest"`
	Versions    map[string]versionDef `json:"versions"`
}

type versionDef struct {
	Released   string            `json:"released"`
	SDKPackage string            `json:"sdk_package"`
	SDKVersion string            `json:"sdk_version"`
	Tier       string            `json:"tier"`
	Checksums  map[string]string `json:"checksums"`
	BinaryURLs map[string]string `json:"binary_urls"`
}

// checkResult stores the outcome of a single check.
type checkResult struct {
	Name   string
	Passed bool
	Detail string
}

func main() {
	registryURL := flag.String("registry-url", defaultRegistryURL, "URL of the registry.json to validate")
	flag.Parse()

	results := run(*registryURL)
	printResults(results)

	for _, r := range results {
		if !r.Passed {
			os.Exit(1)
		}
	}
}

// run performs all validation checks and returns the results.
func run(registryURL string) []checkResult {
	var results []checkResult

	// 1. Fetch registry
	body, err := fetchRegistry(registryURL)
	if err != nil {
		return append(results, checkResult{"Fetch registry", false, err.Error()})
	}
	results = append(results, checkResult{"Fetch registry", true, registryURL})

	// 2. Parse JSON
	var reg registrySchema
	if err := json.Unmarshal(body, &reg); err != nil {
		return append(results, checkResult{"Parse JSON", false, err.Error()})
	}
	results = append(results, checkResult{"Parse JSON", true, ""})

	// 3. Schema version
	if reg.SchemaVersion < 1 {
		results = append(results, checkResult{"Schema version", false, fmt.Sprintf("got %d, expected >= 1", reg.SchemaVersion)})
	} else {
		results = append(results, checkResult{"Schema version", true, fmt.Sprintf("%d", reg.SchemaVersion)})
	}

	if len(reg.Twins) == 0 {
		return append(results, checkResult{"Twins present", false, "registry has no twins"})
	}
	results = append(results, checkResult{"Twins present", true, fmt.Sprintf("%d twin(s)", len(reg.Twins))})

	// 4. Per-twin checks
	for name, entry := range reg.Twins {
		results = append(results, validateTwin(name, entry)...)
	}

	return results
}

func validateTwin(name string, entry twinEntry) []checkResult {
	var results []checkResult

	// latest points to existing version
	if entry.Latest == "" {
		results = append(results, checkResult{fmt.Sprintf("[%s] latest defined", name), false, "latest is empty"})
	} else if _, ok := entry.Versions[entry.Latest]; !ok {
		results = append(results, checkResult{fmt.Sprintf("[%s] latest exists in versions", name), false, fmt.Sprintf("latest=%q not found in versions", entry.Latest)})
	} else {
		results = append(results, checkResult{fmt.Sprintf("[%s] latest exists in versions", name), true, entry.Latest})
	}

	for ver, vd := range entry.Versions {
		results = append(results, validateVersion(name, ver, vd)...)
	}

	return results
}

func validateVersion(name, ver string, vd versionDef) []checkResult {
	var results []checkResult
	prefix := fmt.Sprintf("[%s@%s]", name, ver)

	// Platform entries in binary_urls
	results = append(results, checkPlatforms(prefix+" binary_urls", vd.BinaryURLs)...)

	// Platform entries in checksums
	results = append(results, checkPlatforms(prefix+" checksums", vd.Checksums)...)

	// Checksum format
	for platform, cs := range vd.Checksums {
		if !ValidChecksum(cs) {
			results = append(results, checkResult{fmt.Sprintf("%s checksum format %s", prefix, platform), false, cs})
		}
	}

	// Binary URL reachability (HEAD requests)
	for platform, url := range vd.BinaryURLs {
		ok, detail := headCheck(url)
		results = append(results, checkResult{fmt.Sprintf("%s reachable %s", prefix, platform), ok, detail})
	}

	return results
}

func checkPlatforms(label string, m map[string]string) []checkResult {
	var missing []string
	for _, p := range requiredPlatforms {
		if _, ok := m[p]; !ok {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return []checkResult{{label + " platforms", false, "missing: " + strings.Join(missing, ", ")}}
	}
	return []checkResult{{label + " platforms", true, fmt.Sprintf("%d platforms", len(m))}}
}

// ValidChecksum checks whether a checksum string matches sha256:<hex64>.
func ValidChecksum(cs string) bool {
	return checksumRe.MatchString(cs)
}

func headCheck(url string) (bool, string) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Head(url)
	if err != nil {
		return false, err.Error()
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, "200 OK"
	}
	// GitHub releases sometimes redirect HEAD; try GET with range header
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusMethodNotAllowed {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Range", "bytes=0-0")
		resp2, err := client.Do(req)
		if err != nil {
			return false, err.Error()
		}
		resp2.Body.Close()
		if resp2.StatusCode == http.StatusOK || resp2.StatusCode == http.StatusPartialContent {
			return true, fmt.Sprintf("%d (GET range fallback)", resp2.StatusCode)
		}
		return false, fmt.Sprintf("HTTP %d", resp2.StatusCode)
	}
	return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func fetchRegistry(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func printResults(results []checkResult) {
	passed, failed := 0, 0
	for _, r := range results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
			failed++
		} else {
			passed++
		}
		if r.Detail != "" {
			fmt.Printf("  %s  %s — %s\n", status, r.Name, r.Detail)
		} else {
			fmt.Printf("  %s  %s\n", status, r.Name)
		}
	}
	fmt.Printf("\n%d passed, %d failed\n", passed, failed)
}
