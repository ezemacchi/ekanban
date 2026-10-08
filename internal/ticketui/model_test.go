package ticketui

import (
	"testing"

	"github.com/phin-tech/herdr-phin-board/internal/ticket"
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

func TestCursorLeavesColumnEmptiedByRefresh(t *testing.T) {
	m := boardWith(ticket.Done)
	if m.col != 3 {
		t.Fatalf("an empty first column must not hold the cursor, got %d", m.col)
	}
}
