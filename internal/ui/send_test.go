package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/herdrtest"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// An orchestrator waiting at a prompt is listed but never ticked: not by a
// quick choice, not by space, not by a click. The list says why.
func TestBlockedOrchestratorIsNeverTicked(t *testing.T) {
	m := pipelineBoard(t)
	m.width, m.height = 140, 40
	send(t, m, key("B"))
	send(t, m, recipientsMsg{to: []recipient{
		{key: "ABC-1", pane: "p1", status: "idle", on: true},
		{key: "ABC-2", pane: "p2", status: "blocked"},
	}})
	if out := ansi.Strip(m.View()); !strings.Contains(out, "[-] ABC-2") || !strings.Contains(out, "answer its prompt first") {
		t.Fatalf("the blocked orchestrator is not marked:\n%s", out)
	}
	send(t, m, key("tab"))
	send(t, m, key("a"))
	if got := m.chosen(); len(got) != 1 || got[0].key != "ABC-1" {
		t.Fatalf("All ticked %v", got)
	}
	send(t, m, key("j"))
	send(t, m, key(" "))
	if m.bc.to[1].on || !strings.Contains(m.status, "waiting at a prompt") {
		t.Fatalf("space ticked the blocked one; status %q", m.status)
	}
	clickZoneOf(t, m, isChoice(1))
	if m.bc.to[1].on {
		t.Fatal("a click ticked the blocked one")
	}
}

// broadcastResult runs the commands cmd batches and returns what the send
// reported.
func broadcastResult(t *testing.T, cmd tea.Cmd) broadcastMsg {
	t.Helper()
	msg := cmd()
	if b, ok := msg.(broadcastMsg); ok {
		return b
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if b, ok := c().(broadcastMsg); ok {
				return b
			}
		}
	}
	t.Fatalf("no broadcast result in %T", msg)
	return broadcastMsg{}
}

// The broadcast submits through agent.prompt, Herdr's own submission, to the
// orchestrator's pane, with {target} filled in.
func TestBroadcastSubmitsThroughAgentPrompt(t *testing.T) {
	f := herdrtest.Start(t)
	f.OK(map[string]any{"type": "ok"})
	c, err := herdr.New()
	if err != nil {
		t.Fatal(err)
	}
	m := pipelineBoard(t)
	m.client = c
	got := broadcastResult(t, m.sendBroadcast("Rebase onto {target}.", []recipient{{key: "ABC-1", pane: "w1:p1", target: "main"}}))
	if strings.Join(got.sent, ",") != "ABC-1" {
		t.Fatalf("result %+v", got)
	}
	req := f.Last(t, "agent.prompt")
	params := herdrtest.Params(t, req)
	if params["target"] != "w1:p1" || params["text"] != "Rebase onto main." {
		t.Fatalf("params = %+v", params)
	}
	if f.Called("pane.send_input") != 0 || f.Called("pane.send_text") != 0 {
		t.Fatal("the broadcast typed into the pane")
	}
}

// Herdr's agent_blocked -- the orchestrator went to a prompt after the list
// was read -- is said apart from a failure.
func TestBroadcastSaysWhoWasBlocked(t *testing.T) {
	f := herdrtest.Start(t)
	f.Fail("agent_blocked", "agent is waiting at a dialog")
	c, _ := herdr.New()
	m := pipelineBoard(t)
	m.client = c
	got := broadcastResult(t, m.sendBroadcast("hi", []recipient{{key: "ABC-1", pane: "w1:p1"}}))
	if strings.Join(got.blocked, ",") != "ABC-1" || len(got.failed) != 0 {
		t.Fatalf("result %+v", got)
	}
	m.applyBroadcast(got)
	if !strings.Contains(m.status, "not sent to ABC-1: waiting at a prompt") {
		t.Fatalf("status %q", m.status)
	}
}

// m does not type into an agent waiting at a prompt: the text would land in
// the dialog.
func TestPickAgentRefusesABlockedAgent(t *testing.T) {
	a := agent("claude", "w1", "w1:p1")
	a.AgentStatus = "blocked"
	if _, err := pickAgent([]herdr.Agent{a}, []string{"w1"}); !errors.Is(err, errAgentBlocked) {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

// On a ticket, m goes to its orchestrator, whatever else is open beside it;
// none open, or one at a prompt, is refused.
func TestPickOrchestrator(t *testing.T) {
	run := &ticket.Run{OrchestratorPane: "w1:p2", OrchestratorStatus: "idle"}
	if a, err := pickOrchestrator(run); err != nil || a.PaneID != "w1:p2" {
		t.Fatalf("got %+v, %v", a, err)
	}
	run.OrchestratorStatus = "blocked"
	if _, err := pickOrchestrator(run); !errors.Is(err, errOrchestratorBlocked) {
		t.Fatalf("err = %v, want the blocked refusal", err)
	}
	if _, err := pickOrchestrator(&ticket.Run{}); !errors.Is(err, errNoOrchestrator) {
		t.Fatalf("err = %v, want no orchestrator", err)
	}
}
