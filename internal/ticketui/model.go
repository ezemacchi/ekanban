// Package ticketui is the per-ticket board: one Big Team or Full Team run
// as a kanban of its roles, meant to be the first tab of the run's workspace.
package ticketui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/nav"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

const refreshEvery = 5 * time.Second

var (
	titleStyle  = look.Title
	dimStyle    = look.Dim
	cursorStyle = look.Cursor
	errStyle    = look.Err
	keyStyle    = look.Key
	headStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	waitStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	doneStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
)

// Model is the bubbletea model for one ticket.
type Model struct {
	client   *herdr.Client
	worktree string
	opts     ticket.Options
	icons    look.Icons

	run    *ticket.Run
	err    error
	status string

	col, row int
	chord    string
	width    int
	height   int
	spinner  look.Spinner
}

// New builds the board for the run in worktree. opts says where prototypes
// are and which teams exist; icons turns on Nerd Font glyphs. problems are
// configuration complaints, shown until the first key.
func New(client *herdr.Client, worktree string, opts ticket.Options, icons bool, problems []string) *Model {
	return &Model{client: client, worktree: worktree, opts: opts, icons: look.Icons{On: icons},
		spinner: look.NewSpinner(), status: strings.Join(problems, " · ")}
}

type loadedMsg struct {
	run *ticket.Run
	err error
}
type tickMsg struct{}

func (m *Model) load() tea.Msg {
	agents, err := m.client.Agents()
	if err != nil {
		return loadedMsg{err: err}
	}
	labels := map[string]string{}
	seen := map[string]bool{}
	for _, a := range agents {
		if seen[a.WorkspaceID] {
			continue
		}
		seen[a.WorkspaceID] = true
		if tabs, err := m.client.Tabs(a.WorkspaceID); err == nil {
			for _, t := range tabs {
				labels[t.ID] = t.Label
			}
		}
	}
	run, err := ticket.Load(m.worktree, m.opts, agents, labels)
	return loadedMsg{run: run, err: err}
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

// Init loads the run and starts the refresh clock.
func (m *Model) Init() tea.Cmd { return tea.Batch(m.load, tick(), m.spinner.Start()) }

// Update handles keys, refreshes and resizes.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loadedMsg:
		m.run, m.err = msg.run, msg.err
		m.clamp()
	case tickMsg:
		return m, tea.Batch(m.load, tick())
	case look.SpinMsg:
		return m, m.spinner.Update(msg, m.run == nil && m.err == nil)
	case tea.KeyMsg:
		return m, m.key(msg.String())
	}
	return m, nil
}

func (m *Model) key(k string) tea.Cmd {
	if m.chord == "g" {
		m.chord = ""
		if k == "g" {
			m.row = 0
		}
		return nil
	}
	m.status = ""
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "h", "left":
		m.stepColumn(-1)
	case "l", "right":
		m.stepColumn(1)
	case "j", "down":
		m.row++
	case "k", "up":
		m.row--
	case "g":
		m.chord = "g"
	case "G":
		m.row = 1 << 30
	case "r":
		m.status = "actualizando…"
		return m.load
	case "o":
		if m.run != nil {
			url := m.run.JiraURL
			m.status = "opening " + url
			return func() tea.Msg { _ = look.OpenURL(url); return nil }
		}
	case "p":
		if m.run == nil {
			return nil
		}
		if m.run.Prototype == "" {
			m.status = "no prototype found for " + orDash(m.run.Spec)
			return nil
		}
		page := m.run.Prototype
		m.status = "opening the prototype"
		return func() tea.Msg { _ = look.OpenURL(page); return nil }
	case "enter":
		return m.focusSelected()
	}
	m.clamp()
	return nil
}

func (m *Model) cardsIn(col ticket.Column) []ticket.Card {
	if m.run == nil {
		return nil
	}
	var out []ticket.Card
	for _, c := range m.run.Cards {
		if c.Column == col {
			out = append(out, c)
		}
	}
	return out
}

func (m *Model) cardCount(col int) int { return len(m.cardsIn(ticket.Columns[col].Col)) }

func (m *Model) stepColumn(dir int) {
	m.col = nav.Step(m.col, dir, len(ticket.Columns), m.cardCount)
	m.row = 0
}

func (m *Model) clamp() {
	// A refresh can empty the column under the cursor.
	if col := nav.Settle(m.col, len(ticket.Columns), m.cardCount); col != m.col {
		m.col, m.row = col, 0
	}
	n := m.cardCount(m.col)
	if m.row >= n {
		m.row = n - 1
	}
	if m.row < 0 {
		m.row = 0
	}
}

func (m *Model) focusSelected() tea.Cmd {
	cards := m.cardsIn(ticket.Columns[m.col].Col)
	if m.row >= len(cards) {
		return nil
	}
	c := cards[m.row]
	if c.PaneID == "" {
		m.status = c.Role.Label + " has no agent open"
		return nil
	}
	m.status = "going to " + c.Role.Label
	pane := c.PaneID
	return func() tea.Msg { _ = m.client.FocusAgent(pane); return nil }
}

// View renders the header, the role columns, and what the run is waiting on.
func (m *Model) View() string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	if m.err != nil && m.run == nil {
		return errStyle.Render("No run found in "+m.worktree+": "+m.err.Error()) + "\n" + dimStyle.Render("Looking for .runs/<KEY>/STATE.md · q quit")
	}
	if m.run == nil {
		return dimStyle.Render(m.spinner.Frame() + " loading")
	}
	r := m.run
	ic := m.icons
	var b strings.Builder

	team := r.Team
	if team == "" {
		team = "unknown team"
	}
	fmt.Fprintf(&b, "%s  %s\n", titleStyle.Render(r.Key), dimStyle.Render(fmt.Sprintf("%s · %s · %s",
		team, ic.With(look.Target, orDash(r.Target)), ic.With(look.Branch, orDash(r.Branch)))))
	fmt.Fprintf(&b, "%s\n", dimStyle.Render(ic.With(look.Jira, r.JiraURL)))
	switch {
	case r.Prototype != "":
		fmt.Fprintf(&b, "%s\n", dimStyle.Render(ic.With(look.Brush, "Prototype "+r.Spec+": "+filepath.Base(r.Prototype)+"  (p)")))
	case r.Spec != "":
		fmt.Fprintf(&b, "%s\n", dimStyle.Render(ic.With(look.Brush, "No prototype for "+r.Spec)))
	}
	for _, l := range r.Objective {
		fmt.Fprintf(&b, "%s\n", look.Truncate(l, width))
	}
	orch := "no orchestrator open"
	if r.OrchestratorStatus != "" {
		orch = "orchestrator: " + statusWord(r.OrchestratorStatus)
	}
	fmt.Fprintf(&b, "%s\n\n", dimStyle.Render(ic.With(look.Sitemap, orch)))

	colWidth := (width - 3) / len(ticket.Columns)
	if colWidth < 14 {
		colWidth = 14
	}
	cols := make([]string, len(ticket.Columns))
	for i, c := range ticket.Columns {
		var cb strings.Builder
		cards := m.cardsIn(c.Col)
		cb.WriteString(headStyle.Render(ic.With(columnGlyph[c.Col], fmt.Sprintf("%s (%d)", c.Label, len(cards)))) + "\n")
		cardWidth := colWidth - 1
		text := look.CardInner(cardWidth)
		for j, card := range cards {
			selected := i == m.col && j == m.row
			style := lipgloss.NewStyle()
			switch {
			case selected:
				style = cursorStyle
			case c.Col == ticket.Waiting:
				style = waitStyle
			case c.Col == ticket.Done:
				style = doneStyle
			}
			lines := []string{style.Render(look.Truncate(ic.With(card.Role.Icon, card.Role.Label), text))}
			if card.Note != "" {
				note, noteStyle := card.Note, dimStyle
				if card.Stuck {
					note, noteStyle = ic.With(look.Warning, note), waitStyle
				}
				lines = append(lines, noteStyle.Render(look.Truncate(note, text)))
			}
			border := look.CardBorder
			if selected {
				border = look.CardSelected
			}
			cb.WriteString(strings.Join(look.Card(lines, cardWidth, border), "\n") + "\n")
		}
		cols[i] = lipgloss.NewStyle().Width(colWidth).Render(cb.String())
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cols...))
	b.WriteString("\n")

	if len(r.CurrentStep) > 0 {
		b.WriteString(headStyle.Render(ic.With(look.Play, "Now")) + "\n")
		for _, l := range r.CurrentStep {
			b.WriteString(look.Truncate(l, width) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(headStyle.Render(ic.With(look.Question, fmt.Sprintf("Open questions (%d)", len(r.Questions)))) + "\n")
	if len(r.Questions) == 0 {
		b.WriteString(dimStyle.Render("none in STATE.md") + "\n")
	}
	limit := 8
	for i, q := range r.Questions {
		if i == limit {
			b.WriteString(dimStyle.Render(fmt.Sprintf("and %d more in STATE.md", len(r.Questions)-limit)) + "\n")
			break
		}
		b.WriteString("- " + look.Truncate(q, width-2) + "\n")
	}
	if r.Landed {
		b.WriteString("\n" + doneStyle.Render(ic.With(look.Rocket, "Landed: the pull request is ready for review")) + "\n")
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(dimStyle.Render(m.status) + "\n")
	}
	if m.err != nil {
		b.WriteString(errStyle.Render(m.err.Error()) + "\n")
	}
	b.WriteString(keyStyle.Render("h/l") + " column  " + keyStyle.Render("j/k") + " role  " + keyStyle.Render("enter") + " go to its tab  " +
		keyStyle.Render("o") + " Jira  " + keyStyle.Render("p") + " prototype  " + keyStyle.Render("r") + " refresh  " + keyStyle.Render("q") + " quit")
	return b.String()
}

func statusWord(s string) string {
	switch s {
	case "working":
		return "working"
	case "blocked":
		return "waiting for your answer"
	case "idle", "done":
		return "idle"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
