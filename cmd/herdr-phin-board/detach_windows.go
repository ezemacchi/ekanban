//go:build windows

package main

import "syscall"

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// detachedAttr starts the watcher outside the board's process group, so closing
// the board's pane does not take it down. It gets a hidden console rather than
// none: a process without a console makes every console program it starts (gh,
// git) open a window of its own, which flashes on screen at each poll.
func detachedAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
}
