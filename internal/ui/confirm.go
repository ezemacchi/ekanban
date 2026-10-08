package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/screen"
)

// question is a yes-or-no question over the board: what it asks, what yes
// costs, and what yes does. Anything that cannot be undone asks first.
type question struct {
	title string
	lines []string
	yes   func() (tea.Model, tea.Cmd)
	prev  mode
}

// confirm asks title before doing yes.
func (m *Model) confirm(title string, lines []string, yes func() (tea.Model, tea.Cmd)) {
	m.ask = &question{title: title, lines: lines, yes: yes, prev: m.mode}
	m.mode = modeConfirm
}

// answer takes a key, or a click on an answer: y or Y does it, n, N or esc
// does not, and any other key leaves the question open.
func (m *Model) answer(k string) (tea.Model, tea.Cmd) {
	yes, no := screen.ConfirmKeys(k)
	if !yes && !no {
		return m, nil
	}
	q := m.ask
	m.ask = nil
	m.mode = q.prev
	if m.mode == modeDetail || m.mode == modeConfirm {
		m.mode = modeNormal
	}
	if no {
		m.status = "nothing was changed"
		return m, nil
	}
	return q.yes()
}
