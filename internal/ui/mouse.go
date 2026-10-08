package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/screen"
)

// Clickable zones are screen.Zones: every frame records where it drew what a
// click can act on -- a card, a column header, a status choice, a button --
// so a click is tested against what is on screen now.

type zone = screen.Zone

const (
	zoneCard   = screen.OnCard   // a kanban card: Col, Row
	zoneColumn = screen.OnColumn // a kanban column header: Col
	zoneStatus = screen.OnChoice // a status choice of the picker: Choice
	zoneButton = screen.OnButton // a key hint: Key
	zoneModal  = screen.OnBox    // the detail modal's box; a click outside closes it
)

func (m *Model) resetZones() { m.zones.Reset() }

func (m *Model) addZone(z zone) { m.zones.Add(z) }

func (m *Model) placeFooter(top int) { m.zones.PlaceFooter(top) }

func (m *Model) zoneAt(x, y int) (zone, bool) { return m.zones.At(x, y) }

var linesIn = screen.LinesIn

// hint is one clickable key hint: what to press and what it does.
type hint = screen.Hint

// buttons draws hints as footer buttons on footer row line, from column x.
func (m *Model) buttons(line, x int, hints []hint, width int) string {
	mark := len(m.zones)
	out := screen.Buttons(&m.zones, line, x, hints, width)
	m.zones.MarkFooter(mark)
	return out
}

// clickZone acts on a click that landed on a zone; false when there was none.
func (m *Model) clickZone(x, y int) (bool, tea.Model, tea.Cmd) {
	z, ok := m.zoneAt(x, y)
	if m.menu != nil {
		// A menu takes the click: a choice chooses, anywhere else closes it,
		// as an open menu does.
		switch {
		case ok && z.Kind == zoneStatus:
			model, cmd := m.chooseMenu(z.Choice)
			return true, model, cmd
		case ok && z.Kind == zoneModal:
		default:
			m.menu = nil
		}
		return true, m, nil
	}
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
	switch z.Kind {
	case zoneButton:
		model, cmd := m.pressKey(z.Key)
		return true, model, cmd
	case screen.OnControl:
		model, cmd := m.control(z)
		return true, model, cmd
	case zoneStatus:
		if z.Choice >= 0 && z.Choice < len(m.board.Statuses) {
			m.mode = m.inputParentMode()
			model, cmd := m.applyStatus(m.board.Statuses[z.Choice])
			return true, model, cmd
		}
	case zoneColumn:
		if m.mode != modeNormal {
			return true, m, nil
		}
		// A column's name narrows the board to it; the chip it leaves in
		// the top bar, or a second click, brings the rest back.
		m.col, m.rowInCol = z.Col, 0
		m.toggleStatusFilter()
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
		already := m.col == z.Col && m.rowInCol == z.Row
		m.col, m.rowInCol = z.Col, z.Row
		m.clampColumnCursor()
		model, cmd := m.spaceClick(already)
		return true, model, cmd
	case zoneModal:
		return true, m, nil // inside the box, on nothing clickable
	}
	return true, m, nil
}

// control does what a click on one of the board's own controls means.
func (m *Model) control(z zone) (tea.Model, tea.Cmd) {
	switch z.ID {
	case "views":
		if m.menu != nil {
			m.menu = nil
		} else {
			m.openMenu()
		}
	case "search":
		m.startSearch()
	case "clear-search":
		m.filter = ""
		m.archiveIdx = 0
		m.rebuild()
	case "clear-only":
		if m.statusFilter != "" {
			m.toggleStatusFilter()
		}
	case "fold":
		if m.mode == modeNormal {
			m.toggleFold(z.Col)
		}
	default:
		if by, ok := strings.CutPrefix(z.ID, "sort:"); ok && m.mode == modeNormal {
			m.setSort(parseSort(by))
		}
	}
	return m, nil
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
		m.addZone(zone{Kind: zoneStatus, Y: line, X0: x + 1, X1: x + 1 + w, Choice: i, Footer: true})
		x += 1 + w
	}
	return b.String()
}
