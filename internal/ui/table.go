package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/screen"
)

// The table is the flat view: every space on one line, in aligned columns,
// including the fields the list has no room for. It has no groups and no
// collapse, so it is the view for scanning everything at once, and the only
// one you can re-sort.

type tableSort int

const (
	sortStatus tableSort = iota
	sortName
	sortChanged
)

func (s tableSort) String() string {
	switch s {
	case sortName:
		return "name"
	case sortChanged:
		return "changed"
	default:
		return "status"
	}
}

func parseSort(s string) tableSort {
	switch s {
	case "name":
		return sortName
	case "changed":
		return sortChanged
	default:
		return sortStatus
	}
}

func (s tableSort) next() tableSort { return (s + 1) % 3 }

var tableHeadStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)

// buildFlat orders every space for the table. Sorting by status reuses the
// group order exactly, so a row's position matches the list and grab-moves
// stay meaningful.
func (m *Model) buildFlat() {
	m.flat = m.flat[:0]
	for _, st := range m.board.Statuses {
		if m.statusFilter != "" && st.ID != m.statusFilter {
			continue
		}
		m.flat = append(m.flat, m.groups[st.ID]...)
	}
	if m.sort == sortStatus {
		return
	}

	sort.SliceStable(m.flat, func(i, j int) bool {
		a, b := m.flat[i], m.flat[j]
		switch m.sort {
		case sortName:
			return strings.ToLower(a.Label) < strings.ToLower(b.Label)
		default:
			if a.UpdatedAt.Equal(b.UpdatedAt) {
				return strings.ToLower(a.Label) < strings.ToLower(b.Label)
			}
			return a.UpdatedAt.After(b.UpdatedAt)
		}
	})
}

// tableWidths are the table's column widths; 0 leaves a column out. On the
// computed board name is the ticket, note its title and state the fact that
// says most; agent and pr are out, since the state says them.
type tableWidths struct {
	name, status, note, branch, pr, state, agent, changed int
}

func (m *Model) tableWidths() tableWidths {
	statusW := 8
	for _, st := range m.board.Statuses {
		if w := lipgloss.Width(st.Label); w > statusW {
			statusW = w
		}
	}
	statusW = min(statusW, 14)

	w := tableWidths{name: 20, status: statusW, agent: 8, changed: 10}
	if m.pipelineOn() {
		w.name, w.agent, w.state = 14, 0, 34
	}

	// A column only earns its space when something on the board would fill it.
	if m.anyPR() {
		w.pr = 24
	}
	if m.anyBranch() {
		w.branch = 16
	}
	// Tickets the board placed by itself have never been changed by hand.
	if m.pipelineOn() && !m.anyChanged() {
		w.changed = 0
	}

	gaps := 2
	for _, optional := range []int{w.pr, w.branch, w.state, w.agent, w.changed} {
		if optional > 0 {
			gaps++
		}
	}
	fixed := w.name + w.status + w.branch + w.pr + w.state + w.agent + w.changed + gaps + 3
	w.note = m.width - fixed
	if w.note < 12 {
		// Give the note room by squeezing the name before dropping columns.
		shrink := min(12-w.note, w.name-10)
		w.name -= shrink
		w.note += shrink
	}
	if w.note < 1 {
		w.note = 1
	}
	return w
}

// anyPR reports whether the board has PR context worth a column.
func (m *Model) anyPR() bool {
	if m.prCache == nil {
		return false
	}
	for _, group := range m.groups {
		for _, sp := range group {
			if _, ok := m.prFor(sp.Key); ok {
				return true
			}
		}
	}
	return false
}

// tableColumn is one heading of the table: its words, its width, and the
// order a click on it sorts by (-1 for none).
type tableColumn struct {
	head  string
	width int
	sort  tableSort
}

func (m *Model) tableColumns(w tableWidths) []tableColumn {
	name, note := "SPACE", "NOTE"
	if m.pipelineOn() {
		name, note = "TICKET", "TITLE"
	}
	cols := []tableColumn{{name, w.name, sortName}, {"STATUS", w.status, sortStatus}, {note, w.note, -1}}
	for _, c := range []tableColumn{{"STATE", w.state, -1}, {"BRANCH", w.branch, -1}, {"PR", w.pr, -1}, {"AGENT", w.agent, -1}} {
		if c.width > 0 {
			cols = append(cols, c)
		}
	}
	if w.changed > 0 {
		cols = append(cols, tableColumn{"CHANGED", w.changed, sortChanged})
	}
	return cols
}

// anyChanged reports whether any space was ever changed by hand.
func (m *Model) anyChanged() bool {
	for _, sp := range m.flat {
		if !sp.UpdatedAt.IsZero() {
			return true
		}
	}
	return false
}

// tableTop is the row the table's headings are on: under the top bar and
// a blank line, like the other views. Its rows start on the next.
const tableTop = 2

func (m *Model) viewTable() string {
	var b strings.Builder
	b.WriteString(m.viewHeader())
	b.WriteString("\n\n")

	// The headings that sort are buttons: a click sorts by them.
	w := m.tableWidths()
	head := "   "
	x := 3
	for i, c := range m.tableColumns(w) {
		if i > 0 {
			head += " "
			x++
		}
		text := pad(m.sortMarker(c.sort)+c.head, c.width)
		if c.sort >= 0 {
			m.addZone(zone{Kind: screen.OnControl, ID: "sort:" + c.sort.String(), Y: tableTop, X0: x, X1: x + c.width})
		}
		head += text
		x += c.width
	}
	b.WriteString(tableHeadStyle.Render(truncate(head, m.width)))
	b.WriteString("\n")

	height := m.listHeight() - 1 // the header row costs a line
	if len(m.flat) == 0 && m.filter != "" {
		b.WriteString(m.noMatch(m.width) + "\n")
		height--
	}
	end := min(m.offset+height, len(m.flat))
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderTableRow(i, w))
		b.WriteString("\n")
	}
	for i := end - m.offset; i < height; i++ {
		b.WriteString("\n")
	}

	footer := m.viewFooter()
	m.placeFooter(linesIn(b.String()))
	b.WriteString(footer)
	return b.String()
}

// sortMarker flags which column the table is ordered by.
func (m *Model) sortMarker(s tableSort) string {
	if m.sort == s {
		return "↓"
	}
	return ""
}

func (m *Model) renderTableRow(i int, w tableWidths) string {
	sp := m.flat[i]
	held := sp.Key == m.grabbed

	prefix := "   "
	switch {
	case held:
		prefix = grabStyle.Render(" ▌ ")
	case i == m.cursor:
		prefix = cursorStyle.Render(" ❯ ")
	}

	label := sp.Label
	if m.pipelineOn() {
		label = m.spaceName(sp)
	}
	style := labelStyle
	switch {
	case held:
		style = grabStyle
	case !sp.Live:
		style = archivedStyle
	case sp.Focused:
		style = focusStyle
	}
	name := m.marked(sp, style.Render(truncate(label, w.name-2)))
	name = screen.Pad(name, w.name)

	statusCell := pad("", w.status)
	if st, ok := m.board.StatusByID(sp.StatusID); ok {
		statusCell = lipgloss.NewStyle().
			Foreground(lipgloss.Color(st.Color)).
			Render(pad(st.Label, w.status))
	}

	note := dimStyle.Render(pad("—", w.note))
	switch {
	case m.pipelineOn():
		note = screen.Pad(m.about(sp, w.note), w.note)
	case sp.Note != "":
		note = noteStyle.Render(pad(sp.Note, w.note))
	}

	changed := "—"
	if !sp.UpdatedAt.IsZero() {
		changed = humanAge(sp.UpdatedAt)
	}

	cells := []string{name, statusCell, note}
	if w.state > 0 {
		cell := dimStyle.Render(pad("—", w.state))
		if f, ok := m.heaviest(sp); ok {
			cell = screen.Pad(m.line(f, w.state), w.state)
		}
		cells = append(cells, cell)
	}
	if w.branch > 0 {
		cell := dimStyle.Render(pad("—", w.branch))
		if b := m.branchFor(sp.Key); b != "" {
			cell = branchStyle.Render(pad(b, w.branch))
		}
		cells = append(cells, cell)
	}
	if w.pr > 0 {
		cell := dimStyle.Render(pad("—", w.pr))
		if pr, ok := m.prFor(sp.Key); ok {
			cell = pad(prStyled(pr, w.pr), w.pr)
		}
		cells = append(cells, cell)
	}
	if w.agent > 0 {
		agent := "—"
		if sp.Live && sp.AgentStatus != "" {
			agent = sp.AgentStatus
		} else if !sp.Live {
			agent = "offline"
		}
		cells = append(cells, dimStyle.Render(pad(agent, w.agent)))
	}
	if w.changed > 0 {
		cells = append(cells, dimStyle.Render(pad(changed, w.changed)))
	}
	return prefix + strings.Join(cells, " ")
}
