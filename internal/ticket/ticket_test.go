package ticket

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezemacchi/herdr-phin-board/internal/herdr"
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

func TestPrototypeFromSpecFieldOrMostMentioned(t *testing.T) {
	specsDir := t.TempDir()
	protos := filepath.Join(specsDir, "specifications", "backlog", "E7", "prototypes")
	write(t, filepath.Join(protos, "E7_US_42_batch_01_baseline.html"), "x")
	write(t, filepath.Join(protos, "E7_US_42_batch_00_index.html"), "x")
	write(t, filepath.Join(protos, "E7_US_05_other_00_index.html"), "x")

	wt := t.TempDir()
	run := filepath.Join(wt, ".runs", "ABC-1")
	write(t, filepath.Join(run, "STATE.md"), "Team: Full Team\nSpec: e7-us-42\n")
	r, err := Load(wt, Options{SpecRoot: specsDir}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Spec != "E7_US_42" || filepath.Base(r.Prototype) != "E7_US_42_batch_00_index.html" {
		t.Fatalf("spec %q prototype %q", r.Spec, r.Prototype)
	}

	// No Spec field: the story the run's files mention most wins.
	write(t, filepath.Join(run, "STATE.md"), "Team: Full Team\n")
	write(t, filepath.Join(run, "ENVELOPE.md"), "E7_US_42 E7_US_42 related: E7_US_05")
	if r, _ = Load(wt, Options{SpecRoot: specsDir}, nil, nil); r.Spec != "E7_US_42" {
		t.Fatalf("most mentioned: %q", r.Spec)
	}
}

func TestLoadPlacesRoles(t *testing.T) {
	wt := t.TempDir()
	run := filepath.Join(wt, ".runs", "ABC-1")
	write(t, filepath.Join(run, "STATE.md"), "# Run\r\nTeam: Big Team | Target: master | Jira: ABC-1\r\n\r\n## Objective\r\nFix the thing.\r\n\r\n## Open questions\r\n- Which caption? (for the lead)\r\n- Old one (decided: keep)\r\n\r\n## Known defects\r\n- not a question\r\n\r\n## Landed\r\n")
	write(t, filepath.Join(run, "ENVELOPE.md"), "x")
	write(t, filepath.Join(run, "DISPATCH-01-evidence.md"), "x")
	write(t, filepath.Join(run, "DISPATCH-02-implementer.md"), "x")
	write(t, filepath.Join(run, "DISPATCH-03-implementer.md"), "x")

	agent := "cursor"
	agents := []herdr.Agent{
		{Agent: &agent, AgentStatus: "working", PaneID: "w:p3", TabID: "w:t3", Cwd: filepath.Join(wt, "src")},
		{Agent: &agent, AgentStatus: "blocked", PaneID: "w:p9", TabID: "w:t9", Cwd: filepath.Join(t.TempDir())},
	}
	r, err := Load(wt, Options{}, agents, map[string]string{"w:t3": "implementer", "w:t9": "qa"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Key != "ABC-1" || r.Team != "Big Team" || r.Target != "master" {
		t.Fatalf("header: %+v", r)
	}
	if len(r.Questions) != 1 || r.Questions[0] != "Which caption? (for the lead)" {
		t.Fatalf("questions: %q", r.Questions)
	}
	if r.Landed {
		t.Fatal("empty Landed section must not count")
	}
	r.parse("Team: x\n## Landed\n```\nNOT LANDED: run ABC-1\n```\n")
	if l := strings.Join(r.section(`(?i)^landed`), "\n"); !strings.Contains(l, "NOT LANDED") {
		t.Fatalf("landed section: %q", l)
	}
	want := map[string]Column{"technical-lead": Done, "evidence": Pending, "implementer": Working, "qa": Pending}
	for _, c := range r.Cards {
		if w, ok := want[c.Role.ID]; ok && c.Column != w {
			t.Errorf("%s: column %d, want %d (%s)", c.Role.ID, c.Column, w, c.Note)
		}
		if c.Role.ID == "implementer" && (c.Note != "vuelta 2" || c.PaneID != "w:p3") {
			t.Errorf("implementer: %+v", c)
		}
	}
}
