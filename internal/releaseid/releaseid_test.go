package releaseid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Golden vectors: the published checksums of two releases in
// WonderTwin-AI/registry, and the digest the definition gives them.
var slackChecksums = map[string]string{
	"darwin-amd64": "c4b3f1cf32cd0645d8f33639f3435f0b79c13651cb4e261e25ee1f304af7437c",
	"darwin-arm64": "e1fe69325e118a844c36d5d57c13200b371c41471c0631bde772de20e6d5ff74",
	"linux-amd64":  "2165f4f9796e9d3ba0ad71dc5c9ae83f77da2cfba0f146e60069c2fa254c6ec6",
	"linux-arm64":  "df4858846d3ba27c14cd4f0c3d53572038006833630b5cdcb838a29284254ca2",
}

var stripeChecksums = map[string]string{
	"darwin-amd64": "sha256:326c0e4c4fe3c26d0ff17fe41f17b35b97d32a7eaad7f49e2f9b7215d797fa19",
	"darwin-arm64": "sha256:58823d900eb36e18178b082f041ffca7d91a42c75861c31203f3453699baee4a",
	"linux-amd64":  "sha256:56a86d176da44e4f6edd122676a21e1174399f662d77378210999f5819db1f88",
	"linux-arm64":  "sha256:c48d9c09b572de841c4be6697a39769b78027217a72a8e9b7ea8e9bab9232c85",
}

const (
	slackDigest  = "sha256:56de41a05a98e18df036ad9780c735a642928227fec8efe2361ac06978df1eeb"
	slackID      = "56de41a05a98"
	stripeDigest = "sha256:88e30791251a8fd50b8f35610b3bcf83954382c7ee76a9838b3d0b18a0ecbe9d"
	stripeID     = "88e30791251a"
)

func TestDigestGoldenVectors(t *testing.T) {
	for _, tc := range []struct {
		twin      string
		checksums map[string]string
		digest    string
		id        string
	}{
		{"slack", slackChecksums, slackDigest, slackID},
		{"stripe", stripeChecksums, stripeDigest, stripeID},
	} {
		got, err := Digest(tc.twin, tc.checksums)
		if err != nil {
			t.Fatalf("%s: %v", tc.twin, err)
		}
		if got != tc.digest {
			t.Errorf("%s digest = %s, want %s", tc.twin, got, tc.digest)
		}
		if id := ID(got); id != tc.id {
			t.Errorf("%s id = %s, want %s", tc.twin, id, tc.id)
		}
	}
}

func TestDigestRejects(t *testing.T) {
	missing := copyMap(slackChecksums)
	delete(missing, "linux-arm64")

	extra := copyMap(slackChecksums)
	extra["windows-amd64"] = slackChecksums["linux-amd64"]

	renamed := copyMap(missing)
	renamed["windows-amd64"] = slackChecksums["linux-amd64"]

	upper := copyMap(slackChecksums)
	upper["darwin-amd64"] = strings.ToUpper(upper["darwin-amd64"])

	short := copyMap(slackChecksums)
	short["darwin-amd64"] = short["darwin-amd64"][:63]

	for name, checksums := range map[string]map[string]string{
		"missing platform": missing,
		"extra platform":   extra,
		"unknown platform": renamed,
		"uppercase hex":    upper,
		"short hex":        short,
	} {
		if _, err := Digest("slack", checksums); err == nil {
			t.Errorf("%s: want error, got nil", name)
		}
	}
}

func TestFromChecksumsFile(t *testing.T) {
	lines := []string{
		slackChecksums["linux-arm64"] + "  twin-slack-linux-arm64",
		"",
		"0000000000000000000000000000000000000000000000000000000000000000  twin-slack-0.1.0.cdx.json",
		slackChecksums["darwin-arm64"] + "  twin-slack-darwin-arm64",
		slackChecksums["linux-amd64"] + "  twin-slack-linux-amd64",
		slackChecksums["darwin-amd64"] + "  twin-slack-darwin-amd64",
	}
	digest, id, err := FromChecksumsFile(writeFile(t, lines), "slack")
	if err != nil {
		t.Fatal(err)
	}
	if digest != slackDigest || id != slackID {
		t.Errorf("got %s %s, want %s %s", digest, id, slackDigest, slackID)
	}
}

func TestFromChecksumsFileRejects(t *testing.T) {
	missing := []string{
		slackChecksums["darwin-amd64"] + "  twin-slack-darwin-amd64",
		slackChecksums["darwin-arm64"] + "  twin-slack-darwin-arm64",
		slackChecksums["linux-amd64"] + "  twin-slack-linux-amd64",
	}
	duplicate := append(append([]string{}, missing...),
		slackChecksums["linux-arm64"]+"  twin-slack-linux-arm64",
		slackChecksums["linux-amd64"]+"  twin-slack-linux-amd64",
	)
	for name, lines := range map[string][]string{"missing platform": missing, "duplicate platform": duplicate} {
		if _, _, err := FromChecksumsFile(writeFile(t, lines), "slack"); err == nil {
			t.Errorf("%s: want error, got nil", name)
		}
	}
	// Another twin's binaries are not this release.
	if _, _, err := FromChecksumsFile(writeFile(t, missing), "stripe"); err == nil {
		t.Error("other twin: want error, got nil")
	}
}

func TestIsID(t *testing.T) {
	for s, want := range map[string]bool{
		slackID:            true,
		"0.1.0":            false,
		"56de41a05a9":      false,
		"56de41a05a98e":    false,
		"56DE41A05A98":     false,
		"56de41a05a9g":     false,
		"":                 false,
		"slack-" + slackID: false,
	} {
		if got := IsID(s); got != want {
			t.Errorf("IsID(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestCoordinate(t *testing.T) {
	want := "localstack/app-emulator@slack-56de41a05a98"
	if got := Coordinate("slack", slackID); got != want {
		t.Errorf("Coordinate = %s, want %s", got, want)
	}
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func writeFile(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
