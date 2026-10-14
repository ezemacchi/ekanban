package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/screen"
)

// The chooser is a list with a preview of the highlighted entry beside it, in
// a box over the board or over the broadcast: which agent gets a message,
// with the last lines of its pane; a ticket's open questions, with the whole
// of the highlighted one and its links. What its rows are, what the preview
// shows and what choosing does are the caller's; driving it is here.

type chooserState struct {
	ch   screen.Chooser
	back mode // what was open under it, where esc goes back to
	// use is choosing row i; show fills the preview for row i, or returns
	// the command that will.
	use  func(i int) (tea.Model, tea.Cmd)
	show func(i int) tea.Cmd
	// key handles a key the chooser does not, on row i: false lets it go.
	key func(k string, i int) (tea.Model, tea.Cmd, bool)
	// live reads the preview again every previewEvery while open; ticking is
	// whether that clock runs.
	live, ticking bool
	// rows are the agents, for the agent chooser.
	rows []pickRow
}

type chooserTickMsg struct{}

// previewEvery is how often a live preview is read again.
const previewEvery = time.Second

// openChooser opens s over what is open now.
func (m *Model) openChooser(s chooserState) {
	s.back = m.mode
	m.ap = s
	m.mode = modeChooser
}

// showSelected fills the preview for the highlighted row, and starts the
// clock of a live one.
func (m *Model) showSelected() tea.Cmd {
	m.ap.ch.Preview = nil
	if m.ap.show == nil || m.ap.ch.Sel < 0 || m.ap.ch.Sel >= len(m.ap.ch.Rows) {
		return nil
	}
	return tea.Batch(m.ap.show(m.ap.ch.Sel), m.startChooserClock())
}

func (m *Model) startChooserClock() tea.Cmd {
	if !m.ap.live || m.ap.ticking {
		return nil
	}
	m.ap.ticking = true
	return tea.Tick(previewEvery, func(time.Time) tea.Msg { return chooserTickMsg{} })
}

func (m *Model) onChooserTick() tea.Cmd {
	m.ap.ticking = false
	if m.mode != modeChooser || m.ap.show == nil {
		return nil
	}
	return tea.Batch(m.ap.show(m.ap.ch.Sel), m.startChooserClock())
}

// useChosen does what choosing the highlighted row does, unless it is off.
func (m *Model) useChosen() (tea.Model, tea.Cmd) {
	i := m.ap.ch.Sel
	if i < 0 || i >= len(m.ap.ch.Rows) {
		return m, nil
	}
	if r := m.ap.ch.Rows[i]; r.Off {
		m.status = screen.Plain(ansi.Strip(r.Text)) + ": " + r.Why
		return m, nil
	}
	m.mode = m.ap.back
	return m.ap.use(i)
}

// cancelChooser closes it, back to what was under it.
func (m *Model) cancelChooser() {
	m.mode = m.ap.back
	if m.mode == modeNormal {
		m.status = "nothing was sent"
	}
}

// handleChooserKey drives the chooser. Like a menu's, these keys are its own.
func (m *Model) handleChooserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down", "tab":
		m.ap.ch.Move(1)
		return m, m.showSelected()
	case "k", "up", "shift+tab":
		m.ap.ch.Move(-1)
		return m, m.showSelected()
	case "enter":
		return m.useChosen()
	case "esc", "q":
		m.cancelChooser()
	default:
		if m.ap.key != nil && m.ap.ch.Sel >= 0 && m.ap.ch.Sel < len(m.ap.ch.Rows) {
			if model, cmd, ok := m.ap.key(msg.String(), m.ap.ch.Sel); ok {
				return model, cmd
			}
		}
	}
	return m, nil
}

// clickChooser acts on a click while the chooser is open: a row is
// highlighted, a second click on it chooses it, the buttons press their keys;
// a click elsewhere does nothing.
func (m *Model) clickChooser(x, y int) (tea.Model, tea.Cmd) {
	z, ok := m.zoneAt(x, y)
	switch {
	case !ok:
	case z.Kind == screen.OnChoice && z.Choice < len(m.ap.ch.Rows):
		if z.Choice == m.ap.ch.Sel {
			return m.useChosen()
		}
		m.ap.ch.Sel = z.Choice
		return m, m.showSelected()
	case z.Kind == zoneButton:
		return m.handleChooserKey(screen.KeyMsg(z.Key))
	}
	return m, nil
}

// viewChooser draws the chooser over what is open under it.
func (m *Model) viewChooser() string {
	m.mode = m.ap.back
	base := m.View()
	m.mode = modeChooser
	m.resetZones()
	return m.ap.ch.Over(&m.zones, base, m.width, m.height)
}
