//go:build !windows

package watch

import (
	"os"
	"syscall"
)

// alive reports whether a pid is a running process. Signal 0 checks for
// existence without delivering anything.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
