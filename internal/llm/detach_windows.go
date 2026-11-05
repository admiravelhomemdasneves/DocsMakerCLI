//go:build windows

package llm

import (
	"os/exec"
	"syscall"
)

// detach lets the server outlive docsmaker and keeps it from opening a
// console window.
func detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}
