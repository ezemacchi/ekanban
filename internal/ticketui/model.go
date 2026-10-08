// Package ticketui is the per-ticket board: one Big Team or Full Team run
// as a kanban of its roles, meant to be the first tab of the run's workspace.
package ticketui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/phin-tech/herdr-phin-board/internal/herdr"
	"github.com/phin-tech/herdr-phin-board/internal/ticket"
)

const refreshEvery = 5 * time.Second

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	waitStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	doneStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	keyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
)

// Model is the bubbletea model for one ticket.
type Model struct {
	client   *herdr.Client
	worktree string
	icons    glyphs

	run    *ticket.Run
	err    error
	status string

	col, row int
	chord    string
	width    int
	height   int
}

// New builds the board for the run in worktree. icons turns on Nerd Font
// glyphs.
func New(client *herdr.Client, worktree string, icons bool) *Model {
	return &Model{client: client, worktree: worktree, icons: glyphs{on: icons}}
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
	run, err := ticket.Load(m.worktree, agents, labels)
	return loadedMsg{run: run, err: err}
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

// Init loads the run and starts the refresh clock.
func (m *Model) Init() tea.Cmd { return tea.Batch(m.load, tick()) }

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
		m.col--
	case "l", "right":
		m.col++
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
			m.status = "abriendo " + url
			return func() tea.Msg { _ = openURL(url); return nil }
		}
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

func (m *Model) clamp() {
	if m.col < 0 {
		m.col = 0
	}
	if m.col >= len(ticket.Columns) {
		m.col = len(ticket.Columns) - 1
	}
	n := len(m.cardsIn(ticket.Columns[m.col].Col))
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
		m.status = c.Role.Label + " no tiene un agente abierto"
		return nil
	}
	m.status = "yendo a " + c.Role.Label
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
		return errStyle.Render("No encuentro una corrida en "+m.worktree+": "+m.err.Error()) + "\n" + dimStyle.Render("Busco .runs/<KEY>/STATE.md · q salir")
	}
	if m.run == nil {
		return dimStyle.Render("cargando…")
	}
	r := m.run
	ic := m.icons
	var b strings.Builder

	team := r.Team
	if team == "" {
		team = "equipo desconocido"
	}
	fmt.Fprintf(&b, "%s  %s\n", titleStyle.Render(r.Key), dimStyle.Render(fmt.Sprintf("%s · %s · %s",
		team, ic.g(glyphTarget, orDash(r.Target)), ic.g(glyphBranch, orDash(r.Branch)))))
	fmt.Fprintf(&b, "%s\n", dimStyle.Render(ic.g(glyphJira, r.JiraURL)))
	for _, l := range r.Objective {
		fmt.Fprintf(&b, "%s\n", truncate(l, width))
	}
	orch := "sin orquestador abierto"
	if r.OrchestratorStatus != "" {
		orch = "orquestador: " + statusWord(r.OrchestratorStatus)
	}
	fmt.Fprintf(&b, "%s\n\n", dimStyle.Render(ic.g(glyphOrch, orch)))

	colWidth := (width - 3) / len(ticket.Columns)
	if colWidth < 14 {
		colWidth = 14
	}
	cols := make([]string, len(ticket.Columns))
	for i, c := range ticket.Columns {
		var cb strings.Builder
		cards := m.cardsIn(c.Col)
		cb.WriteString(headStyle.Render(ic.g(columnGlyph[c.Col], fmt.Sprintf("%s (%d)", c.Label, len(cards)))) + "\n")
		for j, card := range cards {
			line := ic.g(roleGlyph[card.Role.ID], card.Role.Label)
			style := lipgloss.NewStyle()
			switch c.Col {
			case ticket.Waiting:
				style = waitStyle
			case ticket.Done:
				style = doneStyle
			}
			prefix := "  "
			if i == m.col && j == m.row {
				prefix = cursorStyle.Render("❯ ")
				style = cursorStyle
			}
			cb.WriteString(prefix + style.Render(truncate(line, colWidth-2)) + "\n")
			if card.Note != "" {
				note, style := card.Note, dimStyle
				if card.Stuck {
					note, style = ic.g(glyphStuck, note), waitStyle
				}
				cb.WriteString("    " + style.Render(truncate(note, colWidth-4)) + "\n")
			}
		}
		cols[i] = lipgloss.NewStyle().Width(colWidth).Render(cb.String())
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cols...))
	b.WriteString("\n")

	if len(r.CurrentStep) > 0 {
		b.WriteString(headStyle.Render(ic.g(glyphNow, "Ahora")) + "\n")
		for _, l := range r.CurrentStep {
			b.WriteString(truncate(l, width) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(headStyle.Render(ic.g(glyphQuestion, fmt.Sprintf("Preguntas abiertas (%d)", len(r.Questions)))) + "\n")
	if len(r.Questions) == 0 {
		b.WriteString(dimStyle.Render("ninguna en STATE.md") + "\n")
	}
	limit := 8
	for i, q := range r.Questions {
		if i == limit {
			b.WriteString(dimStyle.Render(fmt.Sprintf("y %d más en STATE.md", len(r.Questions)-limit)) + "\n")
			break
		}
		b.WriteString("- " + truncate(q, width-2) + "\n")
	}
	if r.Landed {
		b.WriteString("\n" + doneStyle.Render(ic.g(glyphLanded, "Landed: el pull request está listo para que lo revisen")) + "\n")
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(dimStyle.Render(m.status) + "\n")
	}
	if m.err != nil {
		b.WriteString(errStyle.Render(m.err.Error()) + "\n")
	}
	b.WriteString(keyStyle.Render("h/l") + " columna  " + keyStyle.Render("j/k") + " rol  " + keyStyle.Render("enter") + " ir a su pestaña  " +
		keyStyle.Render("o") + " Jira  " + keyStyle.Render("r") + " refrescar  " + keyStyle.Render("q") + " salir")
	return b.String()
}

func statusWord(s string) string {
	switch s {
	case "working":
		return "trabajando"
	case "blocked":
		return "esperando tu respuesta"
	case "idle", "done":
		return "quieto"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func truncate(s string, w int) string {
	if w <= 1 || lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

var openURL = func(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}
