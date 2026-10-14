package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/store"
)

// pipelineBoard is a board in pipeline mode with api classified as a ticket
// in Ready for QA and web as something that is not a ticket.
func pipelineBoard(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t)
	m.SetPipeline(pipeline.New("", pipeline.Settings{}, nil))
	send(t, m, liveWorkspaces())
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key(tmp + "api"): {Stage: pipeline.ReadyQA, Key: "ABC-1", Title: "Arreglar la grilla", PR: 5, MergedTo: "predev", Deployed: "predev"},
		store.Key(tmp + "web"): {},
	}})
	return m
}

func TestPipelineColumnsAreComputed(t *testing.T) {
	m := pipelineBoard(t)
	if m.layout != layoutKanban {
		t.Fatal("pipeline mode should show the kanban")
	}
	if got := labelsIn(m, pipeline.ReadyQA); !equal(got, []string{"api"}) {
		t.Fatalf("Ready for QA holds %v, want [api]", got)
	}
	for _, st := range m.board.Statuses {
		for _, sp := range m.groups[st.ID] {
			if sp.Label == "web" {
				t.Fatalf("a space that is not a ticket is on the board, in %s", st.ID)
			}
		}
	}

	selectSpace(t, m, tmp+"api")
	send(t, m, key("S"))
	if m.status != m.computedColumns() {
		t.Fatalf("editing the statuses must stay refused in pipeline mode, status %q", m.status)
	}

	m.width = 200
	out := m.View()
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╰") {
		t.Fatalf("cards are not boxed:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("a line is %d cells wide on a %d-cell board: %q", w, m.width, line)
		}
	}
	for _, want := range []string{"PR #5", "shipped to predev", "To QA"} {
		if !strings.Contains(out, want) {
			t.Fatalf("board is missing %q:\n%s", want, out)
		}
	}
}

// A card moved by hand sits in the chosen column, marked so, and leaves the
// plain board's status alone. A different computed column read once is held
// back; read twice in a row, the card follows the board again.
func TestPlacedByHandFollowsTheBoardWhenItChanges(t *testing.T) {
	m := pipelineBoard(t)
	api := store.Key(tmp + "api")
	before := m.board.Entries[api].Status
	selectSpace(t, m, tmp+"api")
	send(t, m, key("3")) // Reviewing
	if got := labelsIn(m, pipeline.OnReview); !equal(got, []string{"api"}) {
		t.Fatalf("Reviewing holds %v, want [api]", got)
	}
	if m.board.Entries[api].Placed == nil {
		t.Fatal("3 did not place the card by hand")
	}
	if m.board.Entries[api].Status != before {
		t.Fatalf("the plain board's status changed to %q", m.board.Entries[api].Status)
	}
	m.width = 200
	if out := m.View(); !strings.Contains(out, "board: To QA") {
		t.Fatalf("the card does not say where the board puts it:\n%s", out)
	}
	if text, _ := m.placedText(api); text != "moved by hand; the board says To QA" {
		t.Fatalf("copied mark %q", text)
	}

	// The board still says To QA: the placement holds over a reclassification.
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: m.pipeInfo[api]}})
	if got := labelsIn(m, pipeline.OnReview); !equal(got, []string{"api"}) {
		t.Fatalf("a reclassification to the same column dropped the placement: %v", got)
	}

	// One read says Shipping: it may be a read that failed halfway.
	info := m.pipeInfo[api]
	info.Stage = pipeline.ToDeploy
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: info}})
	if got := labelsIn(m, pipeline.OnReview); !equal(got, []string{"api"}) {
		t.Fatalf("one different read dropped the placement: %v", got)
	}
	// The next says Shipping too: the board has learnt something.
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: info}})
	if got := labelsIn(m, pipeline.ToDeploy); !equal(got, []string{"api"}) {
		t.Fatalf("Shipping holds %v, want [api]", got)
	}
	if m.board.Entries[api].Placed != nil {
		t.Fatal("the placement outlived a change of the computed column")
	}
}

// A different read that is not repeated does not count: back to the column
// it was moved away from, the placement holds, and a later single read is
// held back again.
func TestOneOffReadDoesNotDropThePlacement(t *testing.T) {
	m := pipelineBoard(t)
	api := store.Key(tmp + "api")
	selectSpace(t, m, tmp+"api")
	send(t, m, key("3"))
	qa, todo := m.pipeInfo[api], m.pipeInfo[api]
	todo.Stage = pipeline.ToDo
	for _, info := range []pipeline.Info{todo, qa, todo} {
		send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: info}})
	}
	if m.board.Entries[api].Placed == nil {
		t.Fatal("reads that disagreed once each dropped the placement")
	}
}

// When the board computes the column the card was moved to, it agrees: the
// mark goes at once.
func TestBoardCatchingUpDropsTheMarkAtOnce(t *testing.T) {
	m := pipelineBoard(t)
	api := store.Key(tmp + "api")
	selectSpace(t, m, tmp+"api")
	send(t, m, key("3")) // Reviewing, over To QA
	info := m.pipeInfo[api]
	info.Stage = pipeline.OnReview
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: info}})
	if m.board.Entries[api].Placed != nil {
		t.Fatal("the board agrees, yet the card is still marked as moved by hand")
	}
	if got := labelsIn(m, pipeline.OnReview); !equal(got, []string{"api"}) {
		t.Fatalf("Reviewing holds %v, want [api]", got)
	}
}

// The picker moves a card by hand and marks the board's own column; the card
// then offers Undo, which puts it back.
func TestPickerMovesByHandAndFollowBoardPutsItBack(t *testing.T) {
	m := pipelineBoard(t)
	api := store.Key(tmp + "api")
	selectSpace(t, m, tmp+"api")
	send(t, m, key("s"))
	if m.mode != modeStatusPick {
		t.Fatal("s did not open the picker on the computed board")
	}
	lines := strings.Join(m.pickerLines(60), "\n")
	if !strings.Contains(lines, "move to:") || !strings.Contains(lines, "To QA · board") {
		t.Fatalf("the picker does not mark the board's column:\n%s", lines)
	}
	send(t, m, key("2")) // Working
	if got := labelsIn(m, pipeline.InProgress); !equal(got, []string{"api"}) {
		t.Fatalf("Working holds %v, want [api]", got)
	}
	if !strings.Contains(m.status, "press "+screen.Key("b")) {
		t.Fatalf("the move does not say how to put it back: %q", m.status)
	}
	sp := m.selected()
	if c := m.card(sp, false, 40); !strings.Contains(screen.Plain(c.Badge), handBadge(false)) {
		t.Fatalf("badge %q", c.Badge)
	}
	var undo bool
	for _, a := range m.cardActions(sp) {
		undo = undo || a.action == "follow-board" && a.onCard
	}
	if !undo {
		t.Fatal("a card moved by hand has no Undo button")
	}

	send(t, m, key("b"))
	if m.board.Entries[api].Placed != nil {
		t.Fatal("undo kept the placement")
	}
	if got := labelsIn(m, pipeline.ReadyQA); !equal(got, []string{"api"}) {
		t.Fatalf("To QA holds %v, want [api]", got)
	}
}

// With manual_moves off the computed board refuses every move by hand.
func TestManualMovesOffRefusesMovingByHand(t *testing.T) {
	m := newTestModel(t)
	m.SetPipeline(pipeline.New("", pipeline.Settings{NoHandMoves: true}, nil))
	send(t, m, liveWorkspaces())
	api := store.Key(tmp + "api")
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{api: {Stage: pipeline.ReadyQA, Key: "ABC-1"}}})
	selectSpace(t, m, tmp+"api")
	for _, k := range []string{"1", "s"} {
		send(t, m, key(k))
		if m.status != m.computedColumns() || m.mode == modeStatusPick {
			t.Fatalf("%s moved or opened the picker with manual_moves off: %q", k, m.status)
		}
	}
	if m.board.Entries[api].Placed != nil {
		t.Fatal("a placement was made with manual_moves off")
	}
	for _, a := range m.cardActions(m.selected()) {
		if a.action == "status-picker" {
			t.Fatal("the card offers Move with manual_moves off")
		}
	}
}

// Moving a card back to the column the board says drops the placement.
func TestPlacingBackOnTheComputedColumnDropsIt(t *testing.T) {
	m := pipelineBoard(t)
	api := store.Key(tmp + "api")
	selectSpace(t, m, tmp+"api")
	send(t, m, key("1"))
	if m.board.Entries[api].Placed == nil {
		t.Fatal("1 did not place the card by hand")
	}
	selectSpace(t, m, tmp+"api")
	send(t, m, key("5")) // To QA, where the board puts it
	if m.board.Entries[api].Placed != nil {
		t.Fatal("moving back to the computed column kept the placement")
	}
	if got := labelsIn(m, pipeline.ReadyQA); !equal(got, []string{"api"}) {
		t.Fatalf("To QA holds %v, want [api]", got)
	}
}

// A ticket whose workspace was closed stays on the board, dimmed and marked,
// while a closed checkout that holds no ticket does not appear.
func TestClosedWorktreeStaysOnTheBoard(t *testing.T) {
	m := newTestModel(t)
	m.SetPipeline(pipeline.New("", pipeline.Settings{}, nil))
	ticketDir, plainDir := t.TempDir(), t.TempDir()
	m.branches = map[string]string{store.Key(ticketDir): "feat/ABC-2-thing", store.Key(plainDir): "main"}
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key(ticketDir): {Stage: pipeline.OnReview, Key: "ABC-2", PR: 7},
		store.Key(plainDir):  {},
	}})
	got := m.groups[pipeline.OnReview]
	if len(got) != 1 || got[0].Key != store.Key(ticketDir) || got[0].Live {
		t.Fatalf("Reviewing holds %+v, want the closed ticket worktree", got)
	}
	if got[0].Label != filepath.Base(ticketDir) {
		t.Fatalf("label %q, want the folder name %q", got[0].Label, filepath.Base(ticketDir))
	}
	for _, st := range m.board.Statuses {
		for _, sp := range m.groups[st.ID] {
			if sp.Key == store.Key(plainDir) {
				t.Fatalf("a closed checkout with no ticket is on the board, in %s", st.ID)
			}
		}
	}
	m.width = 200
	if out := m.View(); !strings.Contains(out, "workspace closed") || !strings.Contains(out, "ABC-2") {
		t.Fatalf("the closed worktree is not marked, or not named by its ticket:\n%s", out)
	}
}

// Worktrees of one ticket are told apart by what their folder adds after the
// key; a ticket with one worktree is just its key.
func TestTicketCardsAreNamedByKey(t *testing.T) {
	m := newTestModel(t)
	m.SetPipeline(pipeline.New("", pipeline.Settings{}, nil))
	m.pipeInfo = map[string]pipeline.Info{
		`C:\repos\App-wt-ABC-1`:        {Key: "ABC-1"},
		`C:\repos\App-wt-ABC-1-dialog`: {Key: "ABC-1"},
		`C:\repos\App-wt-ABC-2`:        {Key: "ABC-2"},
		`C:\repos\other-place`:         {Key: "ABC-2"},
	}
	cases := map[string]string{
		`C:\repos\App-wt-ABC-1`:        "ABC-1",
		`C:\repos\App-wt-ABC-1-dialog`: "ABC-1 dialog",
		`C:\repos\other-place`:         "ABC-2 other-place",
	}
	for space, want := range cases {
		if got := m.ticketName(space, m.pipeInfo[space].Key); got != want {
			t.Errorf("%s: got %q, want %q", space, got, want)
		}
	}
}

// A column renamed in config.toml shows its new name, and the messages that
// name the ready_qa column follow it.
func TestConfiguredColumnNamesShow(t *testing.T) {
	m := newTestModel(t)
	cols, _ := columns.Merge(pipeline.DefaultColumns, []columns.Column{{ID: pipeline.ReadyQA, Label: "QA ready"}}, true)
	m.SetPipeline(pipeline.New("", pipeline.Settings{Columns: cols}, nil))
	send(t, m, liveWorkspaces())
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key(tmp + "api"): {Stage: pipeline.InProgress, Key: "ABC-1"},
	}})
	m.width = 200
	if out := m.View(); !strings.Contains(out, "QA ready") || strings.Contains(out, "To QA") {
		t.Fatalf("the renamed column is not shown:\n%s", out)
	}
	selectSpace(t, m, tmp+"api")
	send(t, m, key("a"))
	if !strings.Contains(m.status, "QA ready") {
		t.Fatalf("status %q should name the configured column", m.status)
	}
}

// [keys] accept = "Z": Z accepts, a no longer does, and help shows Z.
func TestReboundAcceptKey(t *testing.T) {
	m := pipelineBoard(t)
	m.SetKeys(map[string][]string{"accept": {"Z"}})
	selectSpace(t, m, tmp+"api")
	api := store.Key(tmp + "api")
	send(t, m, key("a"))
	if m.board.Entries[api].Accepted != nil {
		t.Fatal("a still accepts after accept moved to Z")
	}
	send(t, m, key("Z"))
	if m.board.Entries[api].Accepted == nil {
		t.Fatalf("Z did not accept; status %q", m.status)
	}
	send(t, m, key("?"))
	if out := m.View(); !strings.Contains(out, "Z") || !strings.Contains(out, "accept a ticket in") {
		t.Fatalf("help does not show the bound key:\n%s", out)
	}
}

// With To Do empty the cursor starts on the board's first card, not on an
// empty column where it cannot be seen.
func TestCursorStartsOnTheFirstCard(t *testing.T) {
	m := pipelineBoard(t)
	if st := m.board.Statuses[m.col]; st.ID != pipeline.ReadyQA || m.rowInCol != 0 {
		t.Fatalf("cursor is on %s row %d, want the first card in Ready for QA", st.ID, m.rowInCol)
	}
}

// Empty columns shrink to their header; the column with cards takes the room.
func TestEmptyColumnsTakeOnlyTheirHeader(t *testing.T) {
	m := pipelineBoard(t)
	m.width = 160
	cols := m.kanbanColumns()
	widths := screen.Widths(cols, m.width)
	for col, st := range m.board.Statuses {
		header := lipgloss.Width(cols[col].Header()) + screen.Gutter
		switch {
		case st.ID == pipeline.ReadyQA:
			if widths[col] <= header {
				t.Fatalf("the column with a card is only %d wide", widths[col])
			}
		case widths[col] != header:
			t.Fatalf("empty %s is %d wide, want its header's %d", st.Label, widths[col], header)
		}
	}
	out := m.View()
	for _, st := range m.board.Statuses {
		if !strings.Contains(out, st.Label) {
			t.Fatalf("header %q is cut:\n%s", st.Label, out)
		}
	}
}

// The computed columns must never be written over the user's own statuses.
func TestPipelineSaveKeepsManualStatuses(t *testing.T) {
	m := pipelineBoard(t)
	m.board.SetNote(store.Key(tmp+"api"), "nota")
	m.save()
	if m.board.Statuses[0].ID != pipeline.ToDo {
		t.Fatal("save left the manual statuses on screen")
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Statuses[0].ID != "todo" || len(saved.Statuses) != 4 {
		t.Fatalf("saved statuses were replaced: %+v", saved.Statuses)
	}
	if saved.Entries[store.Key(tmp+"api")].Note != "nota" {
		t.Fatal("note was not saved")
	}
}

func TestAcceptMovesTicketToArchive(t *testing.T) {
	m := pipelineBoard(t)
	selectSpace(t, m, tmp+"api")
	send(t, m, key("a"))
	if got := labelsIn(m, pipeline.ReadyQA); len(got) != 0 {
		t.Fatalf("accepted ticket is still on the board: %v", got)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	rec := saved.Entries[store.Key(tmp+"api")].Accepted
	if rec == nil || rec.Ticket != "ABC-1" || rec.PR != 5 || rec.Title != "Arreglar la grilla" {
		t.Fatalf("archive record not saved: %+v", rec)
	}

	send(t, m, key("A"))
	out := m.View()
	for _, want := range []string{"Archive", "ABC-1", "Arreglar la grilla", "PR #5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("archive is missing %q:\n%s", want, out)
		}
	}

	send(t, m, key("u"))
	send(t, m, key("esc"))
	if m.archiveView {
		t.Fatal("esc did not leave the archive")
	}
	if got := labelsIn(m, pipeline.ReadyQA); !equal(got, []string{"api"}) {
		t.Fatalf("u did not return the ticket to the board: %v", got)
	}
}

func TestAcceptOnlyFromReadyForQA(t *testing.T) {
	m := pipelineBoard(t)
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key(tmp + "api"): {Stage: pipeline.OnReview, Key: "ABC-1", PR: 5},
	}})
	selectSpace(t, m, tmp+"api")
	send(t, m, key("a"))
	if m.board.Entries[store.Key(tmp+"api")].Accepted != nil {
		t.Fatal("a ticket still in review was accepted")
	}
}
