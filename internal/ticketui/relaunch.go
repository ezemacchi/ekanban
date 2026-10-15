package ticketui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/orchestrator"
	"github.com/ezemacchi/ekanban/internal/screen"
)

// O relaunches the orchestrator: o only goes to the one that is open, which
// does not help when it is dead, hung at an empty prompt, or running on the
// wrong model. It asks yes or no first, because the old agent is quit and its
// conversation is lost; the new one resumes from the run's files. Nothing is
// killed, and the tab is kept (orchestrator.Relaunch).

// askRelaunch opens the yes-or-no question.
func (m *Model) askRelaunch() tea.Cmd {
	switch {
	case m.run == nil:
		return nil
	case m.opening:
		return nil // its spinner is already on the status line
	case m.run.Workspace == "":
		m.status = "no Herdr workspace is open on " + m.run.Worktree + ", so there is nowhere to relaunch the " + strings.ToLower(m.run.Orchestrator.Label)
		return nil
	}
	m.relaunching = true
	return nil
}

// answerRelaunch takes a key, or a click on an answer, while the board asks
// whether to relaunch: y does it, n or esc does not, any other key leaves the
// question open.
func (m *Model) answerRelaunch(k string) tea.Cmd {
	yes, no := screen.ConfirmKeys(k)
	if !yes && !no {
		return nil
	}
	m.relaunching = false
	if no {
		m.status = "nothing was changed"
		return nil
	}
	run, client := m.run, m.client
	m.opening = true
	start := m.working("relaunching the " + strings.ToLower(run.Orchestrator.Label) + ".")
	return tea.Batch(start, func() tea.Msg {
		text, err := orchestrator.Relaunch(client, run)
		return orchestratorMsg{text: text, err: err}
	})
}

// relaunchLines say what yes does, for the run as last read.
func (m *Model) relaunchLines() []string {
	r := m.run
	label := strings.ToLower(r.Orchestrator.Label)
	if r.OrchestratorPane == "" {
		return []string{
			"No " + label + " is running. A new one starts in the " + r.Orchestrator.Tab + " tab, or in a new tab if there is none.",
			"It resumes from the run's files.",
		}
	}
	lines := []string{
		"The " + label + " (" + orDash(look.Agent(r.OrchestratorStatus).Word) + ") is quit with ctrl+c and a new one starts in the same tab. Its conversation is lost; the new one resumes from the run's files.",
	}
	if r.OrchestratorStatus == "working" || r.OrchestratorStatus == "blocked" {
		lines = append(lines, "It is "+r.OrchestratorStatus+" now: what it is doing is lost.")
	}
	return lines
}

// relaunchView draws the question over base.
func (m *Model) relaunchView(base string, width, height int) string {
	return screen.Confirm(&m.zones, base, "Relaunch the "+strings.ToLower(m.run.Orchestrator.Label)+"?", m.relaunchLines(), width, height)
}
