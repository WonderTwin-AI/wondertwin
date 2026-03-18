package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed data/content
var embeddedContent embed.FS

// dataDir is set via the -data-dir flag for development/deployment without recompiling.
var dataDir string

// LoadFixture loads a content fixture for the given twin and version.
// If dataDir is set, it reads from disk; otherwise it uses the embedded FS.
func LoadFixture(twin, version string) ([]byte, error) {
	relPath := filepath.Join("data", "content", twin, version+".json")

	if dataDir != "" {
		diskPath := filepath.Join(dataDir, twin, version+".json")
		data, err := os.ReadFile(diskPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("content not found for %s@%s", twin, version)
			}
			return nil, fmt.Errorf("reading content fixture: %w", err)
		}
		return data, nil
	}

	data, err := embeddedContent.ReadFile(relPath)
	if err != nil {
		return nil, fmt.Errorf("content not found for %s@%s", twin, version)
	}
	return data, nil
}
