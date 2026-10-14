package ticketui

import (
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/ticket"
)

// a asks before it opens the one link of the open questions; n leaves it
// closed, y opens it. The returned command, which would open a browser, is
// not run.
func TestAskConfirmsTheOneLink(t *testing.T) {
	m := boardWith(ticket.Pending)
	m.width, m.height = 120, 30
	m.run.Questions = questions("Which caption? [Chat with Ana](https://chat.example.com/new?to=ana&message=Hi)")
	m.View()
	if cmd := m.key("a"); cmd != nil || m.sure == nil {
		t.Fatal("a opened the link without asking")
	}
	if out := m.View(); !strings.Contains(out, "Open the link?") || !strings.Contains(out, "Chat with Ana (chat.example.com)") {
		t.Fatalf("no question over the board:\n%s", out)
	}
	m.key("x")
	if m.sure == nil {
		t.Fatal("another key closed the question")
	}
	if cmd := m.key("n"); cmd != nil || m.sure != nil || strings.HasPrefix(m.status, "opening") {
		t.Fatalf("n opened the link: %q", m.status)
	}
	m.key("a")
	if cmd := m.key("y"); cmd == nil || m.sure != nil || m.status != "opening Chat with Ana" {
		t.Fatalf("y did not open the link: %q", m.status)
	}
}

// With several links a offers a menu, and choosing one is the yes.
func TestAskOffersAMenuForSeveral(t *testing.T) {
	m := boardWith(ticket.Pending)
	m.width, m.height = 120, 30
	m.run.Questions = questions("One https://a.example.com/x", "Two [Chat with Bo](https://chat.example.com/new?to=bo)")
	m.View()
	m.key("a")
	if m.menu == nil || len(m.menu.Items) != 2 || m.sure != nil {
		t.Fatalf("no menu of the two links: %+v", m.menu)
	}
	m.key("j")
	if cmd := m.key("enter"); cmd == nil || m.status != "opening Chat with Bo" {
		t.Fatalf("enter did not open the chosen link: %q", m.status)
	}
}
