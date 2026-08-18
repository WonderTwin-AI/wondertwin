// Command validate-schemas validates twin JSON files against their schemas.
//
// It discovers all twin-* directories and checks that twin.json,
// twin-manifest.json, and provenance.json conform to the schemas
// in the schemas/ directory.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type schemaCheck struct {
	file   string // filename to look for in each twin dir
	schema string // schema filename in schemas/
}

var checks = []schemaCheck{
	{"twin.json", "twin.schema.json"},
	{"twin-manifest.json", "twin-manifest.schema.json"},
	{"provenance.json", "provenance.schema.json"},
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	schemasDir := filepath.Join(root, "schemas")

	// Pre-compile all schemas.
	compiled := make(map[string]*jsonschema.Schema, len(checks))
	for _, c := range checks {
		schemaPath := filepath.Join(schemasDir, c.schema)
		sch, err := compileSchema(schemaPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: cannot compile schema %s: %v\n", c.schema, err)
			os.Exit(1)
		}
		compiled[c.schema] = sch
	}

	// Discover twin directories.
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: cannot read directory %s: %v\n", root, err)
		os.Exit(1)
	}

	var twinDirs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "twin-") {
			twinDirs = append(twinDirs, e.Name())
		}
	}

	if len(twinDirs) == 0 {
		fmt.Fprintf(os.Stderr, "FAIL: no twin-* directories found in %s\n", root)
		os.Exit(1)
	}

	var failures []string
	validated := 0

	for _, dir := range twinDirs {
		for _, c := range checks {
			filePath := filepath.Join(root, dir, c.file)

			//nolint:gosec // G703: build-time validator reading a repo-relative path it
			// constructed itself from a fixed dir list.
			data, err := os.ReadFile(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					failures = append(failures, fmt.Sprintf("%s/%s: missing", dir, c.file))
					continue
				}
				failures = append(failures, fmt.Sprintf("%s/%s: read error: %v", dir, c.file, err))
				continue
			}

			var v any
			if err := json.Unmarshal(data, &v); err != nil {
				failures = append(failures, fmt.Sprintf("%s/%s: invalid JSON: %v", dir, c.file, err))
				continue
			}

			if err := compiled[c.schema].Validate(v); err != nil {
				failures = append(failures, fmt.Sprintf("%s/%s: %v", dir, c.file, err))
				continue
			}

			validated++
			fmt.Printf("  ok  %s/%s\n", dir, c.file)
		}
	}

	fmt.Printf("\n%d files validated across %d twins\n", validated, len(twinDirs))

	if len(failures) > 0 {
		fmt.Printf("%d failures:\n", len(failures))
		for _, f := range failures {
			fmt.Printf("  FAIL  %s\n", f)
		}
		os.Exit(1)
	}
}

func compileSchema(path string) (*jsonschema.Schema, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	c := jsonschema.NewCompiler()
	return c.Compile("file://" + absPath)
}
