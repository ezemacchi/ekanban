package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/store"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// questionBoard is the pipeline board with ABC-1's run asking two questions,
// the first with a chat to answer it in.
func questionBoard(t *testing.T) *Model {
	t.Helper()
	m := pipelineBoard(t)
	m.width, m.height = 140, 40
	api := store.Key(tmp + "api")
	info := m.pipeInfo[api]
	info.Questions = []ticket.Question{
		ticket.NewQuestion("Product: is the hidden Ready remembered per browser acceptable? [Chat with Ana](https://chat.example.com/new?to=ana)"),
		ticket.NewQuestion("Technical: which storage key?"),
	}
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: info, store.Key(tmp + "web"): {}}})
	m.restoreCursor(api)
	m.restoreColumnCursor(api)
	return m
}

// A card whose run has open questions says how many in its border, so the
// board shows them without opening each ticket.
func TestCardMarksItsOpenQuestions(t *testing.T) {
	m := questionBoard(t)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "?2") {
		t.Fatalf("no question mark on the card:\n%s", out)
	}
	if !strings.Contains(out, "2 questions · Q reads them") {
		t.Fatalf("the selected card does not say how to read them:\n%s", out)
	}
}

// Q lists the questions with the whole of the highlighted one beside them;
// enter asks before opening the chat it carries, and no comes back.
func TestQReadsTheQuestionsAndOpensTheChat(t *testing.T) {
	m := questionBoard(t)
	send(t, m, key("Q"))
	if m.mode != modeChooser {
		t.Fatalf("Q left mode %v", m.mode)
	}
	out := ansi.Strip(m.View())
	for _, want := range []string{"ABC-1 · open questions", "Technical: which storage key?", "remembered per browser acceptable?", "↗ Chat with Ana (chat.example.com)", "⏎ Open", "r Reply", "D Discuss"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	send(t, m, key("enter"))
	if m.mode != modeConfirm || m.ask == nil || m.ask.lines[0] != "Chat with Ana (chat.example.com)" {
		t.Fatalf("enter did not ask to open the chat; mode %v", m.mode)
	}
	send(t, m, key("n"))
	if m.mode != modeChooser {
		t.Fatalf("no did not go back to the questions; mode %v", m.mode)
	}
	send(t, m, key("enter"))
	_, cmd := m.Update(key("y"))
	if m.mode != modeNormal || cmd == nil || m.status != "opening Chat with Ana" {
		t.Fatalf("yes did not open the chat; mode %v, status %q", m.mode, m.status)
	}
}

// A question with no link stays in the chooser and says so.
func TestAQuestionWithNoLinkSaysSo(t *testing.T) {
	m := questionBoard(t)
	send(t, m, key("Q"))
	send(t, m, key("j"))
	send(t, m, key("enter"))
	if m.mode != modeChooser || m.status != "the question carries no link" {
		t.Fatalf("mode %v, status %q", m.mode, m.status)
	}
}

// A question for the user turns the card red, like an agent asking, and !
// finds it; the questions reader lists it first.
func TestAQuestionForYouAsksForYou(t *testing.T) {
	m := questionBoard(t)
	api := store.Key(tmp + "api")
	info := m.pipeInfo[api]
	mine := ticket.NewQuestion("Which model runs the implementer?")
	mine.To = ticket.You
	info.ForYou = []ticket.Question{mine}
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: info, store.Key(tmp + "web"): {}}})
	m.restoreCursor(api)
	m.restoreColumnCursor(api)
	if why := m.attention(m.selected()); !strings.Contains(why, "1 question for you") {
		t.Fatalf("the card does not ask for you: %q", why)
	}
	if got := m.needingYou(); len(got) != 1 || got[0].Key != api {
		t.Fatalf("! does not find it: %v", got)
	}
	send(t, m, key("Q"))
	if m.ap.ch.Sel != 0 || !strings.Contains(ansi.Strip(m.ap.ch.Rows[0].Text), "Which model runs the implementer?") {
		t.Fatalf("the question for you is not first: %+v", m.ap.ch.Rows)
	}
	// r there replies (the m path) and closes the reader.
	send(t, m, key("r"))
	if m.mode != modeNormal {
		t.Fatalf("r in the reader left mode %v", m.mode)
	}
}

// Q is the questions scope's: on a ticket with open questions the footer
// offers it and it reads them; on one without, there is no hint and Q does
// nothing.
func TestQShowsWhereThereAreQuestions(t *testing.T) {
	m := questionBoard(t)
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Q Questions") {
		t.Fatalf("no Q hint on a ticket with questions:\n%s", out)
	}

	bare := pipelineBoard(t)
	bare.width, bare.height = 140, 40
	bare.restoreCursor(store.Key(tmp + "api"))
	bare.restoreColumnCursor(store.Key(tmp + "api"))
	if out := ansi.Strip(bare.View()); strings.Contains(out, "Questions") {
		t.Fatalf("a Q hint on a ticket without questions:\n%s", out)
	}
	send(t, bare, key("Q"))
	if bare.mode != modeNormal {
		t.Fatalf("Q did something on a ticket without questions: mode %v", bare.mode)
	}
}

// Rebinding questions moves the footer's hint and the card's line with it.
func TestRebindingQuestionsMovesItsHints(t *testing.T) {
	m := questionBoard(t)
	m.keys.pipeline, _ = keys.New(PipelineActions, map[string][]string{"questions": {"W"}})
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "W Questions") || !strings.Contains(out, "W reads them") || strings.Contains(out, "Q Questions") {
		t.Fatalf("hints do not follow the binding:\n%s", out)
	}
	send(t, m, key("W"))
	if m.mode != modeChooser {
		t.Fatalf("W did not read the questions: mode %v", m.mode)
	}
}
