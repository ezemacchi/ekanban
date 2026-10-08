package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/store"
)

// The help is a box over the board, grouped into sections, and on the
// computed board it leaves out what that board refuses.
func TestHelpIsABoxOverTheBoard(t *testing.T) {
	m := pipelineBoard(t)
	m.width, m.height = 140, 44
	send(t, m, key("?"))
	out := ansi.Strip(m.View())
	for _, want := range []string{"Help", "Move", "The selected card", "The board", "╭"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help box is missing %q:\n%s", want, out)
		}
	}
	for _, refused := range []string{"status picker", "manage statuses", "send to that status"} {
		if strings.Contains(out, refused) {
			t.Fatalf("the computed board's help offers %q:\n%s", refused, out)
		}
	}
	send(t, m, key("x"))
	if m.mode != modeNormal {
		t.Fatal("a key did not close the help")
	}
}

// A click on a table heading that sorts sorts by it.
func TestClickingATableHeadingSorts(t *testing.T) {
	m := threeInTodo(t)
	m.setLayout(layoutTable)
	clickZoneOf(t, m, isControl("sort:name"))
	if m.sort != sortName || m.board.TableSort != "name" {
		t.Fatalf("sort is %v after clicking the name heading", m.sort)
	}
	if !strings.Contains(ansi.Strip(m.View()), "↓SPACE") {
		t.Fatal("the heading does not mark the order")
	}
}

// On the computed board the table names tickets and says what they are
// about, not the folder they are in.
func TestComputedTableShowsTickets(t *testing.T) {
	m := pipelineBoard(t)
	m.width = 160
	m.setLayout(layoutTable)
	out := ansi.Strip(m.View())
	for _, want := range []string{"TICKET", "TITLE", "STATE", "ABC-1", "Arreglar la grilla"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table is missing %q:\n%s", want, out)
		}
	}
}

// A list row is a one-line card: its name, what it is about and the fact
// that says most.
func TestListRowIsAOneLineCard(t *testing.T) {
	m := pipelineBoard(t)
	m.width = 160
	m.setLayout(layoutList)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "Arreglar la grilla") || !strings.Contains(out, "PR #5") {
		t.Fatalf("the row does not say what the ticket is and how it is doing:\n%s", out)
	}
	if !strings.Contains(out, "▾ To QA 1") {
		t.Fatalf("the group's heading is not the kanban's:\n%s", out)
	}
}

// A chord is in the card's menu when it applies, and choosing it does what
// the chord does.
func TestTheMenuOffersAChordAndPressesIt(t *testing.T) {
	opened := captureOpens(t)
	m := newTestModel(t)
	send(t, m, liveWorkspaces())
	withPR(m, tmp+"api", gh.PR{Number: 42, State: "OPEN", URL: "https://git.example/pr/42"})
	selectSpace(t, m, tmp+"api")
	m.openCardMenu(-1, -1)
	if m.menu == nil {
		t.Fatal("no menu")
	}
	choice := -1
	for i, it := range m.menu.Items {
		if it.Key == "gp" {
			choice = i
		}
	}
	if choice < 0 {
		t.Fatalf("the menu does not offer gp: %+v", m.menu.Items)
	}
	_, cmd := m.chooseMenu(choice)
	if cmd == nil {
		t.Fatalf("choosing gp did nothing; status %q", m.status)
	}
	run(cmd)
	if len(*opened) != 1 {
		t.Fatalf("opened %v", *opened)
	}
}

// Every button the selected card shows presses a key the board answers.
func TestCardButtonsAreKeys(t *testing.T) {
	m := pipelineBoard(t)
	m.width = 200
	m.View()
	var keys []string
	for _, z := range m.zones {
		if z.Kind == screen.OnButton && z.Y < m.height-2 {
			keys = append(keys, z.Key)
		}
	}
	if len(keys) == 0 || keys[len(keys)-1] != m.hintKey("menu") {
		t.Fatalf("the selected card's buttons are %v; the last must open its menu", keys)
	}
}

// The detail closes from the ✕ at the right of its title.
func TestDetailClosesFromItsCross(t *testing.T) {
	m := pipelineBoard(t)
	m.width, m.height = 160, 40
	send(t, m, key("d"))
	out := strings.Split(m.View(), "\n")
	var x, y = -1, -1
	for row, l := range out {
		if i := strings.Index(ansi.Strip(l), "✕"); i >= 0 {
			x, y = len([]rune(ansi.Strip(l)[:i])), row
			break
		}
	}
	if x < 0 {
		t.Fatal("no ✕ on the detail")
	}
	send(t, m, click(x, y))
	if m.mode != modeNormal {
		t.Fatalf("a click on ✕ left mode %v", m.mode)
	}
}

// The note is a field in the detail: a click on it writes it right there,
// with the detail still open, and enter keeps it.
func TestTheNoteIsWrittenInTheDetail(t *testing.T) {
	m := pipelineBoard(t)
	m.width, m.height = 160, 40
	send(t, m, key("d"))
	m.View()
	var box linkRegion
	for _, r := range m.links {
		if r.key == m.hintKey("note") {
			box = r
		}
	}
	if box.key == "" {
		t.Fatal("the note has no field to click")
	}
	send(t, m, click(box.x0+2, box.row))
	if m.mode != modeNote || m.prevMode != modeDetail {
		t.Fatalf("a click on the note left mode %v, from %v", m.mode, m.prevMode)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "NOTE") || !strings.Contains(out, "save") {
		t.Fatalf("the detail is not open while the note is written:\n%s", out)
	}
	for _, r := range "waiting on QA" {
		send(t, m, key(string(r)))
	}
	send(t, m, key("enter"))
	if m.mode != modeDetail || m.selected().Note != "waiting on QA" {
		t.Fatalf("mode %v, note %q", m.mode, m.selected().Note)
	}
}

// The handoff's choices and its ✕ take clicks, so it can be driven with the
// mouse as well as the keys.
func TestTheHandoffTakesClicks(t *testing.T) {
	m := jiraBoard(t)
	m.width, m.height = 120, 40
	selectJiraCard(t, m, "ABC-3")
	send(t, m, key("H"))
	clickZoneOf(t, m, isChoice(1)) // master
	if m.jr.handoff.target != "master" || m.jr.handoff.step != 1 {
		t.Fatalf("a click on a target did not pick it: %+v", m.jr.handoff)
	}
	clickZoneOf(t, m, func(z screen.Zone) bool { return z.Kind == screen.OnButton && z.Key == "esc" && z.X1-z.X0 == 3 })
	if m.mode == modeHandoff {
		t.Fatal("the ✕ did not cancel the handoff")
	}
}

// A search that finds nothing says so, and how to lift it, in every view.
func TestASearchThatFindsNothingSaysSo(t *testing.T) {
	for _, l := range []layout{layoutKanban, layoutList, layoutTable} {
		m := threeInTodo(t)
		m.width = 120
		m.setLayout(l)
		m.filter = "zzz"
		m.rebuild()
		if out := ansi.Strip(m.View()); !strings.Contains(out, "nothing matches “zzz”") || !strings.Contains(out, "0 found") {
			t.Fatalf("%v view:\n%s", l, out)
		}
	}
}

// Esc backs out of a search, then a filter, and then does nothing: only the
// quit key closes the board.
func TestEscNeverClosesTheBoard(t *testing.T) {
	m := threeInTodo(t)
	m.filter = "alpha"
	m.rebuild()
	send(t, m, key("esc"))
	if m.filter != "" || m.quitting {
		t.Fatal("esc did not clear the search first")
	}
	send(t, m, key("esc"))
	if m.quitting {
		t.Fatal("esc closed the board")
	}
	if got := screen.Plain(m.status); !strings.Contains(got, "press q to close the board") {
		t.Fatalf("status %q", got)
	}
	send(t, m, key("q"))
	if !m.quitting {
		t.Fatal("q did not close the board")
	}
}

// Forgetting asks (Y)es/(N)o: n, N and esc keep the space, Y forgets it, any
// other key leaves the question open, and the answers take clicks.
func TestForgetAsksYesOrNo(t *testing.T) {
	m := newTestModel(t)
	send(t, m, liveWorkspaces())
	selectSpace(t, m, tmp+"api")
	send(t, m, key("3"))
	api := store.Key(tmp + "api")
	for _, no := range []string{"n", "N", "esc"} {
		send(t, m, key("x"))
		if out := ansi.Strip(m.View()); !strings.Contains(out, "(Y)es") || !strings.Contains(out, "(N)o") {
			t.Fatalf("no (Y)es/(N)o question:\n%s", out)
		}
		send(t, m, key("j")) // not an answer
		if m.mode != modeConfirm {
			t.Fatal("a key that is not an answer closed the question")
		}
		send(t, m, key(no))
		if _, ok := m.board.Entries[api]; !ok || m.mode != modeNormal {
			t.Fatalf("%s forgot the space, or left mode %v", no, m.mode)
		}
	}
	send(t, m, key("x"))
	clickZoneOf(t, m, func(z screen.Zone) bool { return z.Kind == screen.OnButton && z.Key == "y" })
	if _, ok := m.board.Entries[api]; ok {
		t.Fatal("a click on (Y)es did not forget it")
	}
	send(t, m, key("3"))
	send(t, m, key("x"))
	send(t, m, key("Y"))
	if _, ok := m.board.Entries[api]; ok {
		t.Fatal("Y did not forget it")
	}
}

// The broadcast lists the orchestrators, all ticked; a click unticks one, a
// ready-made message fills the message, and enter asks (Y)es/(N)o before
// anything is sent, with {target} as each ticket gets it.
func TestTheBroadcastAsksBeforeItSends(t *testing.T) {
	m := pipelineBoard(t)
	m.width, m.height = 140, 40
	m.SetBroadcast([]string{"Rebase onto origin/{target} and push."})
	send(t, m, key("B"))
	if m.mode != modeBroadcast {
		t.Fatalf("B left mode %v", m.mode)
	}
	send(t, m, recipientsMsg{to: []recipient{
		{key: "ABC-1", pane: "p1", status: "idle", target: "main", on: true},
		{key: "ABC-2", pane: "p2", status: "working", target: "next", on: true},
	}})
	out := ansi.Strip(m.View())
	for _, want := range []string{"Broadcast to the orchestrators", "[x] ABC-1", "[x] ABC-2", "Send to 2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("box is missing %q:\n%s", want, out)
		}
	}
	clickZoneOf(t, m, isChoice(1))
	if m.bc.to[1].on {
		t.Fatal("a click did not untick ABC-2")
	}
	clickZoneOf(t, m, isControl("preset:0"))
	if m.input.Value() != "Rebase onto origin/{target} and push." {
		t.Fatalf("the preset did not fill the message: %q", m.input.Value())
	}
	send(t, m, key("enter"))
	if m.mode != modeConfirm {
		t.Fatalf("enter sent without asking; mode %v", m.mode)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Send to 1 orchestrator?") || !strings.Contains(out, "Rebase onto origin/main and push.") {
		t.Fatalf("the question does not say what goes where:\n%s", out)
	}
	send(t, m, key("n"))
	if m.mode != modeBroadcast || m.input.Value() == "" {
		t.Fatal("no did not go back to the box with the message")
	}
	send(t, m, key("enter"))
	_, cmd := m.Update(key("y"))
	if m.mode != modeNormal || cmd == nil {
		t.Fatalf("yes did not send; mode %v", m.mode)
	}
}

// A quick choice ticks exactly the orchestrators it names -- the ones with a
// pull request open, say -- and only choices that would tick someone show.
func TestBroadcastQuickChoices(t *testing.T) {
	m := pipelineBoard(t)
	m.width, m.height = 140, 40
	send(t, m, key("B"))
	send(t, m, recipientsMsg{to: []recipient{
		{key: "ABC-1", status: "idle", stage: pipeline.ReadyQA, openPR: true, on: true},
		{key: "ABC-2", status: "working", stage: pipeline.InProgress, on: true},
		{key: "ABC-3", status: "idle", stage: pipeline.InProgress, openPR: true, on: true},
	}})
	out := ansi.Strip(m.View())
	for _, want := range []string{"a All 3", "n None", "p Open PR 2", "i Idle 2", "w Working 1", "1 Working 2", "2 To QA 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing the quick choice %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Needs you") {
		t.Fatal("a quick choice that ticks nobody is shown")
	}
	pr := -1
	for i, p := range m.picks() {
		if p.label == "Open PR" {
			pr = i
		}
	}
	clickZoneOf(t, m, isControl("pick:"+strconv.Itoa(pr)))
	var on []string
	for _, r := range m.chosen() {
		on = append(on, r.key)
	}
	if strings.Join(on, ",") != "ABC-1,ABC-3" {
		t.Fatalf("Open PR ticked %v", on)
	}
	if !m.inForce(m.picks()[pr]) {
		t.Fatal("the choice in force is not marked")
	}

	// From the keyboard: in the list, a badge's key picks it.
	send(t, m, key("tab"))
	send(t, m, key("w"))
	if got := m.chosen(); len(got) != 1 || got[0].key != "ABC-2" {
		t.Fatalf("w ticked %v", got)
	}
	send(t, m, key("a"))
	if len(m.chosen()) != 3 {
		t.Fatal("a did not tick everyone")
	}
	send(t, m, key("n"))
	if len(m.chosen()) != 0 {
		t.Fatal("n did not tick nobody")
	}
	send(t, m, key("2"))
	if got := m.chosen(); len(got) != 1 || got[0].key != "ABC-1" {
		t.Fatalf("2 (the second column shown, To QA) ticked %v", got)
	}

	// Sideways goes through the quick choices from the one in force.
	send(t, m, key("a")) // All, the first
	send(t, m, key("l"))
	if len(m.chosen()) != 0 || !m.inForce(m.picks()[1]) {
		t.Fatal("l after All did not move to None")
	}
	send(t, m, key("h"))
	send(t, m, key("h"))
	if !m.inForce(m.picks()[len(m.picks())-1]) {
		t.Fatal("h from All did not go round to the last choice")
	}
}

// In the message, the arrows go through the ready-made messages.
func TestArrowsGoThroughTheReadyMadeMessages(t *testing.T) {
	m := pipelineBoard(t)
	m.SetBroadcast([]string{"first", "second"})
	send(t, m, key("B"))
	send(t, m, key("down"))
	if m.input.Value() != "first" {
		t.Fatalf("down: %q", m.input.Value())
	}
	send(t, m, key("down"))
	if m.input.Value() != "second" {
		t.Fatalf("down again: %q", m.input.Value())
	}
	send(t, m, key("up"))
	if m.input.Value() != "first" {
		t.Fatalf("up: %q", m.input.Value())
	}
}
