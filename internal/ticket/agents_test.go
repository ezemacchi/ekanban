package ticket

import (
	"testing"

	"github.com/ezemacchi/ekanban/internal/herdr"
)

func TestChangesAnnounceMovesNotTheFirstReading(t *testing.T) {
	now := []Agent{{Who: "Implementer", Pane: "p1", Status: "working"}}
	seen, moved := Changes(nil, now)
	if len(moved) != 0 {
		t.Fatalf("first reading announced %v", moved)
	}

	now = []Agent{
		{Who: "Implementer", Pane: "p1", Status: "done", Title: "Tests pass"},
		{Who: "Reviewer", Pane: "p2", Status: "working"},
	}
	seen, moved = Changes(seen, now)
	if len(moved) != 2 || moved[0].Says() != "Implementer finished" || moved[1].Says() != "Reviewer started" {
		t.Fatalf("moves %+v", moved)
	}

	now[0].Status = "idle" // the user looked at the finished work
	_, moved = Changes(seen, now)
	if len(moved) != 0 {
		t.Fatalf("done to idle announced %v", moved)
	}
}

func TestLoudestAsksMostOfTheUser(t *testing.T) {
	agents := []Agent{
		{Who: "Orchestrator", Status: "idle"},
		{Who: "Implementer", Status: "working"},
		{Who: "Reviewer", Status: "done"},
	}
	if a, ok := Loudest(agents); !ok || a.Who != "Reviewer" {
		t.Fatalf("loudest %+v %v", a, ok)
	}
	if _, ok := Loudest(agents[:1]); ok {
		t.Fatal("an idle agent alone is not worth a line")
	}
}

func TestTitleThatOnlyNamesTheProgramIsDropped(t *testing.T) {
	kind := "cursor"
	if got := titleOf(herdr.Agent{Agent: &kind, Title: "Cursor Agent"}); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := titleOf(herdr.Agent{Agent: &kind, Title: "File Processor"}); got != "File Processor" {
		t.Fatalf("got %q", got)
	}
}
