package ticket

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/team"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fullTeam is a five-role team with an orchestrator for its orchestrator.
func fullTeam(t *testing.T) []team.Team {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "full.toml"), `name = "Full"
match = '(?i)full'
[orchestrator]
label = "Orchestrator"
match = 'orchestrator'
[[role]]
id = "technical-orchestrator"
match = 'tech(nical)?-?orchestrator'
done_file = "ENVELOPE.md"
[[role]]
id = "evidence"
match = 'evidence'
done_file = "EVIDENCE.md"
[[role]]
id = "implementer"
match = 'implementer'
done_file = "IMPL.md"
[[role]]
id = "reviewer"
match = 'reviewer'
done_file = "REVIEW.md"
[[role]]
id = "qa"
match = '(^|[^a-z])qa([^a-z]|$)'
done_when_landed = true
`)
	teams, problems := team.Load(dir)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	return teams
}

func TestPrototypeFromSpecFieldOrMostMentioned(t *testing.T) {
	specs, problems := NewLayout(LayoutConfig{
		SpecCode:   `(?i)(?:^|[^a-z0-9])(E\d+)[_-](US|TS)[_-](\d+)`,
		Prototypes: "specifications/backlog/{1}",
	})
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	specsDir := t.TempDir()
	protos := filepath.Join(specsDir, "specifications", "backlog", "E7", "prototypes")
	write(t, filepath.Join(protos, "E7_US_42_batch_01_baseline.html"), "x")
	write(t, filepath.Join(protos, "E7_US_42_batch_00_index.html"), "x")
	write(t, filepath.Join(protos, "E7_US_05_other_00_index.html"), "x")

	wt := t.TempDir()
	run := filepath.Join(wt, ".runs", "ABC-1")
	write(t, filepath.Join(run, "STATE.md"), "Team: Example\nSpec: e7-us-42\n")
	opts := Options{SpecRoot: specsDir, Layout: &specs}
	r, err := Load(wt, opts, Live{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Spec != "E7_US_42" || filepath.Base(r.Prototype) != "E7_US_42_batch_00_index.html" {
		t.Fatalf("spec %q prototype %q", r.Spec, r.Prototype)
	}

	// No Spec field: the story the run's files mention most wins.
	write(t, filepath.Join(run, "STATE.md"), "Team: Example\n")
	write(t, filepath.Join(run, "ENVELOPE.md"), "E7_US_42 E7_US_42 related: E7_US_05")
	if r, _ = Load(wt, opts, Live{}); r.Spec != "E7_US_42" {
		t.Fatalf("most mentioned: %q", r.Spec)
	}
}

func TestLoadPlacesRoles(t *testing.T) {
	wt := t.TempDir()
	run := filepath.Join(wt, ".runs", "ABC-1")
	write(t, filepath.Join(run, "STATE.md"), "# Run\r\nTeam: Full team | Target: master | Jira: ABC-1\r\n\r\n## Objective\r\nFix the thing.\r\n\r\n## Open questions\r\n- Which caption? (for the orchestrator)\r\n- Old one (decided: keep)\r\n\r\n## Known defects\r\n- not a question\r\n\r\n## Landed\r\n")
	write(t, filepath.Join(run, "ENVELOPE.md"), "x")
	write(t, filepath.Join(run, "DISPATCH-01-evidence.md"), "x")
	write(t, filepath.Join(run, "DISPATCH-02-implementer.md"), "x")
	write(t, filepath.Join(run, "DISPATCH-03-implementer.md"), "x")

	agent := "cursor"
	agents := []herdr.Agent{
		{Agent: &agent, AgentStatus: "working", PaneID: "w:p3", TabID: "w:t3", Cwd: filepath.Join(wt, "src")},
		{Agent: &agent, AgentStatus: "blocked", PaneID: "w:p9", TabID: "w:t9", Cwd: filepath.Join(t.TempDir())},
	}
	r, err := Load(wt, Options{Teams: fullTeam(t)}, Live{Agents: agents, Tabs: map[string]string{"w:t3": "implementer", "w:t9": "qa"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Key != "ABC-1" || r.Team != "Full team" || r.Target != "master" {
		t.Fatalf("header: %+v", r)
	}
	if len(r.Questions) != 1 || r.Questions[0] != "Which caption? (for the orchestrator)" {
		t.Fatalf("questions: %q", r.Questions)
	}
	if r.Landed {
		t.Fatal("empty Landed section must not count")
	}
	r.parse("Team: x\n## Landed\n```\nNOT LANDED: run ABC-1\n```\n")
	if l := strings.Join(r.section(r.layout.landed), "\n"); !strings.Contains(l, "NOT LANDED") {
		t.Fatalf("landed section: %q", l)
	}
	if r.TeamName != "Full" || len(r.Cards) != 5 {
		t.Fatalf("team %q with %d cards", r.TeamName, len(r.Cards))
	}
	want := map[string]Column{"technical-orchestrator": Done, "evidence": Pending, "implementer": Working, "qa": Pending}
	for _, c := range r.Cards {
		if w, ok := want[c.Role.ID]; ok && c.Column != w {
			t.Errorf("%s: column %d, want %d (%s)", c.Role.ID, c.Column, w, c.Note)
		}
		if c.Role.ID == "implementer" && (c.Note != "round 2" || c.PaneID != "w:p3") {
			t.Errorf("implementer: %+v", c)
		}
	}
}

// An agent in the ticket's workspace belongs to it wherever its folder is,
// and one that is no role's is the orchestrator: an orchestrator started by hand in
// the main clone, in a tab nobody renamed.
func TestWorkspaceAgentsAndOrchestrator(t *testing.T) {
	wt := t.TempDir()
	write(t, filepath.Join(wt, ".runs", "ABC-1", "STATE.md"), "Team: Full team\n")
	write(t, filepath.Join(wt, ".runs", "ABC-1", "DISPATCH-01-implementer.md"), "x")
	agent := "cursor"
	elsewhere := t.TempDir()
	live := Live{
		Agents: []herdr.Agent{
			{Agent: &agent, AgentStatus: "idle", PaneID: "w:p1", TabID: "w:t1", WorkspaceID: "w", Cwd: elsewhere},
			{Agent: &agent, AgentStatus: "working", PaneID: "w:p2", TabID: "w:t2", WorkspaceID: "w", Name: "abc-1-implementer4", Cwd: wt},
			{Agent: &agent, AgentStatus: "working", PaneID: "x:p1", TabID: "x:t1", WorkspaceID: "x", Cwd: elsewhere},
		},
		Tabs:       map[string]string{"w:t1": "2", "w:t2": "implementer-rebase", "x:t1": "orchestrator"},
		Workspaces: map[string]string{"w": `\\?\` + wt, "x": elsewhere},
	}
	opts := Options{Teams: fullTeam(t)}
	r, err := Load(wt, opts, live)
	if err != nil {
		t.Fatal(err)
	}
	if r.Workspace != "w" {
		t.Fatalf("workspace = %q", r.Workspace)
	}
	if r.OrchestratorPane != "w:p1" || r.OrchestratorStatus != "idle" || r.Orchestrator.Label != "Orchestrator" {
		t.Fatalf("orchestrator = %q %q %q", r.OrchestratorPane, r.OrchestratorStatus, r.Orchestrator.Label)
	}
	for _, c := range r.Cards {
		if c.Role.ID == "implementer" && (c.Column != Working || c.PaneID != "w:p2") {
			t.Fatalf("implementer: %+v", c)
		}
	}

	// A tab its match names wins over an unnamed one.
	live.Tabs["w:t1"] = "notes"
	live.Agents = append(live.Agents, herdr.Agent{Agent: &agent, AgentStatus: "working", PaneID: "w:p3", TabID: "w:t3", WorkspaceID: "w", Cwd: wt})
	live.Tabs["w:t3"] = "orchestrator"
	if r, _ = Load(wt, opts, live); r.OrchestratorPane != "w:p3" {
		t.Fatalf("named orchestrator = %q", r.OrchestratorPane)
	}

	// No workspace open: only agents inside the worktree count, and none is
	// the orchestrator by elimination.
	live.Workspaces = map[string]string{"x": elsewhere}
	if r, _ = Load(wt, opts, live); r.Workspace != "" || r.OrchestratorPane != "w:p3" {
		t.Fatalf("without workspace: %q %q", r.Workspace, r.OrchestratorPane)
	}
	live.Tabs["w:t3"] = "scratch"
	if r, _ = Load(wt, opts, live); r.OrchestratorPane != "" {
		t.Fatalf("orchestrator by elimination without a workspace: %q", r.OrchestratorPane)
	}
}
