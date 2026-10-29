package ticket

import (
	"path/filepath"
	"testing"

	"github.com/ezemacchi/ekanban/internal/herdr"
)

func marked(key, role string) map[string]string {
	return map[string]string{TokenKey: key, TokenRole: role}
}

// An agent whose pane is marked is found by the mark, whatever it is called
// and whatever its tab says: the name is gone after a restart, and a tab can
// be named anything.
func TestMarksPlaceAgentsWhateverTheirNamesSay(t *testing.T) {
	wt := t.TempDir()
	write(t, filepath.Join(wt, ".runs", "ABC-1", "STATE.md"), "Team: Full team\n")
	agent := "cursor"
	live := Live{
		Agents: []herdr.Agent{
			// Its tab says orchestrator, but its pane is the implementer's.
			{Agent: &agent, AgentStatus: "working", PaneID: "w:p1", TabID: "w:t1", WorkspaceID: "w", Cwd: wt, Tokens: marked("ABC-1", "implementer")},
			// No name, a tab nobody renamed: the orchestrator by its mark.
			{Agent: &agent, AgentStatus: "idle", PaneID: "w:p2", TabID: "w:t2", WorkspaceID: "w", Cwd: wt, Tokens: marked("abc-1", "orchestrator")},
			// A second QA instance, marked as Start-RoleAgent labels it.
			{Agent: &agent, AgentStatus: "blocked", PaneID: "w:p3", TabID: "w:t3", WorkspaceID: "w", Cwd: wt, Tokens: marked("ABC-1", "qa-2")},
			// A discussion: in the workspace, no role's, and still not the orchestrator.
			{Agent: &agent, AgentStatus: "idle", PaneID: "w:p4", TabID: "w:t4", WorkspaceID: "w", Cwd: wt, Tokens: marked("ABC-1", "question")},
			// Marked for another ticket: not this run's, even in its folder.
			{Agent: &agent, AgentStatus: "working", PaneID: "w:p5", TabID: "w:t5", WorkspaceID: "w", Cwd: wt, Name: "abc-1-reviewer", Tokens: marked("ABC-2", "reviewer")},
		},
		Tabs:       map[string]string{"w:t1": "orchestrator", "w:t2": "2", "w:t3": "qa", "w:t4": "question x", "w:t5": "reviewer"},
		Workspaces: map[string]string{"w": wt},
	}
	r, err := Load(wt, Options{Teams: fullTeam(t)}, live)
	if err != nil {
		t.Fatal(err)
	}
	if r.OrchestratorPane != "w:p2" {
		t.Fatalf("orchestrator = %q, want the pane marked as it", r.OrchestratorPane)
	}
	panes := map[string]string{}
	for _, c := range r.Cards {
		panes[c.Role.ID] = c.PaneID
	}
	if panes["implementer"] != "w:p1" || panes["qa"] != "w:p3" || panes["reviewer"] != "" {
		t.Fatalf("roles by pane: %v", panes)
	}
	for _, a := range r.Agents() {
		if a.Pane == "w:p2" && a.Role != RoleOrchestrator || a.Pane == "w:p1" && a.Role != "implementer" {
			t.Fatalf("agent %s reported as role %q", a.Pane, a.Role)
		}
	}

	// Without the orchestrator's mark, the discussion still is not taken for it.
	live.Agents = append(live.Agents[:1], live.Agents[2:]...)
	if r, _ = Load(wt, Options{Teams: fullTeam(t)}, live); r.OrchestratorPane != "" {
		t.Fatalf("orchestrator = %q, want none: every agent left is marked as something else", r.OrchestratorPane)
	}
}

type marks struct{ got []string }

func (m *marks) ReportPaneMetadata(pane, source, display string, tokens map[string]string) error {
	m.got = append(m.got, pane+"|"+source+"|"+display+"|"+tokens[TokenKey]+"|"+tokens[TokenRole])
	return nil
}

func TestMarkNamesTheTicketAndRole(t *testing.T) {
	m := &marks{}
	if err := Mark(m, "w:p1", "ABC-1", "reviewer", "Reviewer"); err != nil {
		t.Fatal(err)
	}
	if len(m.got) != 1 || m.got[0] != "w:p1|plugin:ekanban|ABC-1 · Reviewer|ABC-1|reviewer" {
		t.Fatalf("marked %q", m.got)
	}
}
