package ui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// `m` types a message into the agent running in the selected space -- a
// ticket's orchestrator, on the board of tickets -- without submitting it. The board knows which space is which and what you are waiting
// on; this closes the loop from noticing to saying something.

var (
	errNoAgentHere  = errors.New("no agent in that space")
	errSeveralHere  = errors.New("several agents in that space — focus the one you mean")
	errAgentUnknown = errors.New("that space is not open, so it has no agent")
)

// pickAgent finds the single agent to send to for a space.
//
// Panes with no agent field are shells and plugin sidebars, and this board is
// itself one of them, so they are excluded. Ambiguity is refused rather than
// guessed: sending a review comment to the wrong agent is worse than not
// sending it.
func pickAgent(agents []herdr.Agent, workspaceIDs []string) (herdr.Agent, error) {
	if len(workspaceIDs) == 0 {
		return herdr.Agent{}, errAgentUnknown
	}
	wanted := map[string]bool{}
	for _, id := range workspaceIDs {
		wanted[id] = true
	}

	var found []herdr.Agent
	for _, a := range agents {
		if a.Agent == nil || *a.Agent == "" {
			continue
		}
		if wanted[a.WorkspaceID] {
			found = append(found, a)
		}
	}

	switch len(found) {
	case 0:
		return herdr.Agent{}, errNoAgentHere
	case 1:
		if found[0].AgentStatus == "blocked" {
			return herdr.Agent{}, errAgentBlocked
		}
		return found[0], nil
	default:
		return herdr.Agent{}, errSeveralHere
	}
}

// errAgentBlocked refuses an agent waiting at a dialog: what is typed would
// land in the dialog, not in its input.
var errAgentBlocked = errors.New("its agent is waiting at a prompt: answer it there first")

// pickOrchestrator is the agent `m` types into for a ticket: its run's
// orchestrator, found as the broadcast finds it, so a workspace with role
// agents open beside it still has one clear target.
func pickOrchestrator(run *ticket.Run) (herdr.Agent, error) {
	switch {
	case run.OrchestratorPane == "":
		return herdr.Agent{}, errNoOrchestrator
	case run.OrchestratorStatus == "blocked":
		return herdr.Agent{}, errOrchestratorBlocked
	}
	return herdr.Agent{PaneID: run.OrchestratorPane, AgentStatus: run.OrchestratorStatus}, nil
}

var (
	errNoOrchestrator      = errors.New("no orchestrator open")
	errOrchestratorBlocked = errors.New("its orchestrator is waiting at a prompt: answer it there first")
)

type agentSentMsg struct {
	label string
	pane  string
}

// sendToAgent types the message into the space's agent and goes there, so the
// reviewer can add a line and submit it themselves.
func (m *Model) sendToAgent(text string) tea.Cmd {
	sp := m.selected()
	if sp == nil || text == "" {
		return nil
	}
	if m.client == nil {
		return nil
	}

	client := m.client
	ids := append([]string(nil), sp.WorkspaceIDs...)
	label := sp.Label
	// A ticket's card goes to its orchestrator; any other space to its one
	// agent.
	src, worktree := m.pipe, ""
	if m.pipelineOn() && m.pipeInfo[sp.Key].Key != "" {
		worktree, label = sp.Key, m.pipeInfo[sp.Key].Key
	}

	return func() tea.Msg {
		var agent herdr.Agent
		var err error
		if worktree != "" {
			var live ticket.Live
			var run *ticket.Run
			if live, err = ticket.ReadLive(client); err != nil {
				return errMsg{err}
			}
			if run, err = src.Run(worktree, live); err != nil {
				return errMsg{err}
			}
			agent, err = pickOrchestrator(run)
		} else {
			var agents []herdr.Agent
			if agents, err = client.Agents(); err != nil {
				return errMsg{err}
			}
			agent, err = pickAgent(agents, ids)
		}
		switch {
		case errors.Is(err, errNoOrchestrator), errors.Is(err, errSeveralHere):
			// Not a guess: ask which, with the message kept.
			return needAgentMsg{ws: ids, text: text, label: label, why: err.Error()}
		case err != nil:
			return statusMsg(fmt.Sprintf("%s: %s", label, err))
		}
		return typeInto(client, agent.PaneID, text, label)()
	}
}

// needAgentMsg is `m` not knowing which agent is meant: the chooser asks,
// the message kept.
type needAgentMsg struct {
	ws               []string
	text, label, why string
}

// askWhichAgent opens the chooser for a message; the agent chosen gets it
// typed in, unsent, once.
func (m *Model) askWhichAgent(msg needAgentMsg) tea.Cmd {
	title := msg.label + ": " + msg.why + " · which agent gets it?"
	client := m.client
	return m.chooseAgent(title, msg.ws, func(r pickRow) (tea.Model, tea.Cmd) {
		return m, typeInto(client, r.pane, msg.text, msg.label)
	})
}

// typeInto types text into pane without submitting it, and then goes there.
func typeInto(client *herdr.Client, pane, text, label string) tea.Cmd {
	return func() tea.Msg {
		if err := client.SendToAgent(pane, text); err != nil {
			return errMsg{err}
		}
		return agentSentMsg{label: label, pane: pane}
	}
}

// focusAgentAndQuit hands over to the agent pane: the text is waiting in its
// input, unsent, for the user to finish and submit. A board in its own tab
// stays open.
func (m *Model) focusAgentAndQuit(pane string) tea.Cmd {
	client := m.client
	focus := func() tea.Msg {
		_ = client.FocusAgent(pane)
		return nil
	}
	if m.stays {
		return focus
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.quitting = true

	return tea.Sequence(focus, tea.Quit)
}
