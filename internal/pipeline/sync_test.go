package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitT runs git in dir for a test, failing it on error.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, file string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "add", file)
	gitT(t, dir, "commit", "-q", "-m", file)
}

// syncRepos is a remote with branches main, next, side and own, a clone of it
// with main checked out, and next checked out in a worktree; then the remote
// moves every branch on by one commit.
func syncRepos(t *testing.T) (clone, worktree, upstream string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	upstream = filepath.Join(root, "upstream")
	gitT(t, root, "init", "-q", "-b", "main", upstream)
	commit(t, upstream, "a")
	for _, b := range []string{"next", "side", "own"} {
		gitT(t, upstream, "branch", b)
	}
	bare := filepath.Join(root, "origin.git")
	gitT(t, root, "clone", "-q", "--bare", upstream, bare)
	clone = filepath.Join(root, "clone")
	gitT(t, root, "clone", "-q", bare, clone)
	for _, b := range []string{"next", "side", "own"} {
		gitT(t, clone, "branch", "-q", b, "origin/"+b)
	}
	worktree = filepath.Join(root, "wt")
	gitT(t, clone, "worktree", "add", "-q", worktree, "next")

	// The remote moves on.
	gitT(t, upstream, "remote", "add", "origin", bare)
	for _, b := range []string{"main", "next", "side", "own"} {
		gitT(t, upstream, "checkout", "-q", b)
		commit(t, upstream, "up-"+b)
		gitT(t, upstream, "push", "-q", "origin", b)
	}
	return clone, worktree, upstream
}

// Sync moves a branch nobody has checked out and one checked out in a clean
// checkout, and leaves alone one with changes where it is checked out and one
// with commits of its own -- saying why.
func TestSyncFastForwardsOnlyWhereItIsSafe(t *testing.T) {
	clone, worktree, _ := syncRepos(t)
	// main, checked out in the clone, has a change in a tracked file.
	if err := os.WriteFile(filepath.Join(clone, "a"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	// own has a commit of its own.
	gitT(t, clone, "worktree", "add", "-q", filepath.Join(filepath.Dir(clone), "own"), "own")
	commit(t, filepath.Join(filepath.Dir(clone), "own"), "mine")

	src := New(clone, Settings{Targets: []Target{{Branch: "main"}, {Branch: "next"}, {Branch: "side"}, {Branch: "own"}}}, nil)
	got := src.Sync(context.Background())

	if strings.Join(got.Moved, ",") != "next ↑1,side ↑1" {
		t.Fatalf("moved %v", got.Moved)
	}
	if len(got.Left) != 2 || !strings.Contains(got.Left[0], "main ↓1 · local changes") || !strings.Contains(got.Left[1], "own ↓1 · 1 commit of its own") {
		t.Fatalf("left %v", got.Left)
	}
	for _, b := range []string{"next", "side"} {
		if gitT(t, clone, "rev-parse", b) != gitT(t, clone, "rev-parse", "origin/"+b) {
			t.Fatalf("%s was not brought up to origin", b)
		}
	}
	if _, err := os.Stat(filepath.Join(worktree, "up-next")); err != nil {
		t.Fatal("the checkout of next does not have the new commit's file")
	}
	if gitT(t, clone, "rev-parse", "main") == gitT(t, clone, "rev-parse", "origin/main") {
		t.Fatal("main moved over a local change")
	}
	if src.LastSync().String() != got.String() {
		t.Fatalf("last sync %q", src.LastSync())
	}
}

// Refresh fast-forwards only when the setting asks for it.
func TestRefreshFastForwardsOnlyWhenAsked(t *testing.T) {
	clone, _, _ := syncRepos(t)
	targets := []Target{{Branch: "side"}}
	New(clone, Settings{Targets: targets}, nil).Refresh(context.Background())
	if gitT(t, clone, "rev-parse", "side") == gitT(t, clone, "rev-parse", "origin/side") {
		t.Fatal("a refresh without fast_forward moved side")
	}
	New(clone, Settings{Targets: targets, FastForward: true}, nil).Refresh(context.Background())
	if gitT(t, clone, "rev-parse", "side") != gitT(t, clone, "rev-parse", "origin/side") {
		t.Fatal("a refresh with fast_forward did not move side")
	}
}
