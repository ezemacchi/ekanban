//go:build windows

package watch

import "golang.org/x/sys/windows"

const stillActive = 259

// alive reports whether a pid is a running process. Windows has no signal 0,
// so it opens the process and asks whether it has exited yet.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
