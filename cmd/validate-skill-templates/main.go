// Command validate-skill-templates validates JSON template blocks in skill
// markdown files against their declared schemas.
//
// It finds <!-- schema: X.schema.json --> annotations followed by ```json
// code blocks, substitutes {placeholder} values with valid dummy data,
// and validates the result against the referenced schema.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// placeholders maps known placeholder patterns to valid substitution values.
// String placeholders are substituted as-is. The validator also handles
// special cases like {port} which need to become integers.
var placeholders = map[string]string{
	// Names and identifiers
	"{name}":     "test",
	"{Service}":  "Test Service",
	"{category}": "testing",
	"{Platform}": "Test Platform",

	// SDK fields
	"{sdk_import_path}": "github.com/example/sdk",
	"{sdk_version}":     "v1.0.0",
	"{org}":             "example",
	"{sdk-repo}":        "sdk-repo",

	// Timestamps
	"{ISO 8601 timestamp}": "2026-01-01T00:00:00Z",
	"{ISO 8601}":           "2026-01-01T00:00:00Z",
	"{date}":               "2026-01-01",

	// Hashes and refs
	"{hash of spec content}":  "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
	"{hash of arazzo file}":   "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
	"{SHA-256 of spec}":       "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
	"{tag or commit}":         "v1.0.0",
	"{pinned version}":        "v1.0.0",
	"{pinned API version}":    "2024-01-01",
	"{git commit of archive}": "abc123def456",

	// Researcher-specific
	"{developer docs URL}": "https://developer.example.com",
	"{platform}":           "test-platform",
	"{source_type}":        "official_docs",
	"{url}":                "https://example.com/docs",
	"{archive_path}":       "archive/official-docs/2026-01-01",
	"{scope}":              "candidate",

	// Ports (handled specially — substituted as integer)
	"{port}": "4200",
}

var (
	// Matches <!-- schema: something.schema.json -->
	annotationRe = regexp.MustCompile(`<!--\s*schema:\s*(\S+\.schema\.json)\s*-->`)
	// Matches ```json ... ``` blocks
	codeBlockRe = regexp.MustCompile("```json\\s*\\n([\\s\\S]*?)\\n```")
	// Matches {placeholder} patterns in strings (single-line, no nested braces/newlines)
	placeholderRe = regexp.MustCompile(`\{[^{}\n]+\}`)
)

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

	var failures []string
	validated := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		path := filepath.Join(skillsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: cannot read %s: %v\n", e.Name(), err)
			continue
		}

		content := string(data)
		blocks := findAnnotatedBlocks(content)

		if len(blocks) == 0 {
			fmt.Printf("  skip  %s (no annotated blocks)\n", e.Name())
			continue
		}

		for _, block := range blocks {
			schemaPath := filepath.Join(schemasDir, block.schema)
			sch, err := compileSchema(schemaPath)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s [%s]: cannot compile schema: %v", e.Name(), block.schema, err))
				continue
			}

			substituted, err := substitutePlaceholders(block.json)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s [%s]: %v", e.Name(), block.schema, err))
				continue
			}

			var v any
			if err := json.Unmarshal([]byte(substituted), &v); err != nil {
				failures = append(failures, fmt.Sprintf("%s [%s]: invalid JSON after substitution: %v\nSubstituted JSON:\n%s", e.Name(), block.schema, err, substituted))
				continue
			}

			if err := sch.Validate(v); err != nil {
				failures = append(failures, fmt.Sprintf("%s [%s]: validation failed: %v", e.Name(), block.schema, err))
				continue
			}

			validated++
			fmt.Printf("  ok    %s [%s]\n", e.Name(), block.schema)
		}
	}

	fmt.Printf("\n%d template blocks validated\n", validated)

	if len(failures) > 0 {
		fmt.Printf("%d failures:\n", len(failures))
		for _, f := range failures {
			fmt.Printf("  FAIL  %s\n", f)
		}
		os.Exit(1)
	}
}

type annotatedBlock struct {
	schema string
	json   string
}

// findAnnotatedBlocks finds <!-- schema: X --> annotations followed by ```json blocks.
func findAnnotatedBlocks(content string) []annotatedBlock {
	var blocks []annotatedBlock

	annotations := annotationRe.FindAllStringSubmatchIndex(content, -1)
	for _, loc := range annotations {
		schema := content[loc[2]:loc[3]]
		// Look for the next ```json block after this annotation
		rest := content[loc[1]:]
		match := codeBlockRe.FindStringSubmatch(rest)
		if match == nil {
			continue
		}
		// Verify the code block comes before any other annotation
		nextAnnotation := annotationRe.FindStringIndex(rest[1:])
		codeBlockIdx := strings.Index(rest, "```json")
		if nextAnnotation != nil && nextAnnotation[0]+1 < codeBlockIdx {
			continue // Another annotation comes before this code block
		}

		blocks = append(blocks, annotatedBlock{
			schema: schema,
			json:   match[1],
		})
	}

	return blocks
}

// substitutePlaceholders replaces {placeholder} values with valid dummy data.
// It handles both string and integer contexts.
func substitutePlaceholders(jsonStr string) (string, error) {
	// Find all unique placeholders in the template
	matches := placeholderRe.FindAllString(jsonStr, -1)
	seen := make(map[string]bool)

	for _, m := range matches {
		if seen[m] {
			continue
		}
		seen[m] = true

		replacement, ok := findPlaceholder(m)
		if !ok {
			return "", fmt.Errorf("unrecognized placeholder: %s", m)
		}

		// Check if this placeholder appears in a non-string context (e.g., as a bare value)
		// by looking for patterns like `: {placeholder}` without surrounding quotes
		barePattern := regexp.MustCompile(`:\s*` + regexp.QuoteMeta(m))
		quotedPattern := regexp.MustCompile(`"[^"]*` + regexp.QuoteMeta(m) + `[^"]*"`)

		bareLocs := barePattern.FindAllStringIndex(jsonStr, -1)
		for _, loc := range bareLocs {
			segment := jsonStr[loc[0]:loc[1]]
			// Check if this bare occurrence is inside a quoted string
			inQuotes := false
			for _, qloc := range quotedPattern.FindAllStringIndex(jsonStr, -1) {
				if loc[0] >= qloc[0] && loc[1] <= qloc[1] {
					inQuotes = true
					break
				}
			}
			if !inQuotes {
				// This is a bare value (integer context) — replace with unquoted number
				jsonStr = strings.Replace(jsonStr, segment, ": "+replacement, 1)
			}
		}

		// Replace remaining occurrences (inside quoted strings) with just the value
		jsonStr = strings.ReplaceAll(jsonStr, m, replacement)
	}

	return jsonStr, nil
}

// findPlaceholder looks up a placeholder in the known map. It also handles
// pattern-based matching for variations like {hash ...}, {SHA-256 ...}, etc.
func findPlaceholder(p string) (string, bool) {
	// Direct lookup
	if v, ok := placeholders[p]; ok {
		return v, true
	}

	inner := strings.ToLower(p[1 : len(p)-1])

	// Pattern matching for common variations
	switch {
	case strings.HasPrefix(inner, "hash"):
		return "abc123def456abc123def456abc123def456abc123def456abc123def456abcd", true
	case strings.HasPrefix(inner, "sha-256"):
		return "abc123def456abc123def456abc123def456abc123def456abc123def456abcd", true
	case strings.HasPrefix(inner, "pinned"):
		return "v1.0.0", true
	case strings.HasPrefix(inner, "iso 8601"):
		return "2026-01-01T00:00:00Z", true
	case strings.Contains(inner, "timestamp"):
		return "2026-01-01T00:00:00Z", true
	case strings.Contains(inner, "version"):
		return "v1.0.0", true
	case strings.Contains(inner, "url"):
		return "https://example.com", true
	}

	return "", false
}

func compileSchema(path string) (*jsonschema.Schema, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	c := jsonschema.NewCompiler()
	return c.Compile("file://" + absPath)
}
