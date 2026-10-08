package ui

import tea "github.com/charmbracelet/bubbletea"

// Herdr opens URLs it finds in pane content when you click them -- but only in
// panes that have not taken the mouse. This board has, for the view switcher,
// so Herdr never sees the click and the links are ours to handle.
//
// Doing it ourselves has one advantage: the visible text can be a short label
// rather than a raw URL, which at pane width would wrap across two lines and
// break the click even under Herdr's own handling.

// linkRegion is where a URL, or a field to edit, was drawn on the last frame.
type linkRegion struct {
	row    int
	x0, x1 int // inclusive
	url    string
	key    string // what a click presses: the field's key
}

// trackLinks records the URLs in a block of detail lines, given where the block
// was drawn. Called during render, read on the next click.
func (m *Model) trackLinks(lines []detailLine, originRow, x0, x1 int) {
	for i, l := range lines {
		if l.url == "" && l.key == "" {
			continue
		}
		m.links = append(m.links, linkRegion{row: originRow + i, x0: x0, x1: x1, url: l.url, key: l.key})
	}
}

// resetLinks clears the regions at the start of a frame.
func (m *Model) resetLinks() { m.links = m.links[:0] }

// linkAt finds the region drawn at a point, if any.
func (m *Model) linkAt(x, y int) (linkRegion, bool) {
	for _, r := range m.links {
		if y == r.row && x >= r.x0 && x <= r.x1 {
			return r, true
		}
	}
	return linkRegion{}, false
}

// openLinkAt opens whatever was clicked, or edits the field, reporting
// whether anything was there.
func (m *Model) openLinkAt(x, y int) (tea.Model, tea.Cmd, bool) {
	r, ok := m.linkAt(x, y)
	if !ok {
		return m, nil, false
	}
	if r.key != "" {
		model, cmd := m.pressKey(r.key)
		return model, cmd, true
	}
	url := r.url
	m.status = "opening " + url
	return m, func() tea.Msg {
		if err := openURL(url); err != nil {
			return errMsg{err}
		}
		return nil
	}, true
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
