package ticketui

import (
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/ticket"
)

// The closed questions start folded, with their count; c unfolds them, with
// who answered and how, and their links left out; c again folds them.
func TestClosedQuestionsFoldedUntilC(t *testing.T) {
	m := New(nil, "", Settings{})
	m.run = &ticket.Run{
		Questions: questions("Which caption?"),
		Closed:    []string{"Stays hidden? Closed 2026-01-02 by Ana (chat): yes. [Chat with Ana](https://chat.example.com/new?to=ana)"},
	}
	m.width, m.height = 120, 40
	out := m.View()
	if !strings.Contains(out, "Closed questions") || strings.Contains(out, "by Ana") {
		t.Fatalf("closed questions not folded at first:\n%s", out)
	}
	m.key("c")
	out = m.View()
	if !strings.Contains(out, "Closed 2026-01-02 by Ana (chat): yes.") {
		t.Fatalf("c did not unfold the closed questions:\n%s", out)
	}
	if strings.Contains(out, "chat.example.com") || len(m.asks) != 0 {
		t.Fatalf("a closed question's link is drawn or offered: %v\n%s", m.asks, out)
	}
	m.key("c")
	if out = m.View(); strings.Contains(out, "by Ana") {
		t.Fatalf("c again did not fold them:\n%s", out)
	}
}

// With nothing closed there is no section to fold.
func TestNoClosedSectionWhenNoneClosed(t *testing.T) {
	m := New(nil, "", Settings{})
	m.run = &ticket.Run{Questions: questions("Which caption?")}
	m.width, m.height = 120, 40
	if out := m.View(); strings.Contains(out, "Closed questions") {
		t.Fatalf("an empty closed section is drawn:\n%s", out)
	}
}

// A role not started yet is one line, like a finished one, selected or not;
// one that is stuck is a box.
func TestPendingRoleIsALine(t *testing.T) {
	m := &Model{}
	if c := m.roleCard(ticket.Card{}, ticket.Pending, false); !c.Flat {
		t.Fatal("a pending role is drawn as a box")
	}
	if c := m.roleCard(ticket.Card{}, ticket.Pending, true); !c.Flat {
		t.Fatal("the selected pending role is drawn as a box")
	}
	if c := m.roleCard(ticket.Card{}, ticket.Done, true); !c.Flat {
		t.Fatal("the selected finished role is drawn as a box")
	}
	if c := m.roleCard(ticket.Card{Stuck: true}, ticket.Pending, false); c.Flat {
		t.Fatal("a stuck role is not a box")
	}
}
