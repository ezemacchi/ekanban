package ticketui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/screen"
)

// A run's open questions can carry links: the orchestrator writes in the
// state file a chat with the question already written for whoever answers
// it. The board shows each as a button after its question; a click opens it.
// a asks first when there is one, and offers a menu when there are several.
// What the link is -- a chat, a form -- is the run's business, not the board's.

// ask opens the open questions' link once you say yes, or lets you choose
// among them.
func (m *Model) ask() tea.Cmd {
	switch len(m.asks) {
	case 0:
		m.status = "no open question carries a link"
		return nil
	case 1:
		l := m.asks[0]
		m.sure = &l
		return nil
	}
	mn := &screen.Menu{Title: "Open"}
	for _, l := range m.asks {
		mn.Items = append(mn.Items, screen.MenuItem{Label: l.Label})
	}
	m.menu = mn
	return nil
}

// answer takes a key while the board asks whether to open a link: y opens
// it, n or esc does not, and any other key leaves the question open.
func (m *Model) answer(k string) tea.Cmd {
	yes, no := screen.ConfirmKeys(k)
	if !yes && !no {
		return nil
	}
	l := *m.sure
	m.sure = nil
	if no {
		return nil
	}
	return m.open(l)
}

// confirmView draws the question over the board.
func (m *Model) confirmView(base string, width, height int) string {
	return screen.Confirm(&m.zones, base, "Open the link?", []string{m.sure.Describe()}, width, height)
}

// openAsk opens the i-th link of the open questions.
func (m *Model) openAsk(i int) tea.Cmd {
	if i < 0 || i >= len(m.asks) {
		return nil
	}
	return m.open(m.asks[i])
}

// open opens a link in the browser, or the program that handles it.
func (m *Model) open(l links.Link) tea.Cmd {
	m.status = "opening " + l.Label
	return func() tea.Msg { _ = look.OpenURL(l.URL); return nil }
}
