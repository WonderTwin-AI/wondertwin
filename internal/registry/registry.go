// Package registry fetches the WonderTwin twin registry and resolves
// twin versions for download.
package registry

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultRegistryURL is the canonical location of the WonderTwin registry.
const DefaultRegistryURL = "https://raw.githubusercontent.com/wondertwin-ai/registry/main/registry.yaml"

// Registry represents the top-level registry manifest.
type Registry struct {
	SchemaVersion int                  `yaml:"schema_version"`
	Twins         map[string]TwinEntry `yaml:"twins"`
}

// TwinEntry describes a twin available in the registry.
type TwinEntry struct {
	Description string             `yaml:"description"`
	Repo        string             `yaml:"repo"`
	Category    string             `yaml:"category"`
	Latest      string             `yaml:"latest"`
	Versions    map[string]Version `yaml:"versions"`
}

// Version describes a specific release of a twin.
type Version struct {
	Released   string            `yaml:"released"`
	Checksums  map[string]string `yaml:"checksums"`
	BinaryURLs map[string]string `yaml:"binary_urls"`
}

// FetchRegistry downloads and parses the registry YAML from the given URL.
func FetchRegistry(url string) (*Registry, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading registry response: %w", err)
	}

	var reg Registry
	if err := yaml.Unmarshal(body, &reg); err != nil {
		return nil, fmt.Errorf("parsing registry YAML: %w", err)
	}

	if reg.Twins == nil {
		return nil, fmt.Errorf("registry contains no twins")
	}

	return &reg, nil
}

// ResolveVersion looks up a twin in the registry and resolves the version spec.
// The versionSpec may be "latest" or an exact version string like "0.3.2".
func (r *Registry) ResolveVersion(twinName, versionSpec string) (string, Version, error) {
	entry, ok := r.Twins[twinName]
	if !ok {
		return "", Version{}, fmt.Errorf("twin %q not found in registry", twinName)
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
