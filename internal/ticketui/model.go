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

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/keys"
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
	cols     columns.Set
	keys     *keys.Map

	run    *ticket.Run
	err    error
	status string

	col, row int
	chord    string
	width    int
	height   int
	spinner  look.Spinner
}

// Actions are the ticket board's keys. The first key of each is what key()
// switches on; config.toml's [keys] binds others by name.
var Actions = []keys.Action{
	{Name: "left", Keys: []string{"h", "left"}, Help: "previous column"},
	{Name: "right", Keys: []string{"l", "right"}, Help: "next column"},
	{Name: "down", Keys: []string{"j", "down"}, Help: "next role"},
	{Name: "up", Keys: []string{"k", "up"}, Help: "previous role"},
	{Name: "top", Keys: []string{"gg"}, Help: "first role", Fixed: true},
	{Name: "bottom", Keys: []string{"G"}, Help: "last role"},
	{Name: "jump", Keys: []string{"enter"}, Help: "go to the role's tab"},
	{Name: "open-issue", Keys: []string{"o"}, Help: "open the ticket in the tracker"},
	{Name: "prototype", Keys: []string{"p"}, Help: "open the prototype"},
	{Name: "refresh", Keys: []string{"r"}, Help: "refresh"},
	{Name: "quit", Keys: []string{"q"}, Help: "quit"},
}

// Settings is what config.toml says about the ticket board.
type Settings struct {
	Options  ticket.Options      // where prototypes are, which teams exist
	Columns  columns.Set         // nil: ticket.DefaultColumns
	Keys     map[string][]string // [keys]: action name -> keys
	Icons    bool                // Nerd Font glyphs
	Problems []string            // configuration complaints, shown until the first key
}

// New builds the board for the run in worktree.
func New(client *herdr.Client, worktree string, s Settings) *Model {
	km, problems := keys.New(Actions, s.Keys)
	return &Model{client: client, worktree: worktree, opts: s.Options, cols: s.Columns, keys: km,
		icons: look.Icons{On: s.Icons}, spinner: look.NewSpinner(), status: strings.Join(append(s.Problems, problems...), " · ")}
}

func (m *Model) keyMap() *keys.Map {
	if m.keys == nil {
		m.keys, _ = keys.New(Actions, nil)
	}
	return m.keys
}

func (m *Model) columns() columns.Set {
	if len(m.cols) == 0 {
		return ticket.DefaultColumns
	}
	return m.cols
}

// columnAt is the run column shown in display position i.
func (m *Model) columnAt(i int) ticket.Column {
	c, _ := ticket.ColumnByID(m.columns()[i].ID)
	return c
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
	if k == "ctrl+c" {
		return tea.Quit
	}
	if k = m.keyMap().Resolve(k); k == "" {
		return nil
	}
	switch k {
	case "q":
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
		m.status = "refreshing…"
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

func (m *Model) cardCount(col int) int { return len(m.cardsIn(m.columnAt(col))) }

func (m *Model) stepColumn(dir int) {
	m.col = nav.Step(m.col, dir, len(m.columns()), m.cardCount)
	m.row = 0
}

func (m *Model) clamp() {
	// A refresh can empty the column under the cursor.
	if col := nav.Settle(m.col, len(m.columns()), m.cardCount); col != m.col {
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
	cards := m.cardsIn(m.columnAt(m.col))
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

	shown := m.columns()
	colWidth := (width - 3) / len(shown)
	if colWidth < 14 {
		colWidth = 14
	}
	cols := make([]string, len(shown))
	for i, def := range shown {
		var cb strings.Builder
		col := m.columnAt(i)
		cards := m.cardsIn(col)
		head := headStyle
		if def.Color != "" {
			head = head.Foreground(lipgloss.Color(def.Color))
		}
		cb.WriteString(head.Render(ic.With(def.Icon, fmt.Sprintf("%s (%d)", def.Label, len(cards)))) + "\n")
		cardWidth := colWidth - 1
		text := look.CardInner(cardWidth)
		for j, card := range cards {
			selected := i == m.col && j == m.row
			style := lipgloss.NewStyle()
			switch {
			case selected:
				style = cursorStyle
			case col == ticket.Waiting:
				style = waitStyle
			case col == ticket.Done:
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
	k := func(name string) string { return keyStyle.Render(m.keyMap().Key(name)) }
	b.WriteString(k("left") + "/" + k("right") + " column  " + k("up") + "/" + k("down") + " role  " + k("jump") + " go to its tab  " +
		k("open-issue") + " tracker  " + k("prototype") + " prototype  " + k("refresh") + " refresh  " + k("quit") + " quit")
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
