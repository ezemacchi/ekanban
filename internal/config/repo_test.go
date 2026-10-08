package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeRepo lays out a main checkout and a linked worktree the way git does,
// without running git.
func fakeRepo(t *testing.T) (main, worktree string) {
	t.Helper()
	root := t.TempDir()
	main = filepath.Join(root, "app")
	worktree = filepath.Join(root, "app-wt-1")
	gitDir := filepath.Join(main, ".git", "worktrees", "app-wt-1")
	put(t, filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
	put(t, filepath.Join(gitDir, "commondir"), "../..\n")
	put(t, filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")
	return main, worktree
}

func TestRepoFileIsFoundFromAWorktreeAndASubfolder(t *testing.T) {
	main, wt := fakeRepo(t)
	if got := RepoFile(filepath.Join(wt, "src")); got != "" {
		t.Fatalf("no file anywhere: got %q", got)
	}
	put(t, filepath.Join(main, RepoFileName), "")
	if got := RepoFile(filepath.Join(wt, "src")); got != filepath.Join(main, RepoFileName) {
		t.Fatalf("from the worktree: got %q", got)
	}
	put(t, filepath.Join(wt, RepoFileName), "")
	if got := RepoFile(wt); got != filepath.Join(wt, RepoFileName) {
		t.Fatalf("the worktree's own file wins: got %q", got)
	}
	if got := RepoFile(main); got != filepath.Join(main, RepoFileName) {
		t.Fatalf("from the main checkout: got %q", got)
	}
}

func TestRepoFileOverlaysConfig(t *testing.T) {
	write(t, `icons = true
issue_url = "https://tracker.example/{key}"

[pipeline]
code_host = "github"
rules = ["merged", "working"]

[keys]
accept = "y"

[lead]
kind = "cursor"
args = ["--model", "m"]
prompt = "personal"
`)
	main, wt := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), `[pipeline]
rules = ["landed"]

[keys]
archive = "z"

[lead]
kind = "evil"
prompt = "the project's"

[run]
state = "status.md"
`)
	s := LoadFor(wt)
	if s.RepoPath != filepath.Join(main, RepoFileName) {
		t.Fatalf("repo path %q", s.RepoPath)
	}
	if !s.Icons || s.IssueURL != "https://tracker.example/{key}" || s.Pipeline.CodeHost != "github" {
		t.Fatalf("values the repo file does not mention were lost: %+v", s)
	}
	if strings.Join(s.Pipeline.Rules, ",") != "landed" {
		t.Fatalf("a list is replaced whole: %q", s.Pipeline.Rules)
	}
	if s.Keys["accept"][0] != "y" || s.Keys["archive"][0] != "z" {
		t.Fatalf("a table merges: %v", s.Keys)
	}
	if s.Lead.Prompt != "the project's" || s.Lead.Kind != "cursor" || strings.Join(s.Lead.Args, " ") != "--model m" {
		t.Fatalf("lead: %+v", s.Lead)
	}
	if len(s.Problems) != 1 || !strings.Contains(s.Problems[0], "kind and args") {
		t.Fatalf("problems: %q", s.Problems)
	}
	if s.Layout.State != "status.md" || s.Layout.Dir != ".runs" {
		t.Fatalf("layout: %+v", s.Layout)
	}

	// Outside any repository only config.toml counts.
	if s := LoadFor(t.TempDir()); s.RepoPath != "" || s.Lead.Prompt != "personal" {
		t.Fatalf("outside a repo: %q %q", s.RepoPath, s.Lead.Prompt)
	}
}

func TestBrokenRepoFileLeavesConfigAlone(t *testing.T) {
	write(t, "icons = true\n")
	main, _ := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), "icons = false\n[broken\n")
	s := LoadFor(main)
	if !s.Icons || len(s.Problems) != 1 || !strings.Contains(s.Problems[0], "not valid TOML") {
		t.Fatalf("icons %t problems %q", s.Icons, s.Problems)
	}
}
