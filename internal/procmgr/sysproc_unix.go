//go:build !windows

package procmgr

import (
	"os/exec"
	"syscall"
)

func setDetachedProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
