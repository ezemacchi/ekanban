//go:build !windows

package main

import "syscall"

// detachedAttr starts the watcher in its own session, so it outlives the board.
func detachedAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
