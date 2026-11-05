//go:build !windows

package llm

import (
	"os/exec"
	"syscall"
)

// detach lets the server outlive docsmaker.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
