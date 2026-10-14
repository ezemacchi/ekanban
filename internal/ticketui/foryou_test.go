package ticketui

import (
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/herdrtest"
	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// forYouBoard has a question for the user, two for Ana and one for Bo, an
// orchestrator open, and Herdr faked.
func forYouBoard(t *testing.T) (*Model, *herdrtest.Fake) {
	t.Helper()
	f := herdrtest.Start(t)
	f.OK(map[string]any{"type": "ok"})
	c, err := herdr.New()
	if err != nil {
		t.Fatal(err)
	}
	m := questionBoard()
	m.client = c
	m.width, m.height = 120, 60
	mine := ticket.NewQuestion("Which model runs the implementer?")
	mine.To = ticket.You
	m.run.ForYou = []ticket.Question{mine}
	bo := ticket.NewQuestion("Technical (Bo): which key?")
	ana := ticket.NewQuestion("Product (Ana): which caption?")
	ana.FromWorktree = ".runs/ABC-1/questions/which-caption.md"
	m.run.Questions = []ticket.Question{bo, ana, ticket.NewQuestion("Product (Ana): which colour?")}
	m.run.OrchestratorPane, m.run.OrchestratorStatus = "w1:p1", "idle"
	return m, f
}

// The questions for the user come first, in a section of their own; the
// others are grouped under who answers them; the cursor goes in that order.
func TestQuestionsForYouComeFirst(t *testing.T) {
	m, _ := forYouBoard(t)
	out := m.View()
	you, open := strings.Index(out, "Questions for you"), strings.Index(out, "Open questions")
	if you < 0 || open < you {
		t.Fatalf("no section for you above the others:\n%s", out)
	}
	ana, bo := strings.Index(out, "Ana\n"), strings.Index(out, "Bo\n")
	if ana < 0 || bo < ana {
		t.Fatalf("the others are not grouped by who answers them:\n%s", out)
	}
	m.onQuestion, m.q = true, 0
	if q, kind, _ := m.selectedQuestion(); kind != kindForYou || q.Words != "Which model runs the implementer?" {
		t.Fatalf("first question %+v %v", q, kind)
	}
	m.q = 1
	if q, _, _ := m.selectedQuestion(); q.To != "Ana" {
		t.Fatalf("second question %+v, want Ana's first", q)
	}
}

// r on a question types into the orchestrator the opening of an answer
// naming the question's brief, unsent, and goes there.
func TestReplyTypesIntoTheOrchestrator(t *testing.T) {
	m, f := forYouBoard(t)
	m.View()
	m.onQuestion, m.q = true, 1 // Ana's, with a brief
	cmd := m.key("r")
	if cmd == nil {
		t.Fatalf("r did not reply: %q", m.status)
	}
	cmd()
	p := herdrtest.Params(t, f.Last(t, "pane.send_text"))
	if p["pane_id"] != "w1:p1" || p["text"] != "Answer from Ana to the open question .runs/ABC-1/questions/which-caption.md: " {
		t.Fatalf("typed %+v", p)
	}
	if f.Called("agent.focus") != 1 {
		t.Fatal("did not go to the orchestrator")
	}
	if f.Called("agent.prompt") != 0 {
		t.Fatal("the reply was submitted, not left for the user to finish")
	}
}

// enter on a question for the user replies: it has no chat.
func TestEnterOnAQuestionForYouReplies(t *testing.T) {
	m, f := forYouBoard(t)
	m.View()
	m.onQuestion, m.q = true, 0
	cmd := m.key("enter")
	if cmd == nil || m.sure != nil {
		t.Fatalf("enter did not reply: %q", m.status)
	}
	cmd()
	if p := herdrtest.Params(t, f.Last(t, "pane.send_text")); p["text"] != `My answer to the open question "Which model runs the implementer?": ` {
		t.Fatalf("typed %+v", p)
	}
}

// No reply into an orchestrator waiting at a prompt, or with none open.
func TestReplyRefusesWithoutAnOrchestratorToTake(t *testing.T) {
	m, _ := forYouBoard(t)
	m.onQuestion, m.q = true, 1
	m.run.OrchestratorStatus = "blocked"
	if m.key("r") != nil || !strings.Contains(m.status, "waiting at a prompt") {
		t.Fatalf("blocked: %q", m.status)
	}
	m.run.OrchestratorPane = ""
	if m.key("r") != nil || !strings.Contains(m.status, "no orchestrator") {
		t.Fatalf("none: %q", m.status)
	}
}

// Reply can be bound elsewhere; then r refreshes on a question too.
func TestReplyCanBeRebound(t *testing.T) {
	m, f := forYouBoard(t)
	m.keys, _ = keys.New(Actions, map[string][]string{"reply": {"x"}})
	m.onQuestion, m.q = true, 1
	if !strings.Contains(m.View(), "x Reply") {
		t.Fatal("the button does not show the bound key")
	}
	if cmd := m.key("x"); cmd == nil {
		t.Fatalf("x did not reply: %q", m.status)
	} else {
		cmd()
	}
	if f.Called("pane.send_text") != 1 {
		t.Fatal("nothing typed")
	}
	m.key("r")
	if !m.isBusy() {
		t.Fatalf("r did not refresh: %q", m.status)
	}
}

// A question something is wrong with says so, and the footer counts them.
func TestAProblemIsShown(t *testing.T) {
	m, _ := forYouBoard(t)
	m.run.Questions[0].Problem = "its brief is still a draft"
	out := m.View()
	if !strings.Contains(out, "1 question has a problem") {
		t.Fatalf("no count in the footer:\n%s", out)
	}
	m.onQuestion, m.q = true, 3 // Bo's, after the one for you and Ana's two
	if out := m.View(); !strings.Contains(out, "its brief is still a draft") {
		t.Fatalf("the selected question does not say what is wrong:\n%s", out)
	}
}

// D needs a brief.
func TestDiscussNeedsABrief(t *testing.T) {
	m, _ := forYouBoard(t)
	m.onQuestion, m.q = true, 1
	if m.key("D") != nil || !strings.Contains(m.status, "no brief") {
		t.Fatalf("status %q", m.status)
	}
}

// The footer's enter says what enter does: Reply on a question for you.
func TestFooterSaysEnterReplies(t *testing.T) {
	m, _ := forYouBoard(t)
	m.onQuestion, m.q = true, 0
	if out := m.View(); !strings.Contains(out, "⏎ Reply") {
		t.Fatalf("footer:\n%s", out)
	}
}
