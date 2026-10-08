package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/screen"
)

// Menus open over the board: the view switcher, from the title, and the
// selected card's actions, from its ⋯ button or the menu key. Both are a
// screen.Menu; a choice in the view switcher changes the layout, one in a
// card's menu presses the key it names, so the menu does exactly what the
// key would.

// popup is the open menu.
type popup struct {
	screen.Menu
	x, y  int
	views bool // the view switcher
}

var menuLayouts = []layout{layoutList, layoutTable, layoutKanban}

// title is the view switcher's label: a caret and the current view's name.
func (m *Model) title() string {
	caret := "▾"
	if m.menu != nil && m.menu.views {
		caret = "▴"
	}
	return caret + " " + titleCase(m.layout.String())
}

func (m *Model) menuOpen() bool { return m.menu != nil }

// openMenu drops the view switcher open under the title, the current view
// highlighted.
func (m *Model) openMenu() {
	mn := popup{x: 1, y: 1, views: true}
	for i, l := range menuLayouts {
		mn.Items = append(mn.Items, screen.MenuItem{Label: titleCase(l.String())})
		if l == m.layout {
			mn.Sel = i
		}
	}
	m.menu = &mn
}

// openCardMenu opens the selected card's actions at x, y; below the card
// when x and y are negative.
func (m *Model) openCardMenu(x, y int) {
	sp := m.selected()
	if sp == nil {
		m.status = "no space selected"
		return
	}
	mn := popup{Menu: screen.Menu{Title: m.spaceName(sp)}}
	for _, a := range m.cardActions(sp) {
		if k := m.hintKey(a.action); k != "" {
			mn.Items = append(mn.Items, screen.MenuItem{Label: a.menu, Key: k})
		}
	}
	if len(mn.Items) == 0 {
		return
	}
	if x < 0 || y < 0 {
		x, y = m.cardCorner()
	}
	mn.x, mn.y = x, y
	m.menu = &mn
}

// cardCorner is where a card's menu opens from the keyboard: over the
// selected card's last line, indented, or the middle of the screen when the
// layout draws no cards.
func (m *Model) cardCorner() (int, int) {
	x, y := -1, -1
	for _, z := range m.zones {
		if z.Kind == screen.OnCard && z.Col == m.col && z.Row == m.rowInCol {
			if x < 0 || z.X0 < x {
				x = z.X0
			}
			y = max(y, z.Y)
		}
	}
	if x < 0 {
		return m.width / 3, m.height / 3
	}
	return x + 2, y
}

// chooseMenu does what entry i of the open menu stands for, and closes it.
func (m *Model) chooseMenu(i int) (tea.Model, tea.Cmd) {
	mn := m.menu
	m.menu = nil
	if mn == nil || i < 0 || i >= len(mn.Items) {
		return m, nil
	}
	if mn.views {
		if l := menuLayouts[i]; l != m.layout {
			m.setLayout(l)
		}
		return m, nil
	}
	return m.pressKey(mn.Items[i].Key)
}

// pressKey does what pressing k does, k being a key or a chord such as gp,
// which is its keys one after the other.
func (m *Model) pressKey(k string) (tea.Model, tea.Cmd) {
	if len(k) == 2 && k[0] == 'g' {
		m.handleKey(screen.KeyMsg("g"))
		k = k[1:]
	}
	return m.handleKey(screen.KeyMsg(k))
}

// handleMenuKey drives the open menu from the keyboard.
func (m *Model) handleMenuKey(k string) (tea.Model, tea.Cmd) {
	chosen, done := m.menu.Key(k)
	if !done {
		return m, nil
	}
	if chosen < 0 {
		m.menu = nil
		return m, nil
	}
	return m.chooseMenu(chosen)
}

// overlayMenu draws the open menu over a rendered frame.
func (m *Model) overlayMenu(frame string) string {
	if m.menu == nil {
		return frame
	}
	return m.menu.Over(&m.zones, frame, m.menu.x, m.menu.y, m.width, m.height)
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
