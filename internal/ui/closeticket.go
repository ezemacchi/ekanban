package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/closing"
	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/state"
	"github.com/ezemacchi/ekanban/internal/store"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Closing a finished ticket from the board: the same checks and steps as
// `ekanban close`, in the closing package. The board asks first, lists what
// the checks found, and runs the steps off the UI thread.

type closeSettings struct {
	acceptFrom []string // the columns a card may be accepted from
	check      []string // [close] check, from config.toml
}

// SetClose takes [accept] from and [close] check. It returns a problem for
// each [accept] from column the board does not have; those are left out.
func (m *Model) SetClose(acceptFrom, check []string) []string {
	m.cl = closeSettings{check: check}
	var problems []string
	for _, id := range acceptFrom {
		if _, ok := m.columns.Find(id); !ok {
			problems = append(problems, fmt.Sprintf("[accept] from %q is not a column — left out", id))
			continue
		}
		m.cl.acceptFrom = append(m.cl.acceptFrom, id)
	}
	return problems
}

// acceptable says whether a card in column id may be accepted: one of
// [accept] from, or the last column when that names none.
func (m *Model) acceptable(id string) bool {
	if len(m.cl.acceptFrom) == 0 {
		return id == pipeline.ReadyQA
	}
	for _, from := range m.cl.acceptFrom {
		if from == id {
			return true
		}
	}
	return false
}

// acceptColumns names the columns a card may be accepted from.
func (m *Model) acceptColumns() string {
	ids := m.cl.acceptFrom
	if len(ids) == 0 {
		ids = []string{pipeline.ReadyQA}
	}
	labels := make([]string, len(ids))
	for i, id := range ids {
		labels[i] = m.columns.Label(id)
	}
	return strings.Join(labels, " or ")
}

type closeCheckedMsg struct {
	plan  closing.Plan
	steps []closing.Step
	err   error
}

type closedMsg struct {
	key   string
	steps []closing.Step
	rec   *store.Accepted
	err   error
}

// closeSelected checks whether the selected ticket can be closed, off the UI
// thread; applyCloseChecked asks before anything changes.
func (m *Model) closeSelected() tea.Cmd {
	sp := m.selected()
	if sp == nil || !m.requireWorktree() {
		return nil
	}
	if m.pipeInfo[sp.Key].Key == "" {
		m.status = m.spaceName(sp) + " has no ticket run"
		return nil
	}
	start := m.working("checking whether " + m.spaceName(sp) + " can be closed…")
	src, client, worktree, stateDir, check := m.pipe, m.client, sp.Key, m.stateDir, m.cl.check
	pr := 0
	if e, ok := m.board.Entries[store.Key(worktree)]; ok {
		pr = e.PR
	}
	return tea.Batch(start, func() tea.Msg {
		live, err := ticket.ReadLive(client)
		if err != nil {
			return closeCheckedMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		env := closing.Real(client, stateDir, gh.HideWindow, nil)
		t, ok := state.Of(ctx, src, live, worktree, pr, state.Git(env.Git))
		if !ok {
			return closeCheckedMsg{err: fmt.Errorf("no run found in %s", worktree)}
		}
		plan := closing.Plan{
			Ticket:     t,
			Repo:       src.Repo(),
			Workspaces: closing.WorkspacesOn(live, worktree),
			Self:       os.Getenv("HERDR_WORKSPACE_ID"),
			Check:      check,
		}
		steps, err := closing.Checks(env, plan)
		return closeCheckedMsg{plan: plan, steps: steps, err: err}
	})
}

func (m *Model) applyCloseChecked(msg closeCheckedMsg) {
	if msg.err != nil {
		m.status = "not closed: " + msg.err.Error()
		return
	}
	m.status = ""
	plan := msg.plan
	lines := []string{}
	for _, s := range msg.steps {
		lines = append(lines, "✓ "+s.Name+": "+s.Note)
	}
	lines = append(lines, "", "Then it is accepted into the Archive (A), with a copy of the run's notes,")
	if len(plan.Workspaces) > 0 {
		lines = append(lines, "its workspace "+strings.Join(plan.Workspaces, ", ")+" is closed,")
	}
	lines = append(lines, "and git removes "+plan.Ticket.Worktree+".")
	m.confirm("Close "+plan.Ticket.Key+"?", lines, func() (tea.Model, tea.Cmd) {
		return m, m.runClose(plan)
	})
}

// runClose runs the steps; closing.Close checks again first, since the
// answer may have come minutes later.
func (m *Model) runClose(plan closing.Plan) tea.Cmd {
	start := m.working("closing " + plan.Ticket.Key + "…")
	client, stateDir := m.client, m.stateDir
	return tea.Batch(start, func() tea.Msg {
		msg := closedMsg{key: plan.Ticket.Key}
		env := closing.Real(client, stateDir, gh.HideWindow, func(_ string, a store.Accepted) {
			msg.rec = &a
		})
		msg.err = closing.Close(env, plan, func(s closing.Step) { msg.steps = append(msg.steps, s) })
		return msg
	})
}

// applyClosed keeps the board's copy of the file in step with what the close
// wrote, and says how far it got.
func (m *Model) applyClosed(msg closedMsg) tea.Cmd {
	if msg.rec != nil {
		m.board.Accept(msg.rec.Worktree, *msg.rec)
		m.save()
	}
	switch {
	case msg.err != nil && msg.rec != nil:
		m.status = msg.key + " is in the Archive, but " + msg.err.Error()
	case msg.err != nil:
		m.status = "not closed: " + msg.err.Error()
	default:
		m.status = msg.key + " closed: it is in the Archive (A)"
	}
	m.rebuild()
	return tea.Batch(m.refresh(), m.loadPipeline(false))
}
