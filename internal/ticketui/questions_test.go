package ticketui

import (
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

func questionBoard() *Model {
	m := boardWith(ticket.Pending, ticket.Pending)
	m.width, m.height = 80, 40
	m.run.Questions = questions(
		"Product: is the hidden Ready remembered per browser acceptable, or must it follow the person to another computer as well? [Chat with Ana](https://chat.example.com/new?to=ana)",
		"Technical: which storage key?",
	)
	return m
}

// j past the column's last role goes on to the first open question, and k
// on it comes back to the roles.
func TestCursorGoesFromRolesToQuestions(t *testing.T) {
	m := questionBoard()
	m.View()
	m.key("j")
	if m.onQuestion || m.row != 1 {
		t.Fatalf("j left the roles too soon: row %d, on a question %v", m.row, m.onQuestion)
	}
	m.key("j")
	if !m.onQuestion || m.q != 0 {
		t.Fatalf("j past the last role did not select the first question: %v %d", m.onQuestion, m.q)
	}
	m.key("j")
	m.key("j")
	if m.q != 1 {
		t.Fatalf("j went past the last question: %d", m.q)
	}
	m.key("k")
	m.key("k")
	if m.onQuestion || m.row != 1 {
		t.Fatalf("k on the first question did not come back: %v row %d", m.onQuestion, m.row)
	}
}

// Folded, the open questions unfold when the cursor goes down to them.
func TestCursorUnfoldsTheQuestions(t *testing.T) {
	m := questionBoard()
	m.toggle(openFold)
	m.key("j")
	m.key("j")
	if !m.onQuestion || m.isFolded(openFold) {
		t.Fatalf("the folded questions were not opened: %v", m.onQuestion)
	}
}

// The selected question is drawn whole, with its buttons; the others keep to
// one line. enter asks to open its link.
func TestSelectedQuestionIsWholeAndAsks(t *testing.T) {
	m := questionBoard()
	if out := m.View(); strings.Contains(out, "another computer as well") {
		t.Fatalf("a question at rest is drawn whole:\n%s", out)
	}
	m.onQuestion, m.q = true, 0
	out := m.View()
	if !strings.Contains(out, "another computer as well") || !strings.Contains(out, "❯") || !strings.Contains(out, "Copy") {
		t.Fatalf("the selected question is not whole, marked and with its buttons:\n%s", out)
	}
	if cmd := m.key("enter"); cmd != nil || m.sure == nil || m.sure.Label != "Chat with Ana" {
		t.Fatalf("enter did not ask to open the question's link: %+v", m.sure)
	}
}

// A click on a question selects it.
func TestClickSelectsAQuestion(t *testing.T) {
	m := questionBoard()
	z, ok := clickable(m, func(z screen.Zone) bool { return z.Kind == screen.OnControl && z.ID == "q:1" })
	if !ok {
		t.Fatal("no zone for the second question")
	}
	m.Update(press(z.X0+1, z.Y))
	if !m.onQuestion || m.q != 1 {
		t.Fatalf("the click did not select the question: %v %d", m.onQuestion, m.q)
	}
}

// y copies the selected question as the run wrote it.
func TestYankCopiesTheQuestion(t *testing.T) {
	var copied string
	old := clipboardWrite
	clipboardWrite = func(s string) error { copied = s; return nil }
	t.Cleanup(func() { clipboardWrite = old })
	m := questionBoard()
	m.onQuestion, m.q = true, 1
	m.key("y")
	if copied != "Technical: which storage key?" {
		t.Fatalf("copied %q", copied)
	}
}
