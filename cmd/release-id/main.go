// Command release-id prints the release id, digest and coordinate of an
// app emulator release, computed from its checksums file. The output is
// key=value lines, so a workflow can append it to $GITHUB_OUTPUT. See
// internal/releaseid for the definition.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wondertwin-ai/wondertwin/internal/releaseid"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "release-id: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release-id", flag.ContinueOnError)
	twin := fs.String("twin", "", "twin name (e.g. stripe)")
	checksumsFile := fs.String("checksums-file", "", "path to the release's checksums file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *twin == "" || *checksumsFile == "" {
		return fmt.Errorf("--twin and --checksums-file are both required")
	}

	digest, id, err := releaseid.FromChecksumsFile(*checksumsFile, *twin)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "id=%s\ndigest=%s\ncoordinate=%s\n", id, digest, releaseid.Coordinate(*twin, id))
	return err
}
