package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/pipeline"
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
		store.Key("/tmp/api"): {Stage: pipeline.ReadyQA, Key: "ABC-1", Title: "Arreglar la grilla", PR: 5, MergedTo: "predev", Deployed: "predev"},
		store.Key("/tmp/web"): {},
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

	selectSpace(t, m, "/tmp/api")
	send(t, m, key("1"))
	if m.status != computedColumns {
		t.Fatalf("a number key must not retag in pipeline mode, status %q", m.status)
	}
	if got := labelsIn(m, pipeline.ReadyQA); !equal(got, []string{"api"}) {
		t.Fatalf("api left Ready for QA: %v", got)
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

// A column renamed in config.toml shows its new name, and the messages that
// name the ready_qa column follow it.
func TestConfiguredColumnNamesShow(t *testing.T) {
	m := newTestModel(t)
	cols, _ := columns.Merge(pipeline.DefaultColumns, []columns.Column{{ID: pipeline.ReadyQA, Label: "QA ready"}}, true)
	m.SetPipeline(pipeline.New("", pipeline.Settings{Columns: cols}, nil))
	send(t, m, liveWorkspaces())
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key("/tmp/api"): {Stage: pipeline.InProgress, Key: "ABC-1"},
	}})
	m.width = 200
	if out := m.View(); !strings.Contains(out, "QA ready") || strings.Contains(out, "To QA") {
		t.Fatalf("the renamed column is not shown:\n%s", out)
	}
	selectSpace(t, m, "/tmp/api")
	send(t, m, key("a"))
	if !strings.Contains(m.status, "QA ready") {
		t.Fatalf("status %q should name the configured column", m.status)
	}
}

// [keys] accept = "y": y accepts, a no longer does, and help shows y.
func TestReboundAcceptKey(t *testing.T) {
	m := pipelineBoard(t)
	m.SetKeys(map[string][]string{"accept": {"y"}})
	selectSpace(t, m, "/tmp/api")
	api := store.Key("/tmp/api")
	send(t, m, key("a"))
	if m.board.Entries[api].Accepted != nil {
		t.Fatal("a still accepts after accept moved to y")
	}
	send(t, m, key("y"))
	if m.board.Entries[api].Accepted == nil {
		t.Fatalf("y did not accept; status %q", m.status)
	}
	send(t, m, key("?"))
	if out := m.View(); !strings.Contains(out, "y") || !strings.Contains(out, "accept a ticket in") {
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
	widths := m.columnWidths()
	for col, st := range m.board.Statuses {
		header := lipgloss.Width(m.columnHeader(col)) + columnGutter
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
	m.board.SetNote(store.Key("/tmp/api"), "nota")
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
	if saved.Entries[store.Key("/tmp/api")].Note != "nota" {
		t.Fatal("note was not saved")
	}
}

func TestAcceptMovesTicketToArchive(t *testing.T) {
	m := pipelineBoard(t)
	selectSpace(t, m, "/tmp/api")
	send(t, m, key("a"))
	if got := labelsIn(m, pipeline.ReadyQA); len(got) != 0 {
		t.Fatalf("accepted ticket is still on the board: %v", got)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	rec := saved.Entries[store.Key("/tmp/api")].Accepted
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
		store.Key("/tmp/api"): {Stage: pipeline.OnReview, Key: "ABC-1", PR: 5},
	}})
	selectSpace(t, m, "/tmp/api")
	send(t, m, key("a"))
	if m.board.Entries[store.Key("/tmp/api")].Accepted != nil {
		t.Fatal("a ticket still in review was accepted")
	}
}
