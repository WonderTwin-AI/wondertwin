package main

import (
	"fmt"
	"os"

	"github.com/wondertwin-ai/wondertwin/internal/replay"
)

// cmdReplay dispatches `wt replay <subcommand>`. Currently only
// `wt replay show <path>` is implemented; `wt replay diff` is flagged
// as a future follow-up in WonderTwin-AI/wondertwin-pro#88.
func cmdReplay(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt replay <show> <artifact-path>")
	}
	switch args[0] {
	case "show":
		return cmdReplayShow(args[1:])
	case "diff":
		return fmt.Errorf("wt replay diff: not yet implemented (tracked as a follow-up to WonderTwin-AI/wondertwin-pro#88)")
	default:
		return fmt.Errorf("wt replay: unknown subcommand %q (expected: show)", args[0])
	}
}

func cmdReplayShow(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: wt replay show <artifact-path>")
	}
	path := args[0]
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	art, err := replay.Read(f)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return replay.Show(os.Stdout, art)
}
