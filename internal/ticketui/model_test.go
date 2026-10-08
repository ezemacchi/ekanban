package ticketui

import (
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

func boardWith(cols ...ticket.Column) *Model {
	r := &ticket.Run{}
	for _, c := range cols {
		r.Cards = append(r.Cards, ticket.Card{Column: c})
	}
	m := &Model{run: r}
	m.clamp()
	return m
}

func TestColumnStepsSkipEmptyColumns(t *testing.T) {
	m := boardWith(ticket.Pending, ticket.Done)
	if m.col != 0 {
		t.Fatalf("start col %d", m.col)
	}
	m.key("l")
	if m.col != 3 {
		t.Fatalf("l should jump over empty columns to Done, got %d", m.col)
	}
	m.key("l")
	if m.col != 3 {
		t.Fatalf("l with nothing further right must stay, got %d", m.col)
	}
	m.key("h")
	if m.col != 0 {
		t.Fatalf("h should jump back to Pending, got %d", m.col)
	}
	m.key("h")
	if m.col != 0 {
		t.Fatalf("h with nothing further left must stay, got %d", m.col)
	}
}

// Reordered columns move the cards with them: Done shown first holds the
// finished role, whatever position it had by default.
func TestConfiguredColumnOrderCarriesTheCards(t *testing.T) {
	cols, _ := columns.Merge(ticket.DefaultColumns, []columns.Column{{ID: "done", Label: "Finished"}}, false)
	m := &Model{run: &ticket.Run{Cards: []ticket.Card{{Column: ticket.Done}}}, cols: cols, width: 100}
	m.clamp()
	if m.col != 0 || m.columnAt(0) != ticket.Done {
		t.Fatalf("cursor col %d, first column %v", m.col, m.columnAt(0))
	}
	if out := m.View(); !strings.Contains(out, "Finished 1") {
		t.Fatalf("renamed column missing:\n%s", out)
	}
}

// [keys] right = "n": n moves right, l no longer does.
func TestReboundColumnKey(t *testing.T) {
	m := boardWith(ticket.Pending, ticket.Done)
	m.keys, _ = keys.New(Actions, map[string][]string{"right": {"n"}})
	m.key("l")
	if m.col != 0 {
		t.Fatalf("l still moves after right moved to n, col %d", m.col)
	}
	m.key("n")
	if m.col != 3 {
		t.Fatalf("n did not move right, col %d", m.col)
	}
}

func TestCursorLeavesColumnEmptiedByRefresh(t *testing.T) {
	m := boardWith(ticket.Done)
	if m.col != 3 {
		t.Fatalf("an empty first column must not hold the cursor, got %d", m.col)
	}
}
