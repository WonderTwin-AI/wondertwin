//go:build !windows

package conformance

import (
	"os/exec"
	"syscall"
)

func setConformanceProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
