//go:build windows

package procmgr

import "os/exec"

func setDetachedProcessAttrs(cmd *exec.Cmd) {}
