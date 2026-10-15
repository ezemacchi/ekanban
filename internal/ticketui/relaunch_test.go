package ticketui

import (
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/keys"
)

func relaunchBoard(t *testing.T) (*Model, func() int) {
	t.Helper()
	m, f := forYouBoard(t)
	m.run.Workspace = "w1"
	m.run.Orchestrator.Label, m.run.Orchestrator.Tab = "Orchestrator", "orchestrator"
	return m, func() int { return f.Called("agent.start") + f.Called("pane.send_keys") + f.Called("agent.prompt") }
}

// O asks first, naming what yes costs; n changes nothing, and Herdr is not
// touched until y.
func TestRelaunchAsksYesOrNoFirst(t *testing.T) {
	m, calls := relaunchBoard(t)
	m.run.OrchestratorStatus = "working"
	m.View()

	if cmd := m.key("O"); cmd != nil || !m.relaunching {
		t.Fatalf("O did not ask: relaunching %v", m.relaunching)
	}
	view := m.View()
	for _, want := range []string{"Relaunch the orchestrator?", "ctrl+c", "conversation is lost", "is working now"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the question does not say %q:\n%s", want, view)
		}
	}

	// Another key leaves the question open; n closes it and nothing was done.
	if cmd := m.key("x"); cmd != nil || !m.relaunching {
		t.Fatal("another key answered the question")
	}
	if cmd := m.key("n"); cmd != nil || m.relaunching || m.status != "nothing was changed" || calls() != 0 {
		t.Fatalf("n: relaunching %v, status %q, herdr calls %d", m.relaunching, m.status, calls())
	}

	// esc is a no too.
	m.key("O")
	if m.key("esc"); m.relaunching {
		t.Fatal("esc did not leave it")
	}

	m.key("O")
	cmd := m.key("y")
	if cmd == nil || m.relaunching || !m.opening {
		t.Fatalf("y did not start the relaunch: cmd %v, relaunching %v, opening %v", cmd != nil, m.relaunching, m.opening)
	}
	if m.key("O"); m.relaunching {
		t.Fatal("a second relaunch was asked while one runs")
	}
}

// A run with no orchestrator open says so, rather than describing a quit.
func TestRelaunchWithNoOrchestratorSaysSo(t *testing.T) {
	m, _ := relaunchBoard(t)
	m.run.OrchestratorPane = ""
	m.View()
	m.key("O")
	if view := m.View(); !strings.Contains(view, "No orchestrator is running") || strings.Contains(view, "ctrl+c") {
		t.Fatalf("the question:\n%s", view)
	}
}

// Without a Herdr workspace there is nowhere to start one: no question is
// asked, the status says why.
func TestRelaunchNeedsAWorkspace(t *testing.T) {
	m, _ := relaunchBoard(t)
	m.run.Workspace = ""
	m.key("O")
	if m.relaunching || !strings.Contains(m.status, "no Herdr workspace") {
		t.Fatalf("relaunching %v, status %q", m.relaunching, m.status)
	}
}

// The key is an action like the others: [keys] can move it.
func TestRelaunchKeyIsRebindable(t *testing.T) {
	m, _ := relaunchBoard(t)
	m.keys, _ = keys.New(Actions, map[string][]string{"relaunch": {"R"}})
	m.key("O")
	if m.relaunching {
		t.Fatal("O still relaunches after the action moved to R")
	}
	m.key("R")
	if !m.relaunching {
		t.Fatal("R did not ask")
	}
}
