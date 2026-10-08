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
