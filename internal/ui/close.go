package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// x in the questions reader closes a question without an answer, as on the
// ticket board: it asks why, "no longer needed" already written, and closes
// it in the run's files once you say yes. No agent is asked; a running
// orchestrator reads it closed the next time it reads the state file.

// closeReasonDefault is what a question closed without an answer says when
// you give no other reason.
const closeReasonDefault = "no longer needed"

type closePlanMsg struct {
	run    *ticket.Run
	q      ticket.Question
	reason string
	err    error
}

type questionClosedMsg struct {
	words string
	err   error
}

// startCloseQuestion asks why q is closed.
func (m *Model) startCloseQuestion(q ticket.Question) {
	m.closingQ = q
	m.mode = modeCloseReason
	m.input.SetValue(closeReasonDefault)
	m.input.CursorEnd()
	m.input.Focus()
}

// handleCloseKey takes a key while the reason is being written.
func (m *Model) handleCloseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		m.status = "nothing was changed"
		return m, nil
	case "enter":
		reason := strings.TrimSpace(m.input.Value())
		if reason == "" {
			reason = closeReasonDefault
		}
		m.mode = modeNormal
		m.input.Blur()
		return m, m.planClose(m.closingQ, reason)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// planClose reads the ticket's run again, to know whether its orchestrator is
// running and to find the question as the run has it now.
func (m *Model) planClose(q ticket.Question, reason string) tea.Cmd {
	sp := m.selected()
	if sp == nil || m.client == nil {
		return nil
	}
	client, src, worktree := m.client, m.pipe, sp.Key
	return func() tea.Msg {
		live, err := ticket.ReadLive(client)
		if err != nil {
			return closePlanMsg{err: err}
		}
		run, err := src.Run(worktree, live)
		if err != nil {
			return closePlanMsg{err: err}
		}
		for _, now := range append(append([]ticket.Question(nil), run.ForYou...), run.Questions...) {
			if now.Line == q.Line {
				return closePlanMsg{run: run, q: now, reason: reason}
			}
		}
		return closePlanMsg{err: ticket.ErrQuestionMoved}
	}
}

// applyClosePlan asks yes or no, then closes the question in the run.
func (m *Model) applyClosePlan(msg closePlanMsg) tea.Cmd {
	if msg.err != nil {
		m.status = "could not close it: " + msg.err.Error()
		return nil
	}
	run, q, reason := msg.run, msg.q, msg.reason
	lines := []string{"Its line moves to the decisions as:", ticket.ClosedLine(q, reason, time.Now())}
	if q.Brief != nil {
		lines = append(lines, "and its brief "+q.BriefPath+" is closed.")
	}
	if run.OrchestratorPane != "" {
		lines = append(lines, ticket.OrchestratorReadsIt)
	}
	m.confirm("Close it without an answer?", lines, func() (tea.Model, tea.Cmd) {
		return m, func() tea.Msg {
			return questionClosedMsg{words: q.Words, err: ticket.CloseQuestion(run, q, reason, time.Now())}
		}
	})
	return nil
}

func (m *Model) applyQuestionClosed(msg questionClosedMsg) tea.Cmd {
	if msg.err != nil {
		m.status = "could not close it: " + msg.err.Error()
		return nil
	}
	m.status = "closed without an answer: " + look.Truncate(msg.words, 60)
	return m.loadPipeline(false)
}

// viewCloseReason draws the reason's box over the board.
func (m *Model) viewCloseReason(base string) string {
	lines := []string{m.closingQ.Words, "", "Why it no longer needs an answer:"}
	field := func(inner int) string {
		return screen.Input("›", "", closeReasonDefault, true, m.inputIn(screen.InputRoom("›", inner)), inner)
	}
	return screen.Asking(&m.zones, base, "Close the question", lines, field, m.width, m.height)
}
