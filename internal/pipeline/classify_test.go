package pipeline

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ezemacchi/ekanban/internal/ticket"
)

// mergeRepos is a remote, a clone of it and a worktree of the clone on a
// branch of its own with one commit, plus a working copy of the remote where
// pull requests are merged into main the way Bitbucket words them.
func mergeRepos(t *testing.T, branch string) (clone, worktree, upstream string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	upstream = filepath.Join(root, "upstream")
	gitT(t, root, "init", "-q", "-b", "main", upstream)
	commit(t, upstream, "a")
	bare := filepath.Join(root, "origin.git")
	gitT(t, root, "clone", "-q", "--bare", upstream, bare)
	gitT(t, upstream, "remote", "add", "origin", bare)
	clone = filepath.Join(root, "clone")
	gitT(t, root, "clone", "-q", bare, clone)
	worktree = filepath.Join(root, "wt")
	gitT(t, clone, "worktree", "add", "-q", "-b", branch, worktree, "origin/main")
	commit(t, worktree, "work")
	return clone, worktree, upstream
}

// mergePR merges ref into main upstream with Bitbucket's subject naming
// branch, pushes, and fetches into the clone.
func mergePR(t *testing.T, clone, upstream string, pr int, branch, ref string) {
	t.Helper()
	gitT(t, upstream, "checkout", "-q", "main")
	gitT(t, upstream, "merge", "-q", "--no-ff", "-m",
		"Merge pull request #"+strconv.Itoa(pr)+" in PROJ/repo from "+branch+" to main", ref)
	gitT(t, upstream, "push", "-q", "origin", "main")
	gitT(t, clone, "fetch", "-q", "origin")
}

func classify(clone, worktree string, pr int) Info {
	src := New(clone, Settings{Targets: []Target{{Branch: "main"}}}, nil)
	return src.Classify(context.Background(), worktree, ticket.Live{}, pr)
}

// A merge that holds the worktree's commit is merged and not diverged.
func TestClassifyPlainMerge(t *testing.T) {
	const branch = "feat/ABC-1-thing"
	clone, worktree, upstream := mergeRepos(t, branch)
	gitT(t, worktree, "push", "-q", "origin", branch)
	gitT(t, upstream, "fetch", "-q", "origin", branch+":"+branch)
	mergePR(t, clone, upstream, 7, branch, branch)

	info := classify(clone, worktree, 7)
	if info.MergedTo != "main" || info.PR != 7 || info.Diverged || info.Stage != ToDeploy {
		t.Fatalf("got %+v", info)
	}
}

// A branch rewritten on the server before merging leaves the worktree on
// commits the merge does not hold; the merge's subject still names the branch.
func TestClassifyRebasedMergeNamesTheBranch(t *testing.T) {
	const branch = "feat/ABC-1-thing"
	clone, worktree, upstream := mergeRepos(t, branch)
	gitT(t, upstream, "checkout", "-q", "-b", branch, "main")
	commit(t, upstream, "work-rebased")
	mergePR(t, clone, upstream, 7, branch, branch)

	info := classify(clone, worktree, 7)
	if info.MergedTo != "main" || info.PR != 7 || !info.Diverged || info.Stage != ToDeploy {
		t.Fatalf("got %+v", info)
	}
}

// A branch stacked on another one's pull request does not take that pull
// request's merge as its own.
func TestClassifyStackedBranchDropsTheBasePR(t *testing.T) {
	const base = "feat/ABC-1-thing"
	clone, worktree, upstream := mergeRepos(t, "feat/ABC-2-other")
	gitT(t, upstream, "checkout", "-q", "-b", base, "main")
	commit(t, upstream, "base-work")
	mergePR(t, clone, upstream, 7, base, base)

	info := classify(clone, worktree, 7)
	if info.MergedTo != "" || info.PR != 0 || info.Diverged {
		t.Fatalf("got %+v", info)
	}
	if info.Key != "ABC-2" {
		t.Fatalf("key %q", info.Key)
	}
}
