package gh

import "os/exec"

// HideWindow keeps a child process from flashing a console window on Windows;
// elsewhere it does nothing. Other packages that run git or gh use it too.
func HideWindow(cmd *exec.Cmd) { hideWindow(cmd) }
