package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/jira"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/stack"
)

// stackConfig turns the failed-tests source on, reading every linked test.
func stackConfig() stack.Config {
	return stack.Config{
		Sources: []string{"failed-tests"},
		Source:  map[string]map[string]string{"failed-tests": {"from": "linked"}},
	}
}

// stackBoard is the Jira board with stacked cards on and two tests linked to
// ABC-2 (the ticket in To Do). The keys are made up.
func stackBoard(t *testing.T) *Model {
	t.Helper()
	m := jiraBoard(t)
	m.width, m.height = 200, 40
	if problems := m.SetStack(stackConfig()); len(problems) > 0 {
		t.Fatalf("problems: %v", problems)
	}
	m.jr.detail["ABC-2"] = jira.Detail{
		Issue: jira.Issue{Key: "ABC-2"},
		Tests: []jira.Link{
			{Key: "ABC-11", Summary: "Grid total adds up", Status: "Defined", Type: "Test"},
			{Key: "ABC-12", Summary: "Dialog opens", Status: "Defined", Type: "Test"},
		},
	}
	m.rebuild()
	selectJiraCard(t, m, "ABC-2")
	return m
}

func stackAt(m *Model) int {
	sp := m.selected()
	items, _ := m.stackOf(sp)
	return m.stk.At(sp.Key, len(items))
}

// A board whose [stack] names no source is the board it was.
func TestStackOffLeavesTheBoardAlone(t *testing.T) {
	m := jiraBoard(t)
	m.width, m.height = 200, 40
	before := m.View()
	if problems := m.SetStack(stack.Config{}); len(problems) > 0 {
		t.Fatalf("problems: %v", problems)
	}
	if m.stk.On() {
		t.Fatal("stacked cards are on with no source")
	}
	if after := m.View(); after != before {
		t.Fatalf("the board changed with stacks off:\n%s\n---\n%s", before, after)
	}
	for _, a := range m.keys.pipeline.Actions() {
		if a.Scope == scopeStack {
			t.Fatalf("%s is in the key map with stacks off", a.Name)
		}
	}
}

// j and k, and the arrows, walk the strips; at the end the cursor stays on
// the last strip when there is no card beyond.
func TestStackKeysWalkTheStripsBothWays(t *testing.T) {
	for _, keys := range [][2]string{{"j", "k"}, {"down", "up"}} {
		m := stackBoard(t)
		if got := stackAt(m); got != -1 {
			t.Fatalf("starts on %d, want the card", got)
		}
		send(t, m, key(keys[0]))
		if got := stackAt(m); got != 0 {
			t.Fatalf("%s: first press is on %d, want 0", keys[0], got)
		}
		send(t, m, key(keys[0]))
		if got := stackAt(m); got != 1 {
			t.Fatalf("%s: second press is on %d, want 1", keys[0], got)
		}
		send(t, m, key(keys[0]))
		if got := stackAt(m); got != 1 || m.ticketOf(m.selected()) != "ABC-2" {
			t.Fatalf("%s: past the last strip with no card after: on %d of %s", keys[0], got, m.ticketOf(m.selected()))
		}
		send(t, m, key(keys[1]))
		send(t, m, key(keys[1]))
		if got := stackAt(m); got != -1 {
			t.Fatalf("%s twice back is on %d, want the card", keys[1], got)
		}
	}
}

// Past the last strip the cursor goes on to the next card; going back up from
// that card lands on the strip it left from.
func TestStackWalksOnToTheNextCardAndBack(t *testing.T) {
	m := stackBoard(t)
	// A second card in the same column, below the stacked one.
	m.jr.cfg.Columns = map[string]string{"In Implementation": pipeline.ToDo}
	m.rebuild()
	selectJiraCard(t, m, "ABC-2")
	if got := cardsIn(m, pipeline.ToDo); len(got) != 2 {
		t.Fatalf("To Do holds %v, want two cards", got)
	}
	for range 3 {
		send(t, m, key("j"))
	}
	if got := m.ticketOf(m.selected()); got == "ABC-2" {
		t.Fatalf("still on ABC-2 after walking past its strips")
	}
	send(t, m, key("k"))
	if got := m.ticketOf(m.selected()); got != "ABC-2" || stackAt(m) != 1 {
		t.Fatalf("back up is on %s strip %d, want ABC-2 on its last strip", got, stackAt(m))
	}
}

func TestStackShowsStripsAndCues(t *testing.T) {
	m := stackBoard(t)
	out := ansi.Strip(m.View())
	for _, want := range []string{"ABC-11", "ABC-12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("board is missing the strip %s:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "↓ ABC-11") {
		t.Fatalf("the cue does not say where down leads:\n%s", out)
	}
	send(t, m, key("j"))
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "↑ ABC-2") || !strings.Contains(out, "↓ ABC-12") {
		t.Fatalf("on the first strip, the cue is missing a direction:\n%s", out)
	}
	send(t, m, key("j"))
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "↑ ABC-11") || !strings.Contains(out, "↓ next card") {
		t.Fatalf("on the last strip, the cue is missing a direction:\n%s", out)
	}
}

// The detail follows the same cursor, and says where the keys lead both ways.
func TestStackDetailFollowsTheCursor(t *testing.T) {
	m := stackBoard(t)
	send(t, m, key("d"))
	if m.mode != modeDetail {
		t.Fatalf("d left mode %d", m.mode)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Stack 1 of 3") {
		t.Fatalf("detail does not say where the cursor is:\n%s", out)
	}
	send(t, m, key("j"))
	out := ansi.Strip(m.View())
	if m.mode != modeDetail || !strings.Contains(out, "Stack 2 of 3") || !strings.Contains(out, "Grid total adds up") {
		t.Fatalf("detail did not move to the first strip:\n%s", out)
	}
	send(t, m, key("k"))
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Stack 1 of 3") {
		t.Fatalf("detail did not come back to the card:\n%s", out)
	}
}

// Mouse users: a click on a strip selects it, and Prev and Next move.
func TestStackClicksSelectAStripAndMove(t *testing.T) {
	m := stackBoard(t)
	clickZoneOf(t, m, func(z screen.Zone) bool { return z.Kind == screen.OnStack && z.Choice == 1 })
	if got := stackAt(m); got != 1 {
		t.Fatalf("click on the second strip left the cursor on %d", got)
	}
	clickZoneOf(t, m, func(z screen.Zone) bool { return z.Kind == zoneButton && z.Key == m.hintKey("stack-up") })
	if got := stackAt(m); got != 0 {
		t.Fatalf("Prev left the cursor on %d, want 0", got)
	}
	clickZoneOf(t, m, func(z screen.Zone) bool { return z.Kind == zoneButton && z.Key == m.hintKey("stack-down") })
	if got := stackAt(m); got != 1 {
		t.Fatalf("Next left the cursor on %d, want 1", got)
	}
}

// A key rebound in [keys] moves the stack, and the old one no longer does.
func TestStackKeysCanBeRebound(t *testing.T) {
	m := stackBoard(t)
	m.SetKeys(map[string][]string{"stack-down": {"n"}})
	send(t, m, key("n"))
	if got := stackAt(m); got != 0 {
		t.Fatalf("n left the cursor on %d, want 0", got)
	}
	if got := m.hintKey("stack-down"); got != "n" {
		t.Fatalf("the Next button names %q, want n", got)
	}
}

// On a card with no stack, j and k move between cards as they always did.
func TestStackKeysAreOnlyForCardsWithAStack(t *testing.T) {
	m := stackBoard(t)
	selectJiraCard(t, m, "ABC-3")
	if got := m.keyScopes(); len(got) != 0 {
		t.Fatalf("a card with no stack has scopes %v", got)
	}
}

// With [stack] column set, a card with a stack is brought back to that column.
func TestStackColumnBringsACardBack(t *testing.T) {
	m := jiraBoard(t)
	cfg := stackConfig()
	cfg.Column = pipeline.ToDo
	m.SetStack(cfg)
	m.jr.detail["ABC-3"] = jira.Detail{
		Issue: jira.Issue{Key: "ABC-3"},
		Tests: []jira.Link{{Key: "ABC-13", Summary: "Total", Status: "Defined", Type: "Test"}},
	}
	m.rebuild()
	got := cardsIn(m, pipeline.ToDo)
	found := false
	for _, k := range got {
		found = found || k == "ABC-3"
	}
	if !found {
		t.Fatalf("ABC-3 has a stack but To Do holds %v", got)
	}
}

// H on a card with a worktree and a stack to fix hands off the tests, with
// rework_command; without it the card says what to set.
func TestStackHandoffReworksTheRelatedTests(t *testing.T) {
	m := jiraBoard(t)
	m.width, m.height = 200, 40
	m.SetStack(stackConfig())
	m.jr.detail["ABC-1"] = jira.Detail{
		Issue: jira.Issue{Key: "ABC-1"},
		Tests: []jira.Link{
			{Key: "ABC-11", Summary: "Grid total adds up", Status: "Defined", Type: "Test"},
			{Key: "ABC-12", Summary: "Dialog opens", Status: "Defined", Type: "Test"},
		},
	}
	m.rebuild()
	selectSpace(t, m, tmp+"api")

	send(t, m, key("H"))
	if m.mode == modeHandoff {
		t.Fatal("H handed off with no rework_command")
	}
	if !strings.Contains(m.status, "rework_command") {
		t.Fatalf("status does not say what to set: %q", m.status)
	}

	m.jr.cfg.Handoff.ReworkCommand = []string{"rework", "-Key", "{key}", "-Tests", "{tests}", "-QaComment", "{evidence}"}
	send(t, m, key("H"))
	if m.mode != modeHandoff || !m.jr.handoff.rework {
		t.Fatalf("H did not start a rework handoff: mode %d, %+v", m.mode, m.jr.handoff)
	}
	args := m.jr.cfg.Handoff.ReworkArgs(m.handoffValues())
	if args[0] != "rework" || args[2] != "ABC-1" || args[4] != "ABC-11,ABC-12" || !strings.Contains(args[6], "Dialog opens") {
		t.Fatalf("rework arguments are %q", args)
	}
}
