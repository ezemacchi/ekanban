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
		where, tab := chooseOpening(c.tabs, c.pane)
		if where != c.where || tab != c.tab {
			t.Errorf("%s: got %v %q, want %v %q", c.name, where, tab, c.where, c.tab)
		}
	}
}

func TestNewTabs(t *testing.T) {
	before := []herdr.Tab{{ID: "a"}, {ID: "b"}}
	after := []herdr.Tab{{ID: "a"}, {ID: "c"}, {ID: "b"}}
	if got := newTabs(before, after); len(got) != 1 || got[0] != "c" {
		t.Fatalf("new tabs %v, want [c]", got)
	}
}
