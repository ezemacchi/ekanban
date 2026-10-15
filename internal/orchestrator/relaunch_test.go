package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// rfake is a Herdr with one terminal agent in it: two ctrl+c quit it (when
// quits is set), and a new agent, once started, is idle until enterWorks
// enters have been pressed.
type rfake struct {
	fake
	tabList    []herdr.Tab
	paneList   []herdr.Pane
	keys       []string
	quits      bool
	enterWorks int
	ctrlC      int
	enters     int
}

func str(s string) *string { return &s }

func (f *rfake) Tabs(string) ([]herdr.Tab, error) { return f.tabList, nil }
func (f *rfake) Panes() ([]herdr.Pane, error)     { return f.paneList, nil }
func (f *rfake) SendKeys(pane string, k ...string) error {
	for _, key := range k {
		f.keys = append(f.keys, pane+"|"+key)
		switch key {
		case "ctrl+c":
			f.ctrlC++
			if f.quits && f.ctrlC >= 2 {
				f.agents = nil
			}
		case "enter":
			f.enters++
		}
	}
	return nil
}

// Agents is the old agent, if it has not quit, and the new one once started.
func (f *rfake) Agents() ([]herdr.Agent, error) {
	out := append([]herdr.Agent(nil), f.agents...)
	for _, s := range f.started {
		parts := strings.Split(s, "|")
		status := "idle"
		if f.enters >= f.enterWorks {
			status = "working"
		}
		out = append(out, herdr.Agent{Agent: str("cursor"), Name: parts[0], PaneID: parts[2], AgentStatus: status})
	}
	return out, nil
}

// quick makes Relaunch's waits short enough for a test.
func quick(t *testing.T) {
	was := [5]any{keyGap, stopPoll, stopFor, submitFor, submitTries}
	keyGap, stopPoll, stopFor, submitFor, submitTries = 0, time.Millisecond, 20*time.Millisecond, 10*time.Millisecond, 3
	t.Cleanup(func() {
		keyGap, stopPoll, stopFor, submitFor, submitTries = was[0].(time.Duration), was[1].(time.Duration), was[2].(time.Duration), was[3].(time.Duration), was[4].(int)
	})
}

func relaunchRun(t *testing.T) *ticket.Run {
	r := run(t)
	r.Orchestrator.Kind, r.Orchestrator.Args = "cursor", []string{"--model", "m", "--trust"}
	r.Orchestrator.Prompt = "Resume {key}: read {state}."
	return r
}

// With an orchestrator open, it is quit and the new one starts in the same
// pane: no new tab, and the name is free again because the old one is gone.
func TestRelaunchReplacesARunningOrchestrator(t *testing.T) {
	quick(t)
	f := &rfake{quits: true, enterWorks: 0}
	f.agents = []herdr.Agent{{Agent: str("cursor"), Name: "abc-1", PaneID: "w:p1", AgentStatus: "idle"}}
	r := relaunchRun(t)
	r.OrchestratorPane = "w:p1"

	text, err := Relaunch(f, r)
	if err != nil {
		t.Fatal(err)
	}
	if f.ctrlC != 2 || len(f.tabs) != 0 {
		t.Fatalf("ctrl+c %d, tabs %q", f.ctrlC, f.tabs)
	}
	if len(f.started) != 1 || f.started[0] != "abc-1|cursor|w:p1|--model m --trust" {
		t.Fatalf("started %q", f.started)
	}
	data, err := os.ReadFile(filepath.Join(r.Dir, "orchestrator-resume.md"))
	if err != nil || !strings.HasPrefix(string(data), "Resume ABC-1: read ") {
		t.Fatalf("prompt file %q %v", data, err)
	}
	if len(f.prompts) != 1 || !strings.HasPrefix(f.prompts[0], "abc-1|Read ") {
		t.Fatalf("prompts %q", f.prompts)
	}
	if f.enters != 0 || len(f.focused) != 1 || !strings.Contains(text, "relaunched") {
		t.Fatalf("%q enters %d focused %q", text, f.enters, f.focused)
	}
}

// An agent that does not quit is reported, never killed, and nothing starts.
func TestRelaunchLeavesAnAgentThatWillNotQuit(t *testing.T) {
	quick(t)
	f := &rfake{quits: false}
	f.agents = []herdr.Agent{{Agent: str("cursor"), Name: "abc-1", PaneID: "w:p1", AgentStatus: "idle"}}
	r := relaunchRun(t)
	r.OrchestratorPane = "w:p1"

	_, err := Relaunch(f, r)
	if err == nil || !strings.Contains(err.Error(), "nothing was killed") {
		t.Fatalf("err = %v", err)
	}
	if len(f.started) != 0 || len(f.tabs) != 0 {
		t.Fatalf("started %q tabs %q", f.started, f.tabs)
	}
	if _, statErr := os.Stat(filepath.Join(r.Dir, "orchestrator-resume.md")); statErr == nil {
		t.Fatal("a prompt was written for an agent that never started")
	}
}

// When the orchestrator died and left its tab, the empty shell in it is reused
// instead of opening a second tab with the same name.
func TestRelaunchReusesTheTabAnOrchestratorLeft(t *testing.T) {
	quick(t)
	f := &rfake{tabList: []herdr.Tab{{ID: "w:t1", Label: "board"}, {ID: "w:t2", Label: "orchestrator"}},
		paneList: []herdr.Pane{{PaneID: "w:p1", TabID: "w:t1"}, {PaneID: "w:p2", TabID: "w:t2"}}}
	r := relaunchRun(t)

	if _, err := Relaunch(f, r); err != nil {
		t.Fatal(err)
	}
	if len(f.tabs) != 0 || f.ctrlC != 0 {
		t.Fatalf("tabs %q ctrl+c %d", f.tabs, f.ctrlC)
	}
	if len(f.started) != 1 || !strings.Contains(f.started[0], "|w:p2|") {
		t.Fatalf("started %q", f.started)
	}
}

// With no orchestrator and no tab for it, a tab is opened as Go would.
func TestRelaunchOpensATabWhenThereIsNone(t *testing.T) {
	quick(t)
	f := &rfake{tabList: []herdr.Tab{{ID: "w:t1", Label: "board"}}}
	r := relaunchRun(t)

	if _, err := Relaunch(f, r); err != nil {
		t.Fatal(err)
	}
	if len(f.tabs) != 1 || f.tabs[0] != "w|"+r.Worktree+"|orchestrator" {
		t.Fatalf("tabs %q", f.tabs)
	}
	if len(f.started) != 1 || !strings.Contains(f.started[0], "|w:p9|") {
		t.Fatalf("started %q", f.started)
	}
}

// A prompt whose enter was lost is submitted by pressing enter again, and a
// prompt that never takes is reported with the tab to go to.
func TestRelaunchPressesEnterWhenThePromptWasNotSubmitted(t *testing.T) {
	quick(t)
	f := &rfake{tabList: []herdr.Tab{{ID: "w:t2", Label: "orchestrator"}},
		paneList: []herdr.Pane{{PaneID: "w:p2", TabID: "w:t2"}}, enterWorks: 2}
	if _, err := Relaunch(f, relaunchRun(t)); err != nil || f.enters != 2 {
		t.Fatalf("err %v, enters %d", err, f.enters)
	}

	f = &rfake{tabList: []herdr.Tab{{ID: "w:t2", Label: "orchestrator"}},
		paneList: []herdr.Pane{{PaneID: "w:p2", TabID: "w:t2"}}, enterWorks: 99}
	_, err := Relaunch(f, relaunchRun(t))
	if err == nil || !strings.Contains(err.Error(), "press enter in its tab") {
		t.Fatalf("err = %v", err)
	}
	if f.enters != submitTries-1 {
		t.Fatalf("enters %d, want %d", f.enters, submitTries-1)
	}
}

func TestRelaunchCannotStart(t *testing.T) {
	quick(t)
	r := run(t)
	if _, err := Relaunch(&rfake{}, r); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("no kind: %v", err)
	}
	r.Orchestrator.Kind, r.Workspace = "cursor", ""
	if _, err := Relaunch(&rfake{}, r); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("no workspace: %v", err)
	}
}
