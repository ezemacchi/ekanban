//go:build !windows

package gh

import "os/exec"

func hideWindow(*exec.Cmd) {}
