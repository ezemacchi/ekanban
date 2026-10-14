package pipeline

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// FallbackBranch is the target when the repository names no default branch.
const FallbackBranch = "main"

// DefaultBranch is the branch pull requests in the repository at dir merge
// into when [[pipeline.target]] names none: the remote's default branch
// (origin/HEAD), else FallbackBranch.
func DefaultBranch(dir string, hide func(*exec.Cmd)) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if hide != nil {
		hide(cmd)
	}
	out, err := cmd.Output()
	if err != nil {
		return FallbackBranch
	}
	if b := strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/"); b != "" {
		return b
	}
	return FallbackBranch
}
