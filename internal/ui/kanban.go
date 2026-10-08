package ui

import (
	"strings"

	"github.com/ezemacchi/ekanban/internal/look"
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
		if widths[col] == 0 {
			continue
		}
		inner := widths[col] - screen.Gutter
		for i, sp := range m.columnSpaces(col) {
			card, choices := m.renderCard(sp, col == m.col && i == m.rowInCol, inner)
			c := screen.Card{Lines: card, Choices: choices}
			if choices >= 0 {
				c.NChoices = len(m.board.Statuses)
			}
			cols[col].Cards = append(cols[col].Cards, c)
		}
	}
	b.WriteString(screen.Draw(&m.zones, cols, widths, from, end, linesIn(b.String()), m.listHeight()))

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
			Selected: -1,
		}
		if col == m.col {
			cols[col].Selected = m.rowInCol
		}
	}
	return cols
}

// renderCard draws a space as a boxed card width cells wide. The border, not
// a marker, shows the cursor and a card picked up to move. While the status
// picker is open the selected card expands to hold its choices; choices is
// the card line of the first one, -1 when there are none.
func (m *Model) renderCard(sp *space, selected bool, width int) (card []string, choices int) {
	held := sp.Key == m.grabbed
	text := look.CardInner(width)

	label := m.spaceLabel(sp)
	if m.hasBell(sp.Key) {
		label = bellGlyph + " " + label
	}
	name := truncate(label, text)
	switch {
	case held:
		name = grabStyle.Render(name)
	case selected:
		name = cursorStyle.Render(name)
	case !sp.Live:
		name = archivedStyle.Render(name)
	case sp.Focused:
		name = focusStyle.Render(name)
	}
	lines := []string{name}
	if info, ok := m.pipeInfo[sp.Key]; ok && m.pipelineOn() && info.Title != "" {
		lines = append(lines, dimStyle.Render(truncate(info.Title, text)))
	}

	if sp.Note != "" {
		for _, line := range screen.Wrap(sp.Note, text) {
			lines = append(lines, noteStyle.Render(line))
		}
	}
	if m.pipelineOn() {
		lines = append(lines, m.pipelineLines(sp, text)...)
	} else {
		if pr, ok := m.prFor(sp.Key); ok {
			lines = append(lines, prStyled(pr, text))
		}
		if hint := agentHint(sp); hint != "" {
			lines = append(lines, dimStyle.Render(truncate(hint, text)))
		}
	}

	choices = -1
	if selected && m.mode == modeStatusPick {
		picker := m.pickerLines(text)
		// Past the top border and the lines above, after the caption.
		choices = 1 + len(lines) + 1
		lines = append(lines, picker...)
	}

	border := look.CardBorder
	switch {
	case held:
		border = look.CardGrabbed
	case selected:
		border = look.CardSelected
	}
	return look.Card(lines, width, border), choices
}
