package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/screen"
)

// When the board cannot tell which agent is meant -- a ticket with no
// orchestrator open, a space running several agents -- it asks rather than
// guesses: the chooser lists the agents there, with the last lines of the
// highlighted one's pane beside it, read live, so you can see which is which.
// The choice is used for that one message or broadcast: nothing is renamed or
// remembered. An agent waiting at a prompt is listed but cannot be chosen,
// since what is sent would land in its dialog.

// pickRow is one agent the chooser offers.
type pickRow struct{ pane, tab, agent, status, title string }

// label is how the agent is named once chosen: its tab, or its pane.
func (r pickRow) label() string {
	if r.tab != "" {
		return "tab " + r.tab
	}
	return r.pane
}

type pickRowsMsg struct {
	rows []pickRow
	err  error
}

type previewMsg struct {
	pane, text string
	err        error
}

// previewRead is how many of a pane's lines the preview reads.
const previewRead = 60

// chooseAgent opens the chooser over what is open now, listing the agents in
// the workspaces ws; use is what choosing one does.
func (m *Model) chooseAgent(title string, ws []string, use func(pickRow) (tea.Model, tea.Cmd)) tea.Cmd {
	m.openChooser(chooserState{
		ch:   screen.Chooser{Title: title, Note: "finding the agents…"},
		live: true,
		use:  func(i int) (tea.Model, tea.Cmd) { return use(m.ap.rows[i]) },
		show: m.readPreview,
	})
	client := m.client
	ids := append([]string(nil), ws...)
	return func() tea.Msg {
		if client == nil {
			return pickRowsMsg{}
		}
		rows, err := agentRows(client, ids)
		return pickRowsMsg{rows: rows, err: err}
	}
}

// agentRows are the agents in the workspaces ws, named by their tabs. Panes
// with no agent -- shells, plugin panes, this board -- are left out.
func agentRows(client *herdr.Client, ws []string) ([]pickRow, error) {
	agents, err := client.Agents()
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	tabs := map[string]string{}
	for _, w := range ws {
		wanted[w] = true
		if list, err := client.Tabs(w); err == nil {
			for _, t := range list {
				tabs[t.ID] = t.Label
			}
		}
	}
	var rows []pickRow
	for _, a := range agents {
		if a.Agent == nil || *a.Agent == "" || !wanted[a.WorkspaceID] {
			continue
		}
		rows = append(rows, pickRow{pane: a.PaneID, tab: tabs[a.TabID], agent: *a.Agent, status: a.AgentStatus, title: a.Title})
	}
	return rows, nil
}

// applyPickRows fills the agent chooser and starts reading the highlighted
// pane.
func (m *Model) applyPickRows(msg pickRowsMsg) tea.Cmd {
	if m.mode != modeChooser {
		return nil
	}
	m.ap.rows = msg.rows
	m.ap.ch.Rows, m.ap.ch.Note, m.ap.ch.Sel = nil, "", -1
	for i, r := range msg.rows {
		row := screen.ChooserRow{Text: m.pickRowText(r)}
		if r.status == "blocked" {
			row.Off, row.Why = true, "waiting at a prompt"
		} else if m.ap.ch.Sel < 0 {
			m.ap.ch.Sel = i
		}
		m.ap.ch.Rows = append(m.ap.ch.Rows, row)
	}
	m.ap.ch.Sel = max(m.ap.ch.Sel, 0)
	switch {
	case msg.err != nil:
		m.ap.ch.Note = msg.err.Error()
	case len(msg.rows) == 0:
		m.ap.ch.Note = "no agent open here"
		if k := m.hintKey("orchestrator"); k != "" {
			m.ap.ch.Note += " · " + screen.KeyName(k) + " on the card opens an orchestrator"
		}
	}
	return m.showSelected()
}

// pickRowText is an agent as the chooser lists it: its status glyph in its
// colour, its tab, the program and what it says it is doing.
func (m *Model) pickRowText(r pickRow) string {
	al := look.Agent(r.status)
	name := lipgloss.NewStyle().Bold(true).Render(r.label())
	text := name + " " + dimStyle.Render(r.agent)
	if r.status != "blocked" { // a blocked one says why instead
		text = al.Style.Render(orWord(m.icons, al.Glyph, al.Word)) + " " + text
	}
	if r.title != "" {
		text += dimStyle.Render(" · " + r.title)
	}
	return text
}

// orWord is the glyph with glyphs on, the word without.
func orWord(icons bool, glyph, word string) string {
	if icons && glyph != "" {
		return glyph
	}
	if word == "" {
		return "?"
	}
	return word
}

// readPreview reads the pane of agent i.
func (m *Model) readPreview(i int) tea.Cmd {
	if i < 0 || i >= len(m.ap.rows) || m.client == nil {
		return nil
	}
	client, pane := m.client, m.ap.rows[i].pane
	return func() tea.Msg {
		text, err := client.ReadPane(pane, previewRead)
		return previewMsg{pane: pane, text: text, err: err}
	}
}

func (m *Model) applyPreview(msg previewMsg) {
	i := m.ap.ch.Sel
	if m.mode != modeChooser || i < 0 || i >= len(m.ap.rows) || m.ap.rows[i].pane != msg.pane {
		return // the highlight moved on
	}
	if msg.err != nil {
		m.ap.ch.Preview = []string{look.Err.Render("could not read the pane: " + msg.err.Error())}
		return
	}
	m.ap.ch.Preview = screen.SafeLines(msg.text)
}
