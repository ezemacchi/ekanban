package ticketui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/team"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func clickable(m *Model, match func(zone) bool) (zone, bool) {
	m.View()
	for _, z := range m.zones {
		if match(z) {
			return z, true
		}
	}
	return zone{}, false
}

// A click selects a role card; a second click on it goes to its tab.
func TestClickSelectsThenGoes(t *testing.T) {
	m := boardWith(ticket.Pending, ticket.Pending)
	m.run.Cards[1].Role = team.Role{Label: "Reviewer"}
	m.width = 100
	z, ok := clickable(m, func(z zone) bool { return z.col == 0 && z.row == 1 })
	if !ok {
		t.Fatal("no zone for the second card")
	}
	m.Update(press(z.x0+1, z.y))
	if m.row != 1 {
		t.Fatalf("row %d after the click, want 1", m.row)
	}
	m.Update(press(z.x0+1, z.y))
	if !strings.Contains(m.status, "Reviewer has no agent open") {
		t.Fatalf("the second click did not try to go there: %q", m.status)
	}
}

// The footer's refresh is a button, and its status spins until the load
// lands.
func TestRefreshButtonSpinsUntilLoaded(t *testing.T) {
	m := boardWith(ticket.Pending)
	m.width = 100
	z, ok := clickable(m, func(z zone) bool { return z.col < 0 && z.key == "r" })
	if !ok {
		t.Fatal("no refresh button")
	}
	m.Update(press(z.x0, z.y))
	if !m.isBusy() || !strings.HasSuffix(m.statusText(), refreshing) || m.statusText() == refreshing {
		t.Fatalf("refresh is not spinning: %q", m.statusText())
	}
	m.Update(loadedMsg{run: m.run})
	if m.isBusy() || m.status != "" {
		t.Fatalf("still refreshing after the load: %q", m.status)
	}
}
