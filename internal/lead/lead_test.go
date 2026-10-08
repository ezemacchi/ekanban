package lead

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/team"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

type fake struct {
	agents  []herdr.Agent
	focused []string
	tabs    []string
	started []string
	prompts []string
	failTab bool
}

func (f *fake) Agents() ([]herdr.Agent, error) { return f.agents, nil }
func (f *fake) FocusAgent(p string) error      { f.focused = append(f.focused, p); return nil }
func (f *fake) CreateTab(ws, cwd, label string) (string, string, error) {
	if f.failTab {
		return "", "", errors.New("no")
	}
	f.tabs = append(f.tabs, ws+"|"+cwd+"|"+label)
	return ws + ":t9", ws + ":p9", nil
}
func (f *fake) StartAgent(name, kind, pane string, args []string) error {
	f.started = append(f.started, name+"|"+kind+"|"+pane+"|"+strings.Join(args, " "))
	return nil
}
func (f *fake) PromptAgent(target, text string) error {
	f.prompts = append(f.prompts, target+"|"+text)
	return nil
}

func run(t *testing.T) *ticket.Run {
	l, err := team.Lead{Label: "Orchestrator", Tab: "orchestrator"}.With(team.Lead{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	return &ticket.Run{Key: "ABC-1", Dir: dir, Worktree: filepath.Dir(dir), TeamName: "Example", Lead: l, Workspace: "w"}
}

func TestGoFocusesAnOpenLead(t *testing.T) {
	f := &fake{}
	r := run(t)
	r.LeadPane = "w:p1"
	text, err := Go(f, r)
	if err != nil || len(f.focused) != 1 || f.focused[0] != "w:p1" || len(f.tabs) != 0 {
		t.Fatalf("%q %v %+v", text, err, f)
	}
}

func TestGoOpensANewLead(t *testing.T) {
	f := &fake{agents: []herdr.Agent{{Name: "abc-1"}}}
	r := run(t)
	r.Lead.Kind, r.Lead.Args = "cursor", []string{"--model", "m"}
	r.Lead.Prompt = "Lead {label} of {key} ({team}): read {state}."
	text, err := Go(f, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.tabs) != 1 || f.tabs[0] != "w|"+r.Worktree+"|orchestrator" {
		t.Fatalf("tabs %q", f.tabs)
	}
	// abc-1 is taken, so the tab is added to the name.
	if len(f.started) != 1 || f.started[0] != "abc-1-orchestrator|cursor|w:p9|--model m" {
		t.Fatalf("started %q", f.started)
	}
	file := filepath.Join(r.Dir, "orchestrator-resume.md")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "Lead Orchestrator of ABC-1 (Example): read " + filepath.ToSlash(filepath.Join(r.Dir, "STATE.md")) + ".\n"
	if string(data) != want {
		t.Fatalf("prompt file %q, want %q", data, want)
	}
	if len(f.prompts) != 1 || !strings.Contains(f.prompts[0], filepath.ToSlash(file)) || !strings.HasPrefix(f.prompts[0], "abc-1-orchestrator|") {
		t.Fatalf("prompts %q", f.prompts)
	}
	if len(f.focused) != 1 || f.focused[0] != "w:p9" || !strings.Contains(text, "abc-1-orchestrator") {
		t.Fatalf("%q %q", text, f.focused)
	}
}

func TestGoCannotOpen(t *testing.T) {
	r := run(t)
	if _, err := Go(&fake{}, r); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("no kind: %v", err)
	}
	r.Lead.Kind, r.Workspace = "cursor", ""
	if _, err := Go(&fake{}, r); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("no workspace: %v", err)
	}
	r.Workspace = "w"
	f := &fake{failTab: true}
	if _, err := Go(f, r); err == nil || len(f.started) != 0 {
		t.Fatalf("failed tab: %v %+v", err, f)
	}
}

func TestFreeNameCounts(t *testing.T) {
	r := run(t)
	agents := []herdr.Agent{{Name: "abc-1"}, {Name: "abc-1-orchestrator"}, {Name: "ABC-1-orchestrator-2"}}
	if got := freeName(r, agents); got != "abc-1-orchestrator-3" {
		t.Fatalf("freeName = %q", got)
	}
}
