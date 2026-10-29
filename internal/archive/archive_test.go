package archive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezemacchi/ekanban/internal/ticket"
)

func runIn(t *testing.T) (worktree, runDir string) {
	t.Helper()
	worktree = t.TempDir()
	runDir = filepath.Join(worktree, ".runs", "ABC-1")
	files := map[string]string{
		"STATE.md":               "# STATE ABC-1\n\n## Objective\nAdd the export.\n\n## Questions for you\n- Which model?\n",
		"LOG.md":                 "- started\n",
		"questions/which-key.md": "---\nto: Ana\nstatus: open\n---\n",
	}
	for name, text := range files {
		p := filepath.Join(runDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return worktree, runDir
}

// The copy reads as the run did, with the readers a worktree is read with,
// and is made again only when something changed.
func TestSnapshot(t *testing.T) {
	worktree, runDir := runIn(t)
	dest := Path(t.TempDir(), "ABC-1")

	copied, err := Snapshot(worktree, runDir, dest)
	if err != nil || !copied {
		t.Fatalf("first copy: %t %v", copied, err)
	}
	run, err := ticket.Load(dest, ticket.Options{}, ticket.Live{})
	if err != nil {
		t.Fatal(err)
	}
	if run.Key != "ABC-1" || len(run.ForYou) != 1 || len(run.Objective) == 0 || run.Objective[0] != "Add the export." {
		t.Fatalf("run read from the copy: key %q, for you %d, objective %q", run.Key, len(run.ForYou), run.Objective)
	}

	if copied, err := Snapshot(worktree, runDir, dest); err != nil || copied {
		t.Fatalf("unchanged run copied again: %t %v", copied, err)
	}

	later := time.Now().Add(time.Minute)
	log := filepath.Join(runDir, "LOG.md")
	if err := os.WriteFile(log, []byte("- started\n- landed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(log, later, later)
	if copied, err := Snapshot(worktree, runDir, dest); err != nil || !copied {
		t.Fatalf("changed run not copied: %t %v", copied, err)
	}
	got, _ := os.ReadFile(filepath.Join(dest, ".runs", "ABC-1", "LOG.md"))
	if !strings.Contains(string(got), "landed") {
		t.Fatalf("log in the copy: %q", got)
	}
}

// A file removed from the run is gone from the next copy: it is replaced
// whole, not merged into.
func TestSnapshotReplacesWhole(t *testing.T) {
	worktree, runDir := runIn(t)
	dest := Path(t.TempDir(), "ABC-1")
	if _, err := Snapshot(worktree, runDir, dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, "questions", "which-key.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Snapshot(worktree, runDir, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".runs", "ABC-1", "questions", "which-key.md")); !os.IsNotExist(err) {
		t.Fatalf("a removed file is still in the copy: %v", err)
	}
	if m, err := ReadMeta(dest); err != nil || m.From != runDir {
		t.Fatalf("meta: %+v %v", m, err)
	}
}

func TestPathRefusesWhatIsNotAName(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "C:"} {
		if p := Path("/state", bad); p != "" {
			t.Errorf("Path(%q) = %q", bad, p)
		}
	}
	if p := Path("/state", "ABC-1"); p != filepath.Join("/state", "archive", "ABC-1") {
		t.Fatalf("path %q", p)
	}
}

func TestSnapshotNeedsTheRunInsideTheWorktree(t *testing.T) {
	_, runDir := runIn(t)
	if _, err := Snapshot(t.TempDir(), runDir, Path(t.TempDir(), "ABC-1")); err == nil {
		t.Fatal("a run outside the worktree was copied")
	}
}
