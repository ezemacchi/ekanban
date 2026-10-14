package ticketui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// x on a question closes it without an answer: the ticket moved on and
// nobody needs it any more. It asks why, "no longer needed" already written,
// and closes it in the run's files once you say yes (ticket.CloseQuestion).
// No agent is asked: moving a line is not work worth an agent's tokens. A
// running orchestrator reads the question closed the next time it reads the
// state file.

// closing is a question being closed: its reason being written, then, with
// sure set, the board asking yes or no.
type closing struct {
	q      ticket.Question
	input  textinput.Model
	sure   bool
	reason string
}

type closedMsg struct {
	words string
	err   error
}

// defaultReason is what a question closed without an answer says when you
// give no other reason.
const defaultReason = "no longer needed"

// startClose asks why the selected question is closed.
func (m *Model) startClose() tea.Cmd {
	q, kind, ok := m.selectedQuestion()
	switch {
	case !ok:
		m.status = "select a question to close it"
		return nil
	case kind == kindClosed:
		m.status = "the question is already closed"
		return nil
	}
	in := textinput.New()
	in.Prompt = ""
	in.SetValue(defaultReason)
	in.CursorEnd()
	in.Focus()
	m.closing = &closing{q: q, input: in}
	return textinput.Blink
}

// closeKey takes a key while a question is being closed.
func (m *Model) closeKey(msg tea.KeyMsg) tea.Cmd {
	c := m.closing
	if c.sure {
		yes, no := screen.ConfirmKeys(msg.String())
		switch {
		case no:
			m.closing = nil
			m.status = "nothing was changed"
		case yes:
			m.closing = nil
			return m.closeInRun(c.q, c.reason)
		}
		return nil
	}
	switch msg.String() {
	case "esc":
		m.closing = nil
		m.status = "nothing was changed"
		return nil
	case "enter":
		reason := strings.TrimSpace(c.input.Value())
		if reason == "" {
			reason = defaultReason
		}
		return m.closeWith(reason)
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return cmd
}

// closeWith takes the reason and asks yes or no before writing the run.
func (m *Model) closeWith(reason string) tea.Cmd {
	m.closing.sure, m.closing.reason = true, reason
	return nil
}

// closeInRun closes q in the run's files.
func (m *Model) closeInRun(q ticket.Question, reason string) tea.Cmd {
	run := m.run
	return func() tea.Msg {
		return closedMsg{words: q.Words, err: ticket.CloseQuestion(run, q, reason, time.Now())}
	}
}

// closeView draws the reason's box, or the question whether to close, over
// base.
func (m *Model) closeView(base string, width, height int) string {
	c := m.closing
	if c.sure {
		lines := []string{"Its line moves to the decisions as:", ticket.ClosedLine(c.q, c.reason, time.Now())}
		if c.q.Brief != nil {
			lines = append(lines, "and its brief "+c.q.BriefPath+" is closed.")
		}
		if m.run.OrchestratorPane != "" {
			lines = append(lines, ticket.OrchestratorReadsIt)
		}
		return screen.Confirm(&m.zones, base, "Close it without an answer?", lines, width, height)
	}
	lines := []string{c.q.Words, "", "Why it no longer needs an answer:"}
	field := func(inner int) string {
		c.input.Width = max(screen.InputRoom("›", inner)-1, 1)
		return screen.Input("›", "", defaultReason, true, c.input.View(), inner)
	}
	return screen.Asking(&m.zones, base, "Close the question", lines, field, width, height)
}
