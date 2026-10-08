package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/herdr-phin-board/internal/look"
)

// Kanban lays the same spaces out as columns, one per status. Because a column
// *is* a status, moving a card sideways is what retags it -- the horizontal
// equivalent of crossing a group boundary in the list.

const (
	minColumnWidth = 18
	maxColumnWidth = 34
	columnGutter   = 1
)

func (m *Model) viewKanbanBoard() string {
	var b strings.Builder
	b.WriteString(m.viewHeader())
	b.WriteString("\n\n")

	width := m.columnWidth()
	visible := m.visibleColumns(width)
	m.scrollColumns(visible)

	height := m.listHeight()
	end := min(m.colOffset+visible, len(m.board.Statuses))

	rendered := make([][]string, 0, visible)
	for col := m.colOffset; col < end; col++ {
		if m.statusFilter != "" && m.board.Statuses[col].ID != m.statusFilter {
			continue
		}
		rendered = append(rendered, m.renderColumn(col, width, height))
	}

	for line := 0; line < height; line++ {
		var row strings.Builder
		for _, column := range rendered {
			cell := ""
			if line < len(column) {
				cell = column[line]
			}
			row.WriteString(padCell(cell, width))
		}
		b.WriteString(" " + strings.TrimRight(row.String(), " "))
		b.WriteString("\n")
	}

	b.WriteString(m.viewFooter())
	return b.String()
}

// columnWidth divides the terminal between the statuses, within sane bounds.
func (m *Model) columnWidth() int {
	usable := m.width - 1
	if usable < minColumnWidth {
		return minColumnWidth
	}
	if n := len(m.board.Statuses); n > 0 {
		if w := usable / n; w >= minColumnWidth {
			return min(w, maxColumnWidth)
		}
	}
	return minColumnWidth
}

func (m *Model) visibleColumns(width int) int {
	if width <= 0 {
		return 1
	}
	n := (m.width - 1) / width
	if n < 1 {
		n = 1
	}
	return min(n, len(m.board.Statuses))
}

// scrollColumns keeps the selected column on screen when the statuses do not
// all fit at once.
func (m *Model) scrollColumns(visible int) {
	if m.col < m.colOffset {
		m.colOffset = m.col
	}
	if m.col >= m.colOffset+visible {
		m.colOffset = m.col - visible + 1
	}
	maxOffset := len(m.board.Statuses) - visible
	if m.colOffset > maxOffset {
		m.colOffset = maxOffset
	}
	if m.colOffset < 0 {
		m.colOffset = 0
	}
}

func (m *Model) renderColumn(col, width, height int) []string {
	st := m.board.Statuses[col]
	group := m.columnSpaces(col)
	inner := width - columnGutter

	headStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color)).Bold(true)
	lines := []string{
		headStyle.Render(truncate(m.statusLabel(st), inner-4)) + dimStyle.Render(fmt.Sprintf(" %d", len(group))),
		dimStyle.Render(strings.Repeat("─", inner)),
	}

	if len(group) == 0 {
		lines = append(lines, dimStyle.Render(truncate("—", inner)))
	}

	// Track where the selected card ends so the column can be scrolled to it.
	selectedEnd := -1
	for i, sp := range group {
		lines = append(lines, m.renderCard(sp, col == m.col && i == m.rowInCol, inner)...)
		if col == m.col && i == m.rowInCol {
			selectedEnd = len(lines)
		}
	}

	if selectedEnd >= 0 && len(lines) > height {
		// Keep the header visible where possible, otherwise follow the card.
		if overflow := selectedEnd - height; overflow > 0 {
			keep := append([]string{}, lines[:2]...)
			lines = append(keep, lines[2+overflow:]...)
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// renderCard draws a space as a boxed card width cells wide. The border, not
// a marker, shows the cursor and a card picked up to move.
func (m *Model) renderCard(sp *space, selected bool, width int) []string {
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

	if sp.Note != "" {
		for _, line := range wrap(sp.Note, text) {
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

	border := look.CardBorder
	switch {
	case held:
		border = look.CardGrabbed
	case selected:
		border = look.CardSelected
	}
	return look.Card(lines, width, border)
}

// padCell pads a rendered cell to width, ignoring ANSI escapes.
func padCell(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// wrap breaks text on word boundaries, splitting words that are too long.
func wrap(s string, width int) []string {
	if width < 4 {
		width = 4
	}
	var lines []string
	var current string

	for _, word := range strings.Fields(s) {
		for lipgloss.Width(word) > width {
			head := string([]rune(word)[:width])
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			lines = append(lines, head)
			word = string([]rune(word)[width:])
		}
		switch {
		case current == "":
			current = word
		case lipgloss.Width(current)+1+lipgloss.Width(word) <= width:
			current += " " + word
		default:
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
