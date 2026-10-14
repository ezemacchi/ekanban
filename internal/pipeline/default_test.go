package pipeline

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// The default target is the remote's default branch, whatever it is called,
// and main where there is no remote to ask.
func TestDefaultBranchIsTheRemotesDefault(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	upstream := filepath.Join(root, "upstream")
	gitT(t, root, "init", "-q", "-b", "trunk", upstream)
	commit(t, upstream, "a")
	clone := filepath.Join(root, "clone")
	gitT(t, root, "clone", "-q", upstream, clone)

	if got := DefaultBranch(clone, nil); got != "trunk" {
		t.Fatalf("clone of a trunk remote: got %q", got)
	}
	if got := DefaultBranch(upstream, nil); got != FallbackBranch {
		t.Fatalf("no remote: got %q, want %q", got, FallbackBranch)
	}
	if got := DefaultBranch(filepath.Join(root, "missing"), nil); got != FallbackBranch {
		t.Fatalf("not a repository: got %q", got)
	}
}
