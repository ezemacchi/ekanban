package ticket

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLayoutReadsAnotherTeamsRuns(t *testing.T) {
	l, problems := NewLayout(LayoutConfig{
		Dir:         "work",
		State:       "status.md",
		TeamField:   "Crew",
		Objective:   `(?i)^goal`,
		Landed:      `(?i)^shipped`,
		NotLanded:   `(?i)failed`,
		Questions:   `(?i)^asks`,
		NotQuestion: `(?i)^never`,
	})
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	wt := t.TempDir()
	write(t, filepath.Join(wt, "work", "XY-7", "status.md"),
		"Crew: Solo\n\n## Goal\nShip it.\n\n## Asks\n- Which colour?\n\n## Shipped\nmerged\n")
	r, err := Load(wt, Options{Layout: &l}, Live{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Key != "XY-7" || r.Team != "Solo" || strings.Join(r.Objective, " ") != "Ship it." {
		t.Fatalf("run: key %q team %q objective %q", r.Key, r.Team, r.Objective)
	}
	if len(r.Questions) != 1 || !r.Landed {
		t.Fatalf("questions %q landed %t", r.Questions, r.Landed)
	}
	if r.StatePath() != filepath.Join(wt, "work", "XY-7", "status.md") {
		t.Fatalf("state path %q", r.StatePath())
	}
	if _, err := Load(wt, Options{}, Live{}); err == nil {
		t.Fatal("the default layout must not find a run under work/")
	}
}

func TestLayoutDefaultsAndBadPatterns(t *testing.T) {
	d := DefaultLayout()
	if d.Dir != ".runs" || d.State != "STATE.md" || d.specCode != nil || d.normalizeSpec("E7-US-5") != "" {
		t.Fatalf("defaults: %+v", d)
	}
	l, problems := NewLayout(LayoutConfig{Objective: "(", SpecCode: "(E"})
	if len(problems) != 2 || !strings.Contains(problems[0], "run.objective") {
		t.Fatalf("problems: %q", problems)
	}
	if !l.objective.MatchString("Objective") || l.specCode != nil {
		t.Fatal("a bad pattern must keep its default")
	}
}

func TestSpecCodeGroupsAndPrototypeFolder(t *testing.T) {
	l, _ := NewLayout(LayoutConfig{SpecCode: `(?i)\b(E\d+)[_-](US|TS)[_-](\d+)`, Prototypes: "backlog/{1}/{2}"})
	if got := l.normalizeSpec("see e7-us-5 here"); got != "E7_US_05" {
		t.Fatalf("normalize: %q", got)
	}
	if got := l.prototypeDir("E7_US_05"); got != "backlog/E7/US" {
		t.Fatalf("folder: %q", got)
	}
}
