package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/herdrtest"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// x in the questions reader asks why the question is closed, "no longer
// needed" already written; esc changes nothing.
func TestXInTheReaderAsksWhy(t *testing.T) {
	m := questionBoard(t)
	send(t, m, key("Q"))
	send(t, m, key("x"))
	if m.mode != modeCloseReason || m.input.Value() != closeReasonDefault {
		t.Fatalf("mode %v, reason %q", m.mode, m.input.Value())
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Close the question") || !strings.Contains(out, "Why it no longer needs an answer") {
		t.Fatalf("no box:\n%s", out)
	}
	send(t, m, key("esc"))
	if m.mode != modeNormal || m.status != "nothing was changed" {
		t.Fatalf("esc: mode %v, status %q", m.mode, m.status)
	}
}

// The board closes it in the run once you say yes, an orchestrator running or
// not: closing needs no agent.
func TestClosePlan(t *testing.T) {
	dir := t.TempDir()
	state := "## Open decisions\n- Technical (Bo): which key?\n"
	if err := os.WriteFile(filepath.Join(dir, "STATE.md"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	run := &ticket.Run{Key: "ABC-1", Dir: dir}
	q := ticket.NewQuestion("Technical (Bo): which key?")

	m := questionBoard(t)
	if cmd := m.applyClosePlan(closePlanMsg{run: run, q: q, reason: "ticket already in QA"}); cmd != nil || m.mode != modeConfirm {
		t.Fatalf("no question before writing the run; mode %v", m.mode)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Close it without an answer?") || !strings.Contains(out, "not answered: ticket already in QA.") {
		t.Fatalf("the question does not say what changes:\n%s", out)
	}
	_, cmd := m.Update(key("y"))
	if cmd == nil {
		t.Fatal("yes did nothing")
	}
	send(t, m, cmd())
	data, _ := os.ReadFile(filepath.Join(dir, "STATE.md"))
	if !strings.Contains(string(data), "## Decisions") || !strings.Contains(m.status, "closed without an answer") {
		t.Fatalf("status %q, state:\n%s", m.status, data)
	}

	f := herdrtest.Start(t)
	f.OK(map[string]any{"type": "ok"})
	m.client, _ = herdr.New()
	run.OrchestratorPane, run.OrchestratorStatus = "w1:p1", "idle"
	// With the orchestrator running it is the same: asked here, written here,
	// nothing sent to the orchestrator.
	if cmd = m.applyClosePlan(closePlanMsg{run: run, q: q, reason: "no longer needed"}); cmd != nil || m.mode != modeConfirm {
		t.Fatalf("no question, or something sent; mode %v", m.mode)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "The orchestrator is running") {
		t.Fatalf("the question does not say the orchestrator reads it closed:\n%s", out)
	}
	if f.Called("pane.send_text") != 0 || f.Called("agent.prompt") != 0 {
		t.Fatal("the orchestrator was asked")
	}
}
