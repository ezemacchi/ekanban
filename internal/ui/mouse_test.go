package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/store"
)

// findZone renders the board and returns the first zone that matches.
func findZone(t *testing.T, m *Model, match func(zone) bool) zone {
	t.Helper()
	m.View()
	for _, z := range m.zones {
		if match(z) {
			return z
		}
	}
	t.Fatalf("no such zone among %+v", m.zones)
	return zone{}
}

// The status picker opens inside the selected card, s walks the choices and
// enter sets the highlighted one.
func TestPickerOpensInsideTheCard(t *testing.T) {
	m := kanbanBoard(t)
	target := labelsIn(m, "todo")[0]
	selectSpace(t, m, tmp+target)

	send(t, m, key("s"))
	out := m.View()
	if m.mode != modeStatusPick || !strings.Contains(out, "set status:") {
		t.Fatalf("picker not inside the board:\n%s", out)
	}
	if !strings.Contains(out, "Todo 3") {
		t.Fatalf("the board is gone while picking:\n%s", out)
	}
	send(t, m, key("s")) // Todo -> In Progress
	send(t, m, key("l")) // -> Waiting
	send(t, m, key("h")) // -> In Progress
	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.board.Entries[store.Key(tmp+target)].Status; got != "in_progress" {
		t.Fatalf("status is %q, want in_progress", got)
	}
	if m.mode != modeNormal {
		t.Fatalf("mode %v after setting, want normal", m.mode)
	}
}

// A click on a choice inside the expanded card sets it.
func TestClickingAChoiceSetsIt(t *testing.T) {
	m := kanbanBoard(t)
	target := labelsIn(m, "todo")[0]
	selectSpace(t, m, tmp+target)
	send(t, m, key("s"))

	z := findZone(t, m, func(z zone) bool { return z.kind == zoneStatus && z.status == 2 })
	send(t, m, click(z.x0+1, z.y))
	if got := m.board.Entries[store.Key(tmp+target)].Status; got != "waiting" {
		t.Fatalf("status is %q, want waiting", got)
	}
}

// One click selects a card, a second on it acts, like the list.
func TestClickingACardSelectsIt(t *testing.T) {
	m := kanbanBoard(t)
	names := labelsIn(m, "todo")
	selectSpace(t, m, tmp+names[0])

	z := findZone(t, m, func(z zone) bool { return z.kind == zoneCard && z.row == 2 })
	send(t, m, click(z.x0+1, z.y))
	if sp := m.selected(); sp == nil || sp.Label != names[2] {
		t.Fatalf("selected %+v, want %s", sp, names[2])
	}
}

// The footer's hints are buttons: d opens the detail modal, a click outside
// it closes it.
func TestFooterButtonsAndModalClose(t *testing.T) {
	m := kanbanBoard(t)
	selectSpace(t, m, tmp+labelsIn(m, "todo")[0])

	z := findZone(t, m, func(z zone) bool { return z.kind == zoneButton && z.key == "d" })
	if z.y != linesIn(m.View()) {
		t.Fatalf("the d button is on row %d, want the last row %d", z.y, linesIn(m.View()))
	}
	send(t, m, click(z.x0, z.y))
	if m.mode != modeDetail {
		t.Fatalf("mode %v, want the detail modal", m.mode)
	}
	m.View()
	send(t, m, click(0, 0))
	if m.mode != modeNormal {
		t.Fatalf("mode %v after a click outside, want normal", m.mode)
	}
}

// Opened from the detail modal, the picker sits in the modal and returns to
// it once a choice is clicked.
func TestPickerInsideTheDetailModal(t *testing.T) {
	m := kanbanBoard(t)
	target := labelsIn(m, "todo")[0]
	selectSpace(t, m, tmp+target)
	send(t, m, key("d"))
	send(t, m, key("s"))
	if out := m.View(); m.mode != modeStatusPick || !strings.Contains(out, "set status:") {
		t.Fatalf("picker not in the modal:\n%s", out)
	}
	z := findZone(t, m, func(z zone) bool { return z.kind == zoneStatus && z.status == 3 })
	send(t, m, click(z.x0+1, z.y))
	if got := m.board.Entries[store.Key(tmp+target)].Status; got != "done" {
		t.Fatalf("status is %q, want done", got)
	}
	if m.mode != modeDetail {
		t.Fatalf("mode %v, want back in the modal", m.mode)
	}
}

// The status line of running work has a live spinner; the result ends it.
func TestWorkingStatusSpinsUntilReplaced(t *testing.T) {
	m := newTestModel(t)
	tick := m.working("reading things…")
	before := m.statusText()
	msg, ok := tick().(look.SpinMsg)
	if !ok {
		t.Fatal("working did not start the spinner")
	}
	m.spinner.Update(msg, true)
	if after := m.statusText(); before == after || !strings.HasSuffix(after, "reading things…") {
		t.Fatalf("spinner did not move: %q then %q", before, after)
	}
	m.status = "done"
	if m.isBusy() || m.statusText() != "done" {
		t.Fatalf("still busy after the result: %q", m.statusText())
	}
}
