//go:build windows

package gh

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps gh from flashing a console window when the caller has none.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
}
