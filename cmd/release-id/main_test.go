package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const slackChecksumsFile = `c4b3f1cf32cd0645d8f33639f3435f0b79c13651cb4e261e25ee1f304af7437c  twin-slack-darwin-amd64
e1fe69325e118a844c36d5d57c13200b371c41471c0631bde772de20e6d5ff74  twin-slack-darwin-arm64
2165f4f9796e9d3ba0ad71dc5c9ae83f77da2cfba0f146e60069c2fa254c6ec6  twin-slack-linux-amd64
df4858846d3ba27c14cd4f0c3d53572038006833630b5cdcb838a29284254ca2  twin-slack-linux-arm64
`

func TestRunPrintsOutputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(path, []byte(slackChecksumsFile), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"--twin", "slack", "--checksums-file", path}, &out); err != nil {
		t.Fatal(err)
	}
	want := "id=56de41a05a98\n" +
		"digest=sha256:56de41a05a98e18df036ad9780c735a642928227fec8efe2361ac06978df1eeb\n" +
		"coordinate=localstack/app-emulator@slack-56de41a05a98\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunRequiresFlags(t *testing.T) {
	if err := run([]string{"--twin", "slack"}, &bytes.Buffer{}); err == nil {
		t.Error("want error without --checksums-file")
	}
}
