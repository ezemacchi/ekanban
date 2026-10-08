package main

import (
	"testing"

	"github.com/ezemacchi/ekanban/internal/herdr"
)

func TestChooseOpening(t *testing.T) {
	agent := "cursor"
	shell := herdr.Pane{PaneID: "w:p1"}
	inAgent := herdr.Pane{PaneID: "w:p2", Agent: &agent}
	withBoard := []herdr.Tab{{ID: "w:t1", Label: "2"}, {ID: "w:t9", Label: "Board"}}
	plain := []herdr.Tab{{ID: "w:t1", Label: "2"}}

	cases := []struct {
		name  string
		tabs  []herdr.Tab
		pane  herdr.Pane
		where opening
		tab   string
	}{
		{"an existing board tab wins", withBoard, inAgent, toBoardTab, "w:t9"},
		{"from an agent, a new tab", plain, inAgent, inNewTab, ""},
		{"anywhere else, over the pane", plain, shell, overPane, ""},
	}
	for _, c := range cases {
		where, tab := chooseOpening(c.tabs, nil, c.pane, false)
		if where != c.where || tab != c.tab {
			t.Errorf("%s: got %v %q, want %v %q", c.name, where, tab, c.where, c.tab)
		}
	}
}

// In a ticket's workspace both kinds of board are labelled "board". The key
// must land on the ticket's, never on the space kanban that happens to be
// there, and must make the ticket's when it is missing.
func TestInATicketWorkspaceOnlyTheTicketBoardCounts(t *testing.T) {
	agent := "cursor"
	inAgent := herdr.Pane{PaneID: "w:p4", Agent: &agent}
	tabs := []herdr.Tab{{ID: "w:t4", Label: "orchestrator"}, {ID: "w:t9", Label: "board"}}
	spaceBoard := []herdr.Pane{{PaneID: "w:p9", TabID: "w:t9", Label: "Board"}}
	ticketBoard := []herdr.Pane{{PaneID: "w:p9", TabID: "w:t9", Label: "Ticket"}}

	if where, tab := chooseOpening(tabs, spaceBoard, inAgent, true); where != inNewTab || tab != "" {
		t.Fatalf("the space kanban was taken for the ticket's: %v %q", where, tab)
	}
	if where, tab := chooseOpening(tabs, ticketBoard, inAgent, true); where != toBoardTab || tab != "w:t9" {
		t.Fatalf("the ticket board was not found: %v %q", where, tab)
	}
	// Outside a ticket's workspace a board tab is a board tab, as before.
	if where, tab := chooseOpening(tabs, spaceBoard, inAgent, false); where != toBoardTab || tab != "w:t9" {
		t.Fatalf("the space kanban is the right one here: %v %q", where, tab)
	}

	// Both present: the ticket's is chosen whatever the order.
	both := []herdr.Tab{{ID: "w:t5", Label: "board"}, {ID: "w:t9", Label: "board"}}
	panes := []herdr.Pane{{TabID: "w:t5", Label: "Board"}, {TabID: "w:t9", Label: "Ticket"}}
	if where, tab := chooseOpening(both, panes, inAgent, true); where != toBoardTab || tab != "w:t9" {
		t.Fatalf("picked the wrong one of two: %v %q", where, tab)
	}
}

func TestNewTabs(t *testing.T) {
	before := []herdr.Tab{{ID: "a"}, {ID: "b"}}
	after := []herdr.Tab{{ID: "a"}, {ID: "c"}, {ID: "b"}}
	if got := newTabs(before, after); len(got) != 1 || got[0] != "c" {
		t.Fatalf("new tabs %v, want [c]", got)
	}
}
