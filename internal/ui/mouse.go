package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Clickable zones. Every frame records where it drew what a click can act on
// -- a card, a column header, a status choice, a button -- so a click is
// tested against what is on screen now. Later zones sit on top: a modal's
// zones win over the board under it.

type zoneKind int

const (
	zoneCard   zoneKind = iota // a kanban card: col, row
	zoneColumn                 // a kanban column header: col
	zoneStatus                 // a status choice of the picker: status index
	zoneButton                 // a key hint: key
	zoneModal                  // the detail modal's box; a click outside closes it
)

type zone struct {
	kind     zoneKind
	y        int
	x0, x1   int // x1 exclusive
	h        int // rows covered, 1 when 0
	col, row int
	status   int
	key      string
	footer   bool // y counts from the footer's first line until placeFooter
}

func (m *Model) resetZones() { m.zones = m.zones[:0] }

func (m *Model) addZone(z zone) { m.zones = append(m.zones, z) }

// placeFooter turns the footer's zones into screen rows once the rows above
// the footer are known.
func (m *Model) placeFooter(top int) {
	for i := range m.zones {
		if m.zones[i].footer {
			m.zones[i].y += top
			m.zones[i].footer = false
		}
	}
}

// linesIn is how many rows s takes.
func linesIn(s string) int { return strings.Count(s, "\n") }

// zoneAt is the topmost zone under a point.
func (m *Model) zoneAt(x, y int) (zone, bool) {
	for i := len(m.zones) - 1; i >= 0; i-- {
		z := m.zones[i]
		h := max(z.h, 1)
		if y >= z.y && y < z.y+h && x >= z.x0 && x < z.x1 {
			return z, true
		}
	}
	return zone{}, false
}

// hint is one clickable key hint: what to press and what it does.
type hint struct{ key, label string }

// buttons draws hints as "key label · key label", recording each as a
// button on footer row line, starting at column x.
func (m *Model) buttons(line, x int, hints []hint, width int) string {
	var b strings.Builder
	used := 0
	sep := dimStyle.Render(" · ")
	for i, h := range hints {
		if h.key == "" {
			continue
		}
		text := keyStyle.Render(h.key) + dimStyle.Render(" "+h.label)
		w := lipgloss.Width(text)
		gap := 0
		if i > 0 && used > 0 {
			gap = 3
		}
		if used+gap+w > width {
			break
		}
		if gap > 0 {
			b.WriteString(sep)
		}
		m.addZone(zone{kind: zoneButton, y: line, x0: x + used + gap, x1: x + used + gap + w, key: h.key, footer: true})
		b.WriteString(text)
		used += gap + w
	}
	return b.String()
}

// keyMsg is the key event a button press stands for.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "space", " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// clickZone acts on a click that landed on a zone; false when there was none.
func (m *Model) clickZone(x, y int) (bool, tea.Model, tea.Cmd) {
	z, ok := m.zoneAt(x, y)
	if !ok {
		if m.mode == modeStatusPick {
			// A click away from the choices puts the picker down.
			m.mode = m.inputParentMode()
			return true, m, nil
		}
		if m.mode == modeDetail {
			m.mode = modeNormal
			return true, m, nil
		}
		return false, m, nil
	}
	switch z.kind {
	case zoneButton:
		model, cmd := m.handleKey(keyMsg(z.key))
		return true, model, cmd
	case zoneStatus:
		if z.status >= 0 && z.status < len(m.board.Statuses) {
			m.mode = m.inputParentMode()
			model, cmd := m.applyStatus(m.board.Statuses[z.status])
			return true, model, cmd
		}
	case zoneColumn:
		if m.mode != modeNormal {
			return true, m, nil
		}
		m.col, m.rowInCol = z.col, 0
		m.clampColumnCursor()
		return true, m, nil
	case zoneCard:
		if m.mode == modeStatusPick {
			m.mode = m.inputParentMode()
			return true, m, nil
		}
		if m.mode != modeNormal {
			return true, m, nil
		}
		already := m.col == z.col && m.rowInCol == z.row
		m.col, m.rowInCol = z.col, z.row
		m.clampColumnCursor()
		model, cmd := m.spaceClick(already)
		return true, model, cmd
	case zoneModal:
		return true, m, nil // inside the box, on nothing clickable
	}
	return true, m, nil
}

// pickerStep moves the inline status picker's highlight, wrapping around, so
// pressing s again walks the choices.
func (m *Model) pickerStep(delta int) {
	n := len(m.board.Statuses)
	if n == 0 {
		return
	}
	m.manageIdx = ((m.manageIdx+delta)%n + n) % n
}

// pickerLines are the status choices drawn inside the expanded card or the
// detail modal: a caption, then one status per line, the highlighted one
// marked. The caller records their zones, since it knows where they land.
func (m *Model) pickerLines(width int) []string {
	lines := []string{dimStyle.Render(truncate("set status:", width))}
	for i, st := range m.board.Statuses {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color))
		mark := "  "
		if i == m.manageIdx {
			mark = cursorStyle.Render("❯ ")
			style = style.Bold(true).Underline(true)
		}
		label := st.Label
		if i < 9 {
			label = string(rune('1'+i)) + " " + label
		}
		lines = append(lines, mark+style.Render(truncate(label, width-2)))
	}
	return lines
}

// pickerBar is the picker on one line, for layouts with no card to expand:
// the list and the table. Its choices are buttons on footer row line.
func (m *Model) pickerBar(line int, width int) string {
	var b strings.Builder
	b.WriteString(" " + dimStyle.Render("status:"))
	x := 1 + len("status:")
	for i, st := range m.board.Statuses {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color))
		text := " " + st.Label + " "
		if i == m.manageIdx {
			style = style.Reverse(true).Bold(true)
		}
		w := lipgloss.Width(text)
		if x+1+w > width {
			break
		}
		b.WriteString(" " + style.Render(text))
		m.addZone(zone{kind: zoneStatus, y: line, x0: x + 1, x1: x + 1 + w, status: i, footer: true})
		x += 1 + w
	}
	return b.String()
}
