package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezemacchi/ekanban/internal/archive"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/store"
)

// [accept] from lets a card be accepted from an earlier column too; a column
// the board does not have is left out, and said so.
func TestAcceptFromWidensTheColumnsACardIsAcceptedFrom(t *testing.T) {
	m := pipelineBoard(t)
	problems := m.SetClose([]string{pipeline.ToDeploy, pipeline.ReadyQA, "nowhere"}, nil)
	if len(problems) != 1 || !strings.Contains(problems[0], `"nowhere"`) {
		t.Fatalf("problems %q", problems)
	}
	api := store.Key(tmp + "api")
	info := m.pipeInfo[api]
	info.Stage = pipeline.ToDeploy
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: info}})
	if got := labelsIn(m, pipeline.ToDeploy); !equal(got, []string{"api"}) {
		t.Fatalf("Shipping holds %v, want [api]", got)
	}
	selectSpace(t, m, tmp+"api")
	send(t, m, key("a"))
	if m.board.Entries[api].Accepted == nil {
		t.Fatalf("a card in Shipping was not accepted: %q", m.status)
	}
}

// Without [accept] from, only the last column accepts.
func TestAcceptStaysInTheLastColumnByDefault(t *testing.T) {
	m := pipelineBoard(t)
	api := store.Key(tmp + "api")
	info := m.pipeInfo[api]
	info.Stage = pipeline.ToDeploy
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: info}})
	selectSpace(t, m, tmp+"api")
	send(t, m, key("a"))
	if m.board.Entries[api].Accepted != nil {
		t.Fatal("a card in Shipping was accepted with no [accept] from")
	}
	if !strings.Contains(m.status, "To QA") {
		t.Fatalf("status %q does not say where a card can be accepted from", m.status)
	}
}

// The Archive shows what the run said when its notes were copied, read from
// the copy: the worktree may be gone.
func TestArchiveShowsTheCopiedNotes(t *testing.T) {
	m := pipelineBoard(t)
	worktree := t.TempDir()
	runDir := filepath.Join(worktree, ".runs", "ABC-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "# STATE ABC-1\n\n## Objective\nAdd the export.\n\n## Questions for you\n- Which model?\n"
	if err := os.WriteFile(filepath.Join(runDir, "STATE.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := archive.Path(t.TempDir(), "ABC-1")
	if _, err := archive.Snapshot(worktree, runDir, dest); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	m.board.Accept(store.Key(tmp+"api"), store.Accepted{
		At: time.Now().UTC(), Ticket: "ABC-1", Title: "Arreglar la grilla", Repo: m.scope, Archive: dest,
	})
	send(t, m, key("A"))
	m.width = 160
	out := m.View()
	for _, want := range []string{"Objective: Add the export.", "Left for you: Which model?", "notes copied"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the Archive is missing %q:\n%s", want, out)
		}
	}
}
