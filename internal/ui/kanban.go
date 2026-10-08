package ui

import (
	"strings"

	"github.com/ezemacchi/ekanban/internal/nav"
	"github.com/ezemacchi/ekanban/internal/screen"
)

// Kanban lays the same spaces out as columns, one per status. Because a column
// *is* a status, moving a card sideways is what retags it -- the horizontal
// equivalent of crossing a group boundary in the list.

func (m *Model) viewKanbanBoard() string {
	var b strings.Builder
	b.WriteString(m.viewHeader())
	b.WriteString("\n\n")

	cols := m.kanbanColumns()
	widths := screen.Widths(cols, m.width)
	from, end := screen.ScrollColumns(widths, m.colOffset, m.col, m.width)
	m.colOffset = from
	for col := from; col < end; col++ {
		if widths[col] == 0 || cols[col].Folded {
			continue
		}
		inner := widths[col] - screen.Gutter
		for i, sp := range m.columnSpaces(col) {
			cols[col].Cards = append(cols[col].Cards, m.card(sp, col == m.col && i == m.rowInCol, inner))
		}
	}
	drawn := screen.Draw(&m.zones, cols, widths, from, end, linesIn(b.String()), m.listHeight())
	if m.found() == 0 && m.filter != "" {
		// Under the headings, where the cards would be.
		rows := strings.Split(drawn, "\n")
		if len(rows) > 3 {
			rows[3] = m.noMatch(m.width)
		}
		drawn = strings.Join(rows, "\n")
	}
	b.WriteString(drawn)

	footer := m.viewFooter()
	m.placeFooter(linesIn(b.String()))
	b.WriteString(footer)
	return b.String()
}

// kanbanColumns are the statuses as columns, without their cards yet.
func (m *Model) kanbanColumns() []screen.Column {
	cols := make([]screen.Column, len(m.board.Statuses))
	for col, st := range m.board.Statuses {
		cols[col] = screen.Column{
			Label:    m.statusLabel(st),
			Color:    st.Color,
			Count:    m.columnCount(col),
			Hidden:   m.statusFilter != "" && st.ID != m.statusFilter,
			Folded:   m.board.IsCollapsed(st.ID),
			Selected: -1,
		}
		if col == m.col {
			cols[col].Selected = m.rowInCol
		}
	}
	return cols
}

// folded is whether a kanban column is folded: it shows its header alone.
func (m *Model) folded(col int) bool {
	return col >= 0 && col < len(m.board.Statuses) && m.board.IsCollapsed(m.board.Statuses[col].ID)
}

// navCount is the cards the cursor can reach in a column: none in a folded
// one.
func (m *Model) navCount(col int) int {
	if m.folded(col) {
		return 0
	}
	return m.columnCount(col)
}

// toggleFold folds or unfolds a kanban column, keeping the cursor on a card
// that can be seen.
func (m *Model) toggleFold(col int) {
	if col < 0 || col >= len(m.board.Statuses) {
		return
	}
	m.board.ToggleCollapsed(m.board.Statuses[col].ID)
	m.save()
	if m.folded(m.col) {
		m.col = nav.Settle(m.col, len(m.board.Statuses), m.navCount)
		m.rowInCol = 0
	}
	m.clampColumnCursor()
}
