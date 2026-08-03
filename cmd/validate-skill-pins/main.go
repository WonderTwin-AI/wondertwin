// Command validate-skill-pins checks that skill frontmatter schema pins
// match the $version declared in each schema file.
//
// A MAJOR version mismatch (schema > skill) is a FAIL.
// A MINOR version mismatch (schema > skill) is a WARN.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type skillFrontmatter struct {
	Skill        string            `yaml:"skill"`
	SkillVersion string            `yaml:"skill_version"`
	Schemas      map[string]string `yaml:"schemas"`
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	skillsDir := filepath.Join(root, "skills")
	schemasDir := filepath.Join(root, "schemas")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: cannot read skills directory: %v\n", err)
		os.Exit(1)
	}

	var fails []string
	var warns []string
	checked := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		path := filepath.Join(skillsDir, e.Name())
		fm, err := parseFrontmatter(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: %s: %v\n", e.Name(), err)
			continue
		}

		if len(fm.Schemas) == 0 {
			fmt.Printf("  skip  %s (no schema pins)\n", e.Name())
			continue
		}

		for schemaFile, pinnedVersion := range fm.Schemas {
			schemaPath := filepath.Join(schemasDir, schemaFile)
			schemaVersion, err := extractSchemaVersion(schemaPath)
			if err != nil {
				fails = append(fails, fmt.Sprintf("%s → %s: %v", e.Name(), schemaFile, err))
				continue
			}

			cmp, err := compareVersions(schemaVersion, pinnedVersion)
			if err != nil {
				fails = append(fails, fmt.Sprintf("%s → %s: version parse error: %v", e.Name(), schemaFile, err))
				continue
			}

			checked++
			switch {
			case cmp.majorDrift:
				fails = append(fails, fmt.Sprintf("%s → %s: pinned %s but schema is %s (MAJOR drift)", e.Name(), schemaFile, pinnedVersion, schemaVersion))
			case cmp.minorDrift:
				warns = append(warns, fmt.Sprintf("%s → %s: pinned %s but schema is %s (minor drift)", e.Name(), schemaFile, pinnedVersion, schemaVersion))
				fmt.Printf("  warn  %s → %s: pinned %s, schema %s\n", e.Name(), schemaFile, pinnedVersion, schemaVersion)
			default:
				fmt.Printf("  ok    %s → %s: %s\n", e.Name(), schemaFile, pinnedVersion)
			}
		}
	}

	fmt.Printf("\n%d pins checked\n", checked)

	if len(warns) > 0 {
		fmt.Printf("%d warnings:\n", len(warns))
		for _, w := range warns {
			fmt.Printf("  WARN  %s\n", w)
		}
	}

	if len(fails) > 0 {
		fmt.Printf("%d failures:\n", len(fails))
		for _, f := range fails {
			fmt.Printf("  FAIL  %s\n", f)
		}
		os.Exit(1)
	}
}

// parseFrontmatter extracts YAML frontmatter from a markdown file.
func parseFrontmatter(path string) (skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skillFrontmatter{}, err
	}

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return skillFrontmatter{}, fmt.Errorf("no frontmatter found")
	}

	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return skillFrontmatter{}, fmt.Errorf("unterminated frontmatter")
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(content[4:4+end]), &fm); err != nil {
		return skillFrontmatter{}, fmt.Errorf("invalid frontmatter YAML: %w", err)
	}

	return fm, nil
}

// extractSchemaVersion reads a JSON schema file and returns the $version field.
func extractSchemaVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read schema: %w", err)
	}

	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	v, ok := schema["$version"]
	if !ok {
		return "", fmt.Errorf("no $version field")
	}

	version, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("$version is not a string")
	}

	return version, nil
}

type versionComparison struct {
	majorDrift bool
	minorDrift bool
}

// compareVersions checks if the schema version has drifted from the pinned version.
// Returns majorDrift if schema MAJOR > pin MAJOR, minorDrift if schema MINOR > pin MINOR.
func compareVersions(schemaVersion, pinnedVersion string) (versionComparison, error) {
	schemaMajor, schemaMinor, err := parseVersion(schemaVersion)
	if err != nil {
		return versionComparison{}, fmt.Errorf("schema version %q: %w", schemaVersion, err)
	}

	pinMajor, pinMinor, err := parseVersion(pinnedVersion)
	if err != nil {
		return versionComparison{}, fmt.Errorf("pinned version %q: %w", pinnedVersion, err)
	}

	return versionComparison{
		majorDrift: schemaMajor > pinMajor,
		minorDrift: !(schemaMajor > pinMajor) && schemaMinor > pinMinor,
	}, nil
}

// parseVersion splits "MAJOR.MINOR" into integers.
func parseVersion(v string) (int, int, error) {
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected MAJOR.MINOR format, got %q", v)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid major: %w", err)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minor: %w", err)
	}

	return major, minor, nil
}
