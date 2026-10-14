package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/herdrtest"
)

// m with no orchestrator asks which agent gets the message, keeping it; an
// agent at a prompt cannot be chosen; the one chosen gets it typed, unsent.
func TestMessageAsksWhichAgentWhenThereIsNoOrchestrator(t *testing.T) {
	f := herdrtest.Start(t)
	f.OK(map[string]any{"type": "ok"})
	c, _ := herdr.New()
	m := pipelineBoard(t)
	m.client = c
	m.width, m.height = 140, 40

	send(t, m, needAgentMsg{ws: []string{"w1"}, text: "rebase please", label: "ABC-1", why: "no orchestrator open"})
	if m.mode != modeChooser {
		t.Fatalf("mode %v, want the chooser", m.mode)
	}
	send(t, m, pickRowsMsg{rows: []pickRow{
		{pane: "w1:p1", tab: "2", agent: "claude", status: "blocked"},
		{pane: "w1:p2", tab: "3", agent: "cursor", status: "idle", title: "Planning"},
	}})
	if m.ap.ch.Sel != 1 {
		t.Fatalf("the highlight starts on %d, want the first that can be chosen", m.ap.ch.Sel)
	}
	send(t, m, previewMsg{pane: "w1:p2", text: "\x1b[2Jlast words\n"})
	out := ansi.Strip(m.View())
	for _, want := range []string{"ABC-1: no orchestrator open", "tab 2", "waiting at a prompt", "tab 3", "last words"} {
		if !strings.Contains(out, want) {
			t.Fatalf("chooser is missing %q:\n%s", want, out)
		}
	}

	send(t, m, key("k"))
	_, cmd := m.Update(key("enter"))
	if m.mode != modeChooser || cmd != nil {
		t.Fatal("an agent at a prompt was used")
	}
	send(t, m, key("j"))
	_, cmd = m.Update(key("enter"))
	if m.mode != modeNormal || cmd == nil {
		t.Fatalf("enter did not use the agent; mode %v", m.mode)
	}
	if sent, ok := cmd().(agentSentMsg); !ok || sent.pane != "w1:p2" {
		t.Fatalf("result %#v", sent)
	}
	params := herdrtest.Params(t, f.Last(t, "pane.send_text"))
	if params["pane_id"] != "w1:p2" || params["text"] != "rebase please" {
		t.Fatalf("params = %+v", params)
	}
}

// esc closes the chooser and sends nothing.
func TestChooserEscSendsNothing(t *testing.T) {
	m := pipelineBoard(t)
	send(t, m, needAgentMsg{ws: []string{"w1"}, text: "hi", label: "ABC-1", why: "no orchestrator open"})
	send(t, m, key("esc"))
	if m.mode != modeNormal || m.status != "nothing was sent" {
		t.Fatalf("mode %v, status %q", m.mode, m.status)
	}
}

// A preview read for an agent no longer highlighted is dropped.
func TestStalePreviewIsDropped(t *testing.T) {
	m := pipelineBoard(t)
	send(t, m, needAgentMsg{ws: []string{"w1"}, text: "hi", label: "ABC-1", why: "x"})
	send(t, m, pickRowsMsg{rows: []pickRow{{pane: "a", status: "idle"}, {pane: "b", status: "idle"}}})
	send(t, m, previewMsg{pane: "b", text: "from b"})
	if len(m.ap.ch.Preview) != 0 {
		t.Fatalf("a stale preview was kept: %q", m.ap.ch.Preview)
	}
}

// In the broadcast, a ticket with no orchestrator is listed unticked; space
// on it asks which agent, and the one chosen gets this broadcast.
func TestBroadcastPicksAnAgentForATicketWithNoOrchestrator(t *testing.T) {
	m := pipelineBoard(t)
	m.width, m.height = 140, 40
	send(t, m, key("B"))
	send(t, m, recipientsMsg{to: []recipient{
		{key: "ABC-1", pane: "p1", status: "idle", on: true},
		{key: "ABC-2", ws: []string{"w2"}},
	}})
	if out := ansi.Strip(m.View()); !strings.Contains(out, "[ ] ABC-2") || !strings.Contains(out, "space picks an agent") {
		t.Fatalf("the ticket with no orchestrator is not listed:\n%s", out)
	}
	send(t, m, key("tab"))
	send(t, m, key("a"))
	if len(m.chosen()) != 1 {
		t.Fatal("All ticked a ticket with no orchestrator")
	}
	send(t, m, key("j"))
	send(t, m, key(" "))
	if m.mode != modeChooser || m.ap.back != modeBroadcast {
		t.Fatalf("space did not ask which agent; mode %v", m.mode)
	}
	send(t, m, pickRowsMsg{rows: []pickRow{{pane: "w2:p4", tab: "3", agent: "cursor", status: "idle"}}})
	send(t, m, key("enter"))
	if m.mode != modeBroadcast {
		t.Fatalf("mode %v, want back in the broadcast", m.mode)
	}
	r := m.bc.to[1]
	if r.pane != "w2:p4" || !r.on || r.via != "tab 3" {
		t.Fatalf("recipient %+v", r)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "to tab 3, this once") {
		t.Fatalf("the list does not say who gets it:\n%s", out)
	}
}

// The preview reads the pane's recent lines with their colours.
func TestReadPaneAsksForRecentANSI(t *testing.T) {
	f := herdrtest.Start(t)
	f.OK(map[string]any{"type": "pane_read", "read": map[string]any{"text": "hello"}})
	c, _ := herdr.New()
	text, err := c.ReadPane("w1:p1", 40)
	if err != nil || text != "hello" {
		t.Fatalf("got %q, %v", text, err)
	}
	p := herdrtest.Params(t, f.Last(t, "pane.read"))
	if p["pane_id"] != "w1:p1" || p["source"] != "recent" || p["format"] != "ansi" || p["lines"] != float64(40) {
		t.Fatalf("params = %+v", p)
	}
}
