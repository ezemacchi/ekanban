package ticketui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// With the orchestrator running the board still closes it itself, once you
// say yes: nothing is typed into the orchestrator, and the question says it
// will read it closed.
func TestCloseNeverAsksTheOrchestrator(t *testing.T) {
	m, f := forYouBoard(t)
	dir := t.TempDir()
	line := "Technical (Bo): which key?"
	if err := os.WriteFile(filepath.Join(dir, "STATE.md"), []byte("## Open decisions\n- "+line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.run.Dir = dir
	m.View()
	m.onQuestion, m.q = true, 3 // Bo's, after the one for you and Ana's two
	m.key("x")
	if m.closing == nil || !strings.Contains(m.View(), "no longer needed") {
		t.Fatal("x did not ask why")
	}
	if cmd := m.closeKey(screen.KeyMsg("enter")); cmd != nil || m.closing == nil || !m.closing.sure {
		t.Fatal("enter did not ask yes or no")
	}
	if !strings.Contains(m.View(), "The orchestrator is running") {
		t.Fatal("the question does not say the orchestrator reads it closed")
	}
	cmd := m.closeKey(screen.KeyMsg("y"))
	if msg, _ := cmd().(closedMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	if f.Called("pane.send_text") != 0 || f.Called("agent.prompt") != 0 {
		t.Fatal("the orchestrator was asked")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "STATE.md")); !strings.Contains(string(data), "not answered: no longer needed.") {
		t.Fatalf("state:\n%s", data)
	}
}

// With no orchestrator the board closes it itself, once you say yes.
func TestCloseInTheRunWithNoOrchestrator(t *testing.T) {
	dir := t.TempDir()
	state := "## Open decisions\n- Technical (Bo): which key?\n"
	if err := os.WriteFile(filepath.Join(dir, "STATE.md"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	m := questionBoard()
	m.run.Dir = dir
	m.run.Questions = []ticket.Question{ticket.NewQuestion("Technical (Bo): which key?")}
	m.width, m.height = 120, 50
	m.View()
	m.onQuestion, m.q = true, 0
	m.key("x")
	for _, r := range " QA" {
		m.closeKey(screen.KeyMsg(string(r)))
	}
	m.closeKey(screen.KeyMsg("enter"))
	if m.closing == nil || !m.closing.sure || !strings.Contains(m.View(), "Close it without an answer?") {
		t.Fatal("no yes-or-no before writing the run")
	}
	if m.closeKey(screen.KeyMsg("n")); m.closing != nil {
		t.Fatal("n did not leave it")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "STATE.md")); string(data) != state {
		t.Fatal("no wrote the run")
	}
	m.key("x")
	m.closeKey(screen.KeyMsg("enter"))
	cmd := m.closeKey(screen.KeyMsg("y"))
	if cmd == nil {
		t.Fatal("yes did not close it")
	}
	msg, _ := cmd().(closedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "STATE.md"))
	if !strings.Contains(string(data), "## Decisions") || !strings.Contains(string(data), "which key? Closed ") || !strings.Contains(string(data), "not answered: no longer needed.") {
		t.Fatalf("state:\n%s", data)
	}
}
