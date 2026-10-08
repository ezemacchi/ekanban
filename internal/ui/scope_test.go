package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitCommonDirJoinsWorktreesToTheirRepository(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "repo")
	wt := filepath.Join(root, "repo-wt")
	other := filepath.Join(root, "other")
	private := filepath.Join(main, ".git", "worktrees", "repo-wt")
	for _, d := range []string{private, wt, filepath.Join(other, ".git"), filepath.Join(main, "src")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+private+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	want := gitCommonDir(main)
	if want == "" {
		t.Fatal("main checkout has no common dir")
	}
	if got := gitCommonDir(wt); got != want {
		t.Fatalf("worktree common dir %q, want %q", got, want)
	}
	if got := gitCommonDir(filepath.Join(main, "src")); got != want {
		t.Fatalf("subfolder common dir %q, want %q", got, want)
	}
	if got := gitCommonDir(other); got == want {
		t.Fatal("another repository must not share the scope")
	}

	m := &Model{}
	m.SetScope(main)
	if !m.inScope(wt) || m.inScope(other) {
		t.Fatal("scope must keep the worktree and drop the other repository")
	}
}

// A worktree read before git has finished writing it must not be dismissed for
// good. Otherwise a ticket workspace opened at that moment never reaches the
// board until the board is restarted.
func TestScopeRechecksAFolderThatWasNotYetAWorktree(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "repo")
	wt := filepath.Join(root, "repo-wt")
	private := filepath.Join(main, ".git", "worktrees", "repo-wt")
	for _, d := range []string{private, wt} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	m := &Model{}
	m.SetScope(main)
	if m.inScope(wt) {
		t.Fatal("a folder with no git files is not part of the repository yet")
	}

	// Git finishes writing the worktree.
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+private+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Within the recheck window the earlier answer stands...
	if m.inScope(wt) {
		t.Fatal("the answer should hold until the recheck interval has passed")
	}
	// ...and afterwards it is read again.
	old := m.scopeOf[wt]
	old.at = old.at.Add(-2 * scopeRecheck)
	m.scopeOf[wt] = old
	if !m.inScope(wt) {
		t.Fatal("a worktree that has since appeared must join the board")
	}

	// A yes is kept without reading again.
	if err := os.RemoveAll(filepath.Join(wt, ".git")); err != nil {
		t.Fatal(err)
	}
	if !m.inScope(wt) {
		t.Fatal("a folder already known to belong must stay on the board")
	}
}
