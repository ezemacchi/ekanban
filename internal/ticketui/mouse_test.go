package ticketui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/team"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func clickable(m *Model, match func(screen.Zone) bool) (screen.Zone, bool) {
	m.View()
	for _, z := range m.zones {
		if match(z) {
			return z, true
		}
	}
	return screen.Zone{}, false
}

// A click selects a role card; a second click on it goes to its tab.
func TestClickSelectsThenGoes(t *testing.T) {
	m := boardWith(ticket.Pending, ticket.Pending)
	m.run.Cards[1].Role = team.Role{Label: "Reviewer"}
	m.width = 100
	z, ok := clickable(m, func(z screen.Zone) bool { return z.Kind == screen.OnCard && z.Col == 0 && z.Row == 1 })
	if !ok {
		t.Fatal("no zone for the second card")
	}
	m.Update(press(z.X0+1, z.Y))
	if m.row != 1 {
		t.Fatalf("row %d after the click, want 1", m.row)
	}
	m.Update(press(z.X0+1, z.Y))
	if !strings.Contains(m.status, "Reviewer has no agent open") {
		t.Fatalf("the second click did not try to go there: %q", m.status)
	}
}

// The footer's refresh is a button, and its status spins until the load
// lands.
func TestRefreshButtonSpinsUntilLoaded(t *testing.T) {
	m := boardWith(ticket.Pending)
	m.width = 100
	z, ok := clickable(m, func(z screen.Zone) bool { return z.Kind == screen.OnButton && z.Key == "r" })
	if !ok {
		t.Fatal("no refresh button")
	}
	m.Update(press(z.X0, z.Y))
	if !m.isBusy() || !strings.HasSuffix(m.statusText(), refreshing) || m.statusText() == refreshing {
		t.Fatalf("refresh is not spinning: %q", m.statusText())
	}
	m.Update(loadedMsg{run: m.run})
	if m.isBusy() || m.status != "" {
		t.Fatalf("still refreshing after the load: %q", m.status)
	}
}

// tall is a board taller than its pane: many roles in one column and many
// open questions.
func tall() *Model {
	cols := make([]ticket.Column, 12)
	for i := range cols {
		cols[i] = ticket.Pending
	}
	m := boardWith(cols...)
	for i := range 20 {
		m.run.Questions = append(m.run.Questions, strings.Repeat("q", i+1))
	}
	m.run.Key = "KEY-1"
	m.width, m.height = 100, 20
	return m
}

// A board taller than its pane fits it: the footer stays on the last line and
// the wheel brings the top back.
func TestTallBoardScrollsAndKeepsTheFooter(t *testing.T) {
	m := tall()
	m.follow = true
	m.row = 11 // the last role: following it scrolls the body down
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Fatalf("%d lines in a %d-line pane", len(lines), m.height)
	}
	if !strings.Contains(lines[0], "KEY-1") {
		t.Fatalf("the header scrolled away: %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "Quit") {
		t.Fatalf("the buttons are not on the last line: %q", lines[len(lines)-1])
	}
	if m.scroll == 0 {
		t.Fatal("the selected last role did not scroll the body")
	}
	for range 20 {
		m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	}
	m.View()
	if m.scroll != 0 {
		t.Fatalf("the wheel did not bring the top back: scroll %d", m.scroll)
	}
	for _, z := range m.zones {
		if z.Y >= m.height {
			t.Fatalf("a zone below the pane: %+v", z)
		}
		if z.Kind == screen.OnCard && (z.Y < headerLines || z.Y >= m.height-footerLines) {
			t.Fatalf("a card zone outside the body: %+v", z)
		}
	}
}

// ? opens the help over the board, with the board's keys in sections, and
// any key or a click closes it.
func TestHelpOpensOverTheBoard(t *testing.T) {
	m := boardWith(ticket.Pending)
	m.width, m.height = 120, 30
	m.key("?")
	out := m.View()
	for _, want := range []string{"Help", "The selected role", "go to the role's tab", "╭"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help is missing %q:\n%s", want, out)
		}
	}
	m.key("x")
	if m.helping {
		t.Fatal("a key did not close the help")
	}
	m.key("?")
	m.Update(press(1, 1))
	if m.helping {
		t.Fatal("a click did not close the help")
	}
}
