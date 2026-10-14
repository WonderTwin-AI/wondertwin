// Package releaseid derives an app emulator release's id from its
// published binaries.
//
// A release is the four platform binaries twin-{name}-{platform}. The
// release manifest is one line per binary, in Platforms order, each
// "<sha256 hex>  twin-{name}-{platform}\n": byte-identical to shasum -a
// 256 output for those files sorted by name. The release digest is the
// SHA-256 of that manifest, and the release id is its first 12 hex
// characters. Nothing else is hashed: the SBOM's name carries the id, so
// it cannot be an input to it.
//
// A release is named by its coordinate, localstack/app-emulator@{name}-{id}.
//
// Recompute an id from a published checksums.txt:
//
//	grep -E '  twin-NAME-(darwin|linux)-(amd64|arm64)$' checksums.txt | LC_ALL=C sort -k2 | shasum -a 256 | cut -c1-12
package releaseid

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

// Platforms are the release targets, in manifest order.
var Platforms = []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"}

// IDLength is the number of hex characters of the digest kept in an id.
const IDLength = 12

const digestPrefix = "sha256:"

var (
	idPattern  = regexp.MustCompile(`^[0-9a-f]{12}$`)
	hexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Digest returns the release digest, "sha256:<64 hex>", of a twin's
// release given each platform binary's SHA-256. Values may carry a
// "sha256:" prefix. Every platform must be present, and no other.
func Digest(twin string, checksums map[string]string) (string, error) {
	if len(checksums) != len(Platforms) {
		return "", fmt.Errorf("want checksums for %d platforms %v, got %d", len(Platforms), Platforms, len(checksums))
	}
	var manifest strings.Builder
	for _, p := range Platforms {
		sum, ok := checksums[p]
		if !ok {
			return "", fmt.Errorf("missing checksum for platform %s", p)
		}
		sum = strings.TrimPrefix(sum, digestPrefix)
		if !hexPattern.MatchString(sum) {
			return "", fmt.Errorf("checksum for platform %s is not 64 lowercase hex: %q", p, sum)
		}
		fmt.Fprintf(&manifest, "%s  twin-%s-%s\n", sum, twin, p)
	}
	h := sha256.Sum256([]byte(manifest.String()))
	return digestPrefix + hex.EncodeToString(h[:]), nil
}

// ID returns the release id for a release digest: its first 12 hex
// characters.
func ID(digest string) string {
	sum := strings.TrimPrefix(digest, digestPrefix)
	if len(sum) < IDLength {
		return sum
	}
	return sum[:IDLength]
}

// IsID reports whether s has the form of a release id.
func IsID(s string) bool {
	return idPattern.MatchString(s)
}

// Coordinate returns the name a release is published under,
// localstack/app-emulator@{twin}-{id}.
func Coordinate(twin, id string) string {
	return fmt.Sprintf("localstack/app-emulator@%s-%s", twin, id)
}

// FromChecksumsFile reads a checksums file in "<hex>  <filename>" format,
// keeps the four twin-{twin}-{platform} binaries and ignores every other
// line, and returns the release digest and id.
func FromChecksumsFile(path, twin string) (digest, id string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	prefix := fmt.Sprintf("twin-%s-", twin)
	checksums := make(map[string]string, len(Platforms))
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		sum, filename, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "  ")
		if !ok {
			continue
		}
		platform, ok := strings.CutPrefix(strings.TrimSpace(filename), prefix)
		if !ok || !slices.Contains(Platforms, platform) {
			continue
		}
		if _, dup := checksums[platform]; dup {
			return "", "", fmt.Errorf("%s lists platform %s twice", path, platform)
		}
		checksums[platform] = sum
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	digest, err = Digest(twin, checksums)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", path, err)
	}
	return digest, ID(digest), nil
}
