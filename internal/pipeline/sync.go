package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Keeping the target branches up to date in the checkouts. The board fetches
// them to know what is merged; with fast_forward it also brings the local
// branches up to what it fetched -- only ever by fast-forward, and never
// where that could lose work: a branch with commits of its own, or one
// checked out where there are changes, is left alone and said so.

// Synced is what bringing the target branches up to date did: which were
// moved, and which were left alone and why.
type Synced struct {
	Moved []string // "master ↑4"
	Left  []string // "predev ↓3 · local changes"
}

// String is the result in a few words, "" when there was nothing to do.
func (s Synced) String() string {
	return strings.Join(append(append([]string(nil), s.Moved...), s.Left...), " · ")
}

// Sync fetches the target branches and fast-forwards each local one that is
// behind its remote and safe to move.
func (s *Source) Sync(ctx context.Context) Synced {
	branches := s.targetBranches()
	if len(branches) == 0 {
		return Synced{}
	}
	s.git(ctx, append([]string{"fetch", "--quiet", "origin"}, branches...)...)
	r := s.fastForward(ctx, branches)
	s.setSynced(r)
	return r
}

func (s *Source) fastForward(ctx context.Context, branches []string) Synced {
	var out Synced
	checkedOut := s.checkouts(ctx)
	for _, b := range branches {
		local, remote := "refs/heads/"+b, "refs/remotes/origin/"+b
		old := s.git(ctx, "rev-parse", "--verify", "--quiet", local)
		target := s.git(ctx, "rev-parse", "--verify", "--quiet", remote)
		if old == "" || target == "" {
			continue // no such local branch to keep up to date
		}
		behind := s.count(ctx, local+".."+remote)
		if behind == 0 {
			continue
		}
		if ahead := s.count(ctx, remote+".."+local); ahead > 0 {
			out.Left = append(out.Left, fmt.Sprintf("%s ↓%d · %s of its own", b, behind, commits(ahead)))
			continue
		}
		dir, isOut := checkedOut[b]
		switch {
		case !isOut:
			// No checkout to touch: move the branch, if it is still where
			// it was read.
			if _, err := s.run(ctx, s.repo, "update-ref", local, target, old); err != nil {
				out.Left = append(out.Left, fmt.Sprintf("%s ↓%d · %v", b, behind, err))
				continue
			}
		case s.gitIn(ctx, dir, "status", "--porcelain", "--untracked-files=no") != "":
			out.Left = append(out.Left, fmt.Sprintf("%s ↓%d · local changes", b, behind))
			continue
		default:
			if _, err := s.run(ctx, dir, "merge", "--ff-only", "--quiet", remote); err != nil {
				out.Left = append(out.Left, fmt.Sprintf("%s ↓%d · %v", b, behind, err))
				continue
			}
		}
		out.Moved = append(out.Moved, fmt.Sprintf("%s ↑%d", b, behind))
	}
	return out
}

// checkouts maps each branch checked out in a worktree of the repository to
// that worktree's folder.
func (s *Source) checkouts(ctx context.Context) map[string]string {
	out := map[string]string{}
	dir := ""
	for _, line := range strings.Split(s.git(ctx, "worktree", "list", "--porcelain"), "\n") {
		line = strings.TrimSpace(line)
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			dir = p
		}
		if b, ok := strings.CutPrefix(line, "branch refs/heads/"); ok && dir != "" {
			out[b] = dir
		}
	}
	return out
}

// count is how many commits a range holds, 0 when git cannot say.
func (s *Source) count(ctx context.Context, rng string) int {
	n, _ := strconv.Atoi(s.git(ctx, "rev-list", "--count", rng))
	return n
}

// run is git in dir, with its error and what it said on failure.
func (s *Source) run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	s.hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return "", fmt.Errorf("%s", firstLine(msg))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func commits(n int) string {
	if n == 1 {
		return "1 commit"
	}
	return strconv.Itoa(n) + " commits"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
