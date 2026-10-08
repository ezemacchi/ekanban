// Package ticketui is the per-ticket board: one Big Team or Full Team run
// as a kanban of its roles, meant to be the first tab of the run's workspace.
package ticketui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/lead"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/nav"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Herdr events (an agent appearing, changing status, closing) reload the board
// at once. The run's files raise no event, so every checkEvery the run folder
// is looked at and reread when it changed, and everything is reread every
// fullEvery regardless.
const (
	checkEvery = 3 * time.Second
	fullEvery  = 30 * time.Second
	settle     = 250 * time.Millisecond // events come in bursts
)

var (
	titleStyle  = look.Title
	dimStyle    = look.Dim
	cursorStyle = look.Cursor
	errStyle    = look.Err
	keyStyle    = look.Key
	headStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	waitStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	doneStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
	// linkStyle marks a line a click opens.
	linkStyle = look.Dim.Underline(true)
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

	loading  bool      // a load is running
	again    bool      // something changed during it: load once more
	settling bool      // an event is waiting out settle
	loadedAt time.Time // the last load finished
	stamp    string    // the run folder as last read, see runStamp
	opening  bool      // the lead is being opened

	events    chan herdr.Event
	subCancel context.CancelFunc
	subPanes  string // the agent panes the subscription covers

	seen     map[string]string // agent statuses at the last load, by pane
	activity []string          // the latest status changes, newest first

	busyText string // the status naming work still running; see working
	zones    []zone // what the last frame drew that a click can act on
}

// zone is a clickable part of the last frame: a role card, or a line or
// button that stands for a key.
type zone struct {
	y, x0, x1 int // x1 exclusive
	col, row  int // a card; col < 0 for a key
	key       string
}

func (m *Model) zoneAt(x, y int) (zone, bool) {
	for i := len(m.zones) - 1; i >= 0; i-- {
		z := m.zones[i]
		if y == z.y && x >= z.x0 && x < z.x1 {
			return z, true
		}
	}
	return zone{}, false
}

// keyZone records a line or button that stands for the key bound to action.
func (m *Model) keyZone(y, x0, x1 int, action string) {
	m.zones = append(m.zones, zone{y: y, x0: x0, x1: x1, col: -1, key: m.keyMap().Key(action)})
}

// click selects the card under the pointer; a second click on it goes to its
// tab, like enter. A click on a key's line or button presses that key.
func (m *Model) click(x, y int) tea.Cmd {
	z, ok := m.zoneAt(x, y)
	if !ok {
		return nil
	}
	if z.col < 0 {
		return m.key(z.key)
	}
	already := m.col == z.col && m.row == z.row
	m.col, m.row = z.col, z.row
	m.status = ""
	if already {
		return m.focusSelected()
	}
	return nil
}

// activityKept is how many status changes the Activity list shows.
const activityKept = 5

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
	{Name: "lead", Keys: []string{"o"}, Help: "go to the orchestrator (the team's lead), or open one"},
	{Name: "open-issue", Keys: []string{"t"}, Help: "open the ticket in the tracker"},
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
	run   *ticket.Run
	err   error
	panes []string // agent panes, for the status subscription
	stamp string
}
type tickMsg struct{}
type settledMsg struct{}
type eventMsg struct {
	ch chan herdr.Event
	ok bool
}
type leadMsg struct {
	text string
	err  error
}

func (m *Model) load() tea.Msg {
	live, err := ticket.ReadLive(m.client)
	if err != nil {
		return loadedMsg{err: err}
	}
	run, err := ticket.Load(m.worktree, m.opts, live)
	msg := loadedMsg{run: run, err: err, panes: live.AgentPanes()}
	if run != nil {
		msg.stamp = runStamp(run.Dir)
	}
	return msg
}

// reload loads now, or right after the load already running.
func (m *Model) reload() tea.Cmd {
	if m.loading {
		m.again = true
		return nil
	}
	m.loading = true
	return m.load
}

// runStamp changes whenever a file of the run folder is added, removed or
// written.
func runStamp(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latest time.Time
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return fmt.Sprintf("%d@%d", len(entries), latest.UnixNano())
}

func tick() tea.Cmd {
	return tea.Tick(checkEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

// subscribe listens to Herdr for agents coming, going and changing status.
// Status events are per pane, so a new set of agent panes subscribes again.
func (m *Model) subscribe(panes []string) tea.Cmd {
	key := strings.Join(panes, ",")
	if m.events != nil && key == m.subPanes {
		return nil
	}
	if m.subCancel != nil {
		m.subCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan herdr.Event, 64)
	m.events, m.subCancel, m.subPanes = ch, cancel, key
	client := m.client
	stream := func() tea.Msg {
		_ = client.SubscribeTo(ctx, herdr.AgentSubscriptions(panes), ch)
		close(ch)
		return nil
	}
	return tea.Batch(stream, waitFor(ch))
}

func waitFor(ch chan herdr.Event) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-ch
		return eventMsg{ch: ch, ok: ok}
	}
}

// Init loads the run and starts the refresh clock.
func (m *Model) Init() tea.Cmd {
	m.loading = true
	return tea.Batch(m.load, tick(), m.spinner.Start())
}

// Update handles keys, refreshes and resizes.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loadedMsg:
		m.run, m.err = msg.run, msg.err
		m.loading, m.loadedAt, m.stamp = false, time.Now(), msg.stamp
		m.clamp()
		if msg.run != nil {
			m.note(msg.run.Agents(), time.Now())
		}
		if m.isBusy() && m.busyText == refreshing {
			m.status = ""
		}
		var cmds []tea.Cmd
		if m.client != nil && msg.err == nil {
			cmds = append(cmds, m.subscribe(msg.panes))
		}
		if m.again {
			m.again = false
			cmds = append(cmds, m.reload())
		}
		return m, tea.Batch(cmds...)
	case tickMsg:
		due := time.Since(m.loadedAt) >= fullEvery
		if !due && m.run != nil && runStamp(m.run.Dir) != m.stamp {
			due = true
		}
		if due {
			return m, tea.Batch(m.reload(), tick())
		}
		return m, tick()
	case eventMsg:
		if msg.ch != m.events {
			return m, nil // an earlier subscription, replaced
		}
		if !msg.ok {
			m.events, m.subPanes = nil, "" // dropped; the next load subscribes again
			return m, nil
		}
		if m.settling {
			return m, waitFor(msg.ch)
		}
		m.settling = true
		return m, tea.Batch(waitFor(msg.ch), tea.Tick(settle, func(time.Time) tea.Msg { return settledMsg{} }))
	case settledMsg:
		m.settling = false
		return m, m.reload()
	case leadMsg:
		m.opening = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.status = msg.text
		return m, m.reload()
	case look.SpinMsg:
		return m, m.spinner.Update(msg, m.run == nil && m.err == nil || m.isBusy())
	case tea.KeyMsg:
		return m, m.key(msg.String())
	case tea.MouseMsg:
		switch {
		case msg.Button == tea.MouseButtonWheelUp:
			m.row--
			m.clamp()
		case msg.Button == tea.MouseButtonWheelDown:
			m.row++
			m.clamp()
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
			return m, m.click(msg.X, msg.Y)
		}
	}
	return m, nil
}

// note adds the agents' status changes since the last load to the Activity
// list.
func (m *Model) note(agents []ticket.Agent, at time.Time) {
	var moved []ticket.Change
	m.seen, moved = ticket.Changes(m.seen, agents)
	for _, c := range moved {
		line := at.Format("15:04") + "  " + c.Says()
		if c.Title != "" {
			line += " · " + c.Title
		}
		m.activity = append([]string{line}, m.activity...)
	}
	if len(m.activity) > activityKept {
		m.activity = m.activity[:activityKept]
	}
}

func (m *Model) key(k string) tea.Cmd {
	if m.chord == "g" {
		m.chord = ""
		if k == "g" {
			m.row = 0
		}
		return nil
	}
	if !m.isBusy() {
		m.status = ""
	}
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
		return tea.Batch(m.working(refreshing), m.reload())
	case "o":
		return m.goLead()
	case "t":
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

// goLead goes to the run's lead, or opens a new one when none is open.
func (m *Model) goLead() tea.Cmd {
	if m.run == nil {
		return nil
	}
	if m.opening {
		return nil // its spinner is already on the status line
	}
	run, client := m.run, m.client
	var start tea.Cmd
	if run.LeadPane == "" {
		m.opening = true
		start = m.working("opening a new " + strings.ToLower(run.Lead.Label) + "…")
	}
	return tea.Batch(start, func() tea.Msg {
		text, err := lead.Go(client, run)
		return leadMsg{text: text, err: err}
	})
}

// refreshing is the status while r reloads; the load clears it.
const refreshing = "refreshing…"

// working shows text as the status while the work it names runs, with the
// spinner in front; whatever replaces the status ends it.
func (m *Model) working(text string) tea.Cmd {
	m.status, m.busyText = text, text
	return m.spinner.Start()
}

func (m *Model) isBusy() bool { return m.busyText != "" && m.status == m.busyText }

// statusText is the status line, a live spinner in front while busy.
func (m *Model) statusText() string {
	if m.isBusy() {
		return m.spinner.Frame() + " " + m.status
	}
	return m.status
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
// lookingFor is where a run's state file is expected, for the not-found message.
func (m *Model) lookingFor() string {
	l := m.opts.RunLayout()
	return l.Dir + "/<KEY>/" + l.State
}

func (m *Model) View() string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	if m.err != nil && m.run == nil {
		return errStyle.Render("No run found in "+m.worktree+": "+m.err.Error()) + "\n" + dimStyle.Render("Looking for "+m.lookingFor()+" · q quit")
	}
	if m.run == nil {
		return dimStyle.Render(m.spinner.Frame() + " loading")
	}
	r := m.run
	ic := m.icons
	var b strings.Builder
	m.zones = m.zones[:0]
	// line writes one line that a click turns into the key of action.
	line := func(text, action string) {
		m.keyZone(linesIn(b.String()), 0, lipgloss.Width(text), action)
		b.WriteString(text + "\n")
	}

	team := r.Team
	if team == "" {
		team = "unknown team"
	}
	fmt.Fprintf(&b, "%s  %s\n", titleStyle.Render(r.Key), dimStyle.Render(fmt.Sprintf("%s · %s · %s",
		team, ic.With(look.Target, orDash(r.Target)), ic.With(look.Branch, orDash(r.Branch)))))
	line(linkStyle.Render(ic.With(look.Jira, r.JiraURL)), "open-issue")
	switch {
	case r.Prototype != "":
		line(linkStyle.Render(ic.With(look.Brush, "Prototype "+r.Spec+": "+filepath.Base(r.Prototype))), "prototype")
	case r.Spec != "":
		fmt.Fprintf(&b, "%s\n", dimStyle.Render(ic.With(look.Brush, "No prototype for "+r.Spec)))
	}
	for _, l := range r.Objective {
		fmt.Fprintf(&b, "%s\n", look.Truncate(l, width))
	}
	leadName := strings.ToLower(r.Lead.Label)
	orch := fmt.Sprintf("no %s open · %s opens one", leadName, m.keyMap().Key("lead"))
	orchStyle := dimStyle
	if r.LeadPane != "" {
		al := look.Agent(r.LeadStatus)
		orch = leadName + ": " + orDash(al.Word)
		if r.LeadTitle != "" {
			orch += " · " + r.LeadTitle
		}
		orch += " · " + m.keyMap().Key("lead") + " goes there"
		if r.LeadStatus != "idle" {
			orchStyle = al.Style
		}
	}
	line(orchStyle.Render(look.Truncate(ic.With(look.Sitemap, orch), width)), "lead")
	b.WriteString("\n")

	top := linesIn(b.String())
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
		y := top + 1
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
			if card.PaneID != "" {
				al := look.Agent(card.Status)
				state := orDash(al.Word)
				if card.Title != "" {
					state += " · " + card.Title
				}
				lines = append(lines, al.Style.Render(look.Truncate(ic.With(al.Glyph, state), text)))
			}
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
			boxed := look.Card(lines, cardWidth, border)
			for range boxed {
				m.zones = append(m.zones, zone{y: y, x0: i * colWidth, x1: i*colWidth + cardWidth, col: i, row: j})
				y++
			}
			cb.WriteString(strings.Join(boxed, "\n") + "\n")
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
	if len(m.activity) > 0 {
		b.WriteString(headStyle.Render(ic.With(look.Clock, "Activity")) + "\n")
		for i, l := range m.activity {
			style := lipgloss.NewStyle()
			if i > 0 {
				style = dimStyle
			}
			b.WriteString(style.Render(look.Truncate(l, width)) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(headStyle.Render(ic.With(look.Question, fmt.Sprintf("Open questions (%d)", len(r.Questions)))) + "\n")
	if len(r.Questions) == 0 {
		b.WriteString(dimStyle.Render("none in "+r.Layout().State) + "\n")
	}
	limit := 8
	for i, q := range r.Questions {
		if i == limit {
			b.WriteString(dimStyle.Render(fmt.Sprintf("and %d more in %s", len(r.Questions)-limit, r.Layout().State)) + "\n")
			break
		}
		b.WriteString("- " + look.Truncate(q, width-2) + "\n")
	}
	if r.Landed {
		b.WriteString("\n" + doneStyle.Render(ic.With(look.Rocket, "Landed: the pull request is ready for review")) + "\n")
	}

	b.WriteString("\n")
	if s := m.statusText(); s != "" {
		b.WriteString(dimStyle.Render(s) + "\n")
	}
	if m.err != nil {
		b.WriteString(errStyle.Render(m.err.Error()) + "\n")
	}
	k := func(name string) string { return keyStyle.Render(m.keyMap().Key(name)) }
	b.WriteString(k("left") + "/" + k("right") + " column  " + k("up") + "/" + k("down") + " role  ")
	// The actions are buttons as well as hints.
	y, x := linesIn(b.String()), lipgloss.Width(k("left")+"/"+k("right")+" column  "+k("up")+"/"+k("down")+" role  ")
	for _, a := range []struct{ action, label string }{
		{"jump", "go to its tab"}, {"lead", leadName}, {"open-issue", "tracker"},
		{"prototype", "prototype"}, {"refresh", "refresh"}, {"quit", "quit"},
	} {
		text := k(a.action) + " " + a.label
		m.keyZone(y, x, x+lipgloss.Width(text), a.action)
		b.WriteString(text + "  ")
		x += lipgloss.Width(text) + 2
	}
	return b.String()
}

// linesIn is how many rows s takes before its last line.
func linesIn(s string) int { return strings.Count(s, "\n") }

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
