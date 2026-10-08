package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
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

	widths := m.columnWidths()
	end := m.scrollColumns(widths)
	height := m.listHeight()

	type column struct {
		lines  []string
		owners []lineOwner
		width  int
	}
	var rendered []column
	for col := m.colOffset; col < end; col++ {
		if widths[col] == 0 {
			continue
		}
		lines, owners := m.renderColumn(col, widths[col], height)
		rendered = append(rendered, column{lines, owners, widths[col]})
	}

	// What each line of each column is, as zones a click can land on.
	top := linesIn(b.String())
	x := 1
	for _, c := range rendered {
		for line, o := range c.owners {
			if line >= height {
				break
			}
			z := zone{y: top + line, x0: x, x1: x + c.width - columnGutter, col: o.col, row: o.row}
			switch {
			case o.status >= 0:
				z.kind, z.status = zoneStatus, o.status
			case o.row == rowHeaderLine:
				z.kind = zoneColumn
			case o.row >= 0:
				z.kind = zoneCard
			default:
				continue
			}
			m.addZone(z)
		}
		x += c.width
	}

	for line := 0; line < height; line++ {
		var row strings.Builder
		for _, c := range rendered {
			cell := ""
			if line < len(c.lines) {
				cell = c.lines[line]
			}
			row.WriteString(padCell(cell, c.width))
		}
		b.WriteString(" " + strings.TrimRight(row.String(), " "))
		b.WriteString("\n")
	}

	footer := m.viewFooter()
	m.placeFooter(linesIn(b.String()))
	b.WriteString(footer)
	return b.String()
}

// lineOwner is what one line of a kanban column belongs to: its header, a
// card (row), a status choice of the picker inside a card, or nothing.
type lineOwner struct {
	col, row int // row: the card's index, rowHeaderLine, or -1
	status   int // a picker choice, else -1
}

const rowHeaderLine = -2

// columnWidths gives each status its width; 0 hides it (the status filter).
// An empty column takes only what its header needs, so the columns holding
// cards get the room; those share what is left, within sane bounds.
func (m *Model) columnWidths() []int {
	usable := m.width - 1
	widths := make([]int, len(m.board.Statuses))
	full, used := 0, 0
	for col, st := range m.board.Statuses {
		switch {
		case m.statusFilter != "" && st.ID != m.statusFilter:
		case m.columnCount(col) == 0:
			widths[col] = lipgloss.Width(m.columnHeader(col)) + columnGutter
			used += widths[col]
		default:
			full++
		}
	}
	if full == 0 {
		return widths
	}
	share := minColumnWidth
	if room := usable - used; room/full > minColumnWidth {
		share = min(room/full, maxColumnWidth)
	}
	for col, st := range m.board.Statuses {
		if widths[col] == 0 && (m.statusFilter == "" || st.ID == m.statusFilter) {
			widths[col] = share
		}
	}
	return widths
}

// scrollColumns keeps the selected column on screen when the columns do not
// all fit, and returns the end of the visible range.
func (m *Model) scrollColumns(widths []int) int {
	usable := m.width - 1
	span := func(from, to int) int {
		total := 0
		for _, w := range widths[from:to] {
			total += w
		}
		return total
	}
	m.colOffset = min(max(m.colOffset, 0), max(len(widths)-1, 0))
	if m.col < m.colOffset {
		m.colOffset = m.col
	}
	for m.colOffset < m.col && span(m.colOffset, m.col+1) > usable {
		m.colOffset++
	}
	// Pull earlier columns back in while they fit.
	for m.colOffset > 0 && span(m.colOffset-1, len(widths)) <= usable {
		m.colOffset--
	}
	end := m.colOffset
	for end < len(widths) && (end == m.colOffset || span(m.colOffset, end+1) <= usable) {
		end++
	}
	return max(end, min(m.col+1, len(widths)))
}

// columnHeader is a column's title with its card count.
func (m *Model) columnHeader(col int) string {
	return m.statusLabel(m.board.Statuses[col]) + fmt.Sprintf(" %d", m.columnCount(col))
}

func (m *Model) renderColumn(col, width, height int) ([]string, []lineOwner) {
	st := m.board.Statuses[col]
	group := m.columnSpaces(col)
	inner := width - columnGutter

	headStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color)).Bold(true)
	count := fmt.Sprintf(" %d", len(group))
	lines := []string{
		headStyle.Render(truncate(m.statusLabel(st), inner-len(count))) + dimStyle.Render(count),
		dimStyle.Render(strings.Repeat("─", inner)),
	}
	none := lineOwner{col: col, row: -1, status: -1}
	header := lineOwner{col: col, row: rowHeaderLine, status: -1}
	owners := []lineOwner{header, header}

	if len(group) == 0 {
		lines = append(lines, dimStyle.Render(truncate("—", inner)))
		owners = append(owners, none)
	}

	// Track where the selected card ends so the column can be scrolled to it.
	selectedEnd := -1
	for i, sp := range group {
		selected := col == m.col && i == m.rowInCol
		card, choices := m.renderCard(sp, selected, inner)
		for j := range card {
			o := lineOwner{col: col, row: i, status: -1}
			if choices >= 0 && j >= choices && j < choices+len(m.board.Statuses) {
				o.status = j - choices
			}
			owners = append(owners, o)
		}
		lines = append(lines, card...)
		if selected {
			selectedEnd = len(lines)
		}
	}

	if selectedEnd >= 0 && len(lines) > height {
		// Keep the header visible where possible, otherwise follow the card.
		if overflow := selectedEnd - height; overflow > 0 {
			lines = append(append([]string{}, lines[:2]...), lines[2+overflow:]...)
			owners = append(append([]lineOwner{}, owners[:2]...), owners[2+overflow:]...)
		}
	}
	if len(lines) > height {
		lines, owners = lines[:height], owners[:height]
	}
	return lines, owners
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
