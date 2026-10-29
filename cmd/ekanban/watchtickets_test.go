package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/state"
	"github.com/ezemacchi/ekanban/internal/watch"
)

type marker struct{ got []string }

func (m *marker) ReportPaneMetadata(pane, source, display string, tokens map[string]string) error {
	m.got = append(m.got, pane+"|"+display+"|"+tokens["ekanban_role"])
	return nil
}

// An agent found by its name or tab gets its pane marked; one already marked,
// by whoever started it, keeps its mark.
func TestMarkUnmarked(t *testing.T) {
	agents := []herdr.Agent{
		{PaneID: "w:p1"},
		{PaneID: "w:p2", Tokens: map[string]string{"ekanban_key": "ABC-1", "ekanban_role": "qa"}},
		{PaneID: "w:p3"},
	}
	rows := []watch.Row{{Ticket: state.Ticket{
		Key:          "ABC-1",
		Orchestrator: &state.Agent{Who: "Orchestrator", Pane: "w:p1", Role: "orchestrator"},
		Agents: []state.Agent{
			{Who: "QA", Pane: "w:p2", Role: "qa"},
			{Who: "Reviewer", Pane: "w:p3", Role: "reviewer"},
			{Who: "Gone", Pane: "w:p9", Role: "implementer"},
		},
	}}}
	m := &marker{}
	markUnmarked(m, agents, rows)
	if len(m.got) != 2 || m.got[0] != "w:p1|ABC-1 · Orchestrator|orchestrator" || m.got[1] != "w:p3|ABC-1 · Reviewer|reviewer" {
		t.Fatalf("marked %q", m.got)
	}
}

// Folders are grouped by repository, and what is not a checkout -- a plain
// folder the board lists as a space -- is left out.
func TestByRepo(t *testing.T) {
	root := t.TempDir()
	mk := func(name string, checkout bool) string {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if checkout {
			if err := os.WriteFile(filepath.Join(d, ".git"), []byte("gitdir: x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}
	a1, a2, b1, plain := mk("a-1", true), mk("a-2", true), mk("b-1", true), mk("notes", false)
	repo := map[string]string{a1: "/repo/a", a2: "/repo/a", b1: "/repo/b", plain: "/repo/a"}
	groups := byRepo([]string{a2, plain, b1, a1}, func(d string) string { return repo[d] })
	if len(groups) != 2 || groups[0].repo != "/repo/a" || groups[1].repo != "/repo/b" {
		t.Fatalf("groups: %+v", groups)
	}
	if len(groups[0].dirs) != 2 || groups[0].dirs[0] != a1 || groups[0].dirs[1] != a2 {
		t.Fatalf("repo a: %v", groups[0].dirs)
	}
}
