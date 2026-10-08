package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/pipeline"
)

var (
	titleStyle    = look.Title
	dimStyle      = look.Dim
	cursorStyle   = look.Cursor
	errStyle      = look.Err
	keyStyle      = look.Key
	noteStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	labelStyle    = lipgloss.NewStyle()
	archivedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	focusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	grabStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	branchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
)

// View renders the board.
func (m *Model) View() string {
	// Link regions are rebuilt every frame, so a click is always tested
	// against what is on screen now rather than a stale layout.
	m.resetLinks()
	m.resetZones()
	return m.overlayMenu(m.viewFrame())
}

func (m *Model) viewFrame() string {
	if m.quitting {
		return ""
	}

	if m.archiveView && m.mode == modeNormal {
		return m.viewArchive()
	}

	switch m.mode {
	case modeHelp:
		return m.viewHelp()
	case modeManage, modeManageAdd, modeManageRename:
		return m.viewManage()
	case modeDetail:
		return m.viewDetailOverBoard()
	case modeStatusPick:
		// The picker opens inside what it was opened from: the detail modal,
		// else the selected kanban card, else a bar in the footer.
		if m.prevMode == modeDetail {
			return m.viewDetailOverBoard()
		}
	}

	switch m.layout {
	case layoutKanban:
		return m.viewKanbanBoard()
	case layoutTable:
		return m.viewTable()
	}

	var b strings.Builder
	// A docked board sits under hoarder's own header, which already names the
	// pane, and the status legend does not fit a dock's width. Both rows go
	// back to the list.
	if !m.sidebar {
		b.WriteString(m.viewHeader())
		b.WriteString("\n\n")
	}

	height := m.listHeight()
	left := make([]string, height)
	end := min(m.offset+height, len(m.rows))
	for i := m.offset; i < end; i++ {
		if m.sidebar {
			left[i-m.offset] = m.renderNarrowRow(i)
			continue
		}
		left[i-m.offset] = m.renderRow(i)
	}

	paneWidth := m.detailPaneWidth()
	if paneWidth == 0 {
		for _, line := range left {
			b.WriteString(line)
			b.WriteString("\n")
		}
		footer := m.viewFooter()
		m.placeFooter(linesIn(b.String()))
		b.WriteString(footer)
		return b.String()
	}

	// The pane tracks the cursor, so it always describes what you are looking
	// at without needing to open anything.
	inner := paneWidth - 3
	right := m.detailLines(m.selected(), inner)
	body := m.bodyWidth()

	// The pane starts two rows down (header, blank) and to the right of the
	// separator, which is where its links are drawn.
	m.trackLinks(right, 2, body+1, m.width-1)

	for i := 0; i < height; i++ {
		cell := ""
		if i < len(right) {
			cell = right[i].text
		}
		b.WriteString(padCell(truncateStyled(left[i], body-1), body-1))
		b.WriteString(dimStyle.Render("│ "))
		b.WriteString(strings.TrimRight(cell, " "))
		b.WriteString("\n")
	}

	footer := m.viewFooter()
	m.placeFooter(linesIn(b.String()))
	b.WriteString(footer)
	return b.String()
}

func (m *Model) viewHeader() string {
	// Count from the groups rather than the rows, so a collapsed group or a
	// scrolled-off column still contributes.
	var live, archived int
	for _, group := range m.groups {
		for _, sp := range group {
			if sp.Live {
				live++
			} else {
				archived++
			}
		}
	}

	// The title names the current view and opens the switcher.
	left := " " + titleStyle.Render(m.title())
	if st, ok := m.board.StatusByID(m.statusFilter); ok {
		left += lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color)).Render("  " + st.Label + " only")
	}
	if m.filter != "" {
		left += dimStyle.Render(fmt.Sprintf("  /%s", m.filter))
	}

	right := ""
	if b := m.bellSummary(); b != "" {
		right += b + dimStyle.Render(" · ")
	}
	right += fmt.Sprintf("%d live", live)
	if m.showArchive {
		right += fmt.Sprintf(" · %d archived", archived)
	} else {
		right += " · archive hidden"
	}
	right += " "

	return joinEnds(left, dimStyle.Render(right), m.width)
}

func (m *Model) viewFooter() string {
	if m.err != nil {
		return errStyle.Render(" " + truncate(m.err.Error(), m.width-2))
	}

	switch m.mode {
	case modeNote:
		return keyStyle.Render(" note: ") + m.input.View()
	case modeRename:
		return keyStyle.Render(" rename: ") + m.input.View()
	case modeMessage:
		return keyStyle.Render(" to agent: ") + m.input.View()
	case modeFilter:
		return keyStyle.Render(" filter: ") + m.input.View()
	}

	// A dock is too narrow for the legend, and its rows are worth more as
	// list. Errors and the input prompts above still render -- hiding those
	// would leave you typing blind.
	//
	// What does survive is anything changing which spaces the list is showing.
	// A dock has no header to carry it, and a filter you cannot see is a board
	// that looks like it has lost half your spaces.
	if m.sidebar {
		var parts []string
		if st, ok := m.board.StatusByID(m.statusFilter); ok {
			parts = append(parts, st.Label+" only")
		}
		if m.filter != "" {
			parts = append(parts, "/"+m.filter)
		}
		if m.status != "" {
			parts = append(parts, m.status)
		}
		if len(parts) == 0 {
			return ""
		}
		return dimStyle.Render(" " + truncate(strings.Join(parts, " · "), m.width-2))
	}

	if m.pipelineOn() {
		k := m.hintKey
		hints := []hint{
			{k("accept"), "accept (" + m.columns.Label(pipeline.ReadyQA) + ")"}, {k("archive"), "archive"},
			{k("detail"), "detail"}, {k("note"), "note"}, {k("jump"), "go"}, {k("lead"), "lead"},
			{k("refresh"), "refresh"}, {k("help"), "help"},
		}
		state := m.spinner.Frame() + " reading " + m.pipe.CIName() + " and git"
		if !m.pipeAt.IsZero() {
			state = m.pipe.CIName() + " and git read at " + m.pipeAt.Local().Format("15:04")
			if m.pipeLoading {
				state = m.spinner.Frame() + " " + state + ", reading again"
			}
		}
		if s := m.statusText(); s != "" {
			state += " · " + s
		}
		return dimStyle.Render(" "+truncate(state, m.width-2)) + "\n " + m.buttons(1, 1, hints, m.width-2)
	}

	// The picker in a list or table has no card to open in, so it takes the
	// footer: a bar of choices, then how to drive it.
	if m.mode == modeStatusPick {
		help := "s/arrows move · enter set · esc cancel · or click one"
		if m.layout == layoutKanban {
			return " " + m.statusLegend() + "\n" + dimStyle.Render(" "+truncate(help, m.width-2))
		}
		return m.pickerBar(0, m.width) + "\n" + dimStyle.Render(" "+truncate(help, m.width-2))
	}

	// The numbered statuses are the fastest way to file something, so show the
	// actual mapping rather than a generic "1-9". Each is a button too.
	keys := " " + m.statusLegend()

	var hints []hint
	switch {
	case m.grabbed != "" && m.layout == layoutKanban:
		hints = []hint{{"h/l", "retag"}, {"j/k", "reorder"}, {"enter", "drop"}}
	case m.grabbed != "":
		hints = []hint{{"j/k", "move (across a group changes status)"}, {"enter", "drop"}}
	case m.layout == layoutKanban:
		hints = []hint{{"s", "status"}, {"d", "detail"}, {"n", "note"}, {"v", "move"}, {"enter", "jump"}, {"K", "list"}, {"?", "help"}}
	case m.layout == layoutTable:
		hints = []hint{{"s", "status"}, {"d", "detail"}, {"n", "note"}, {"o", "sort"}, {"enter", "jump"}, {"K", "kanban"}, {"?", "help"}}
	case m.board.HideDetail:
		hints = []hint{{"s", "status"}, {"d", "detail"}, {"n", "note"}, {"v", "move"}, {"enter", "jump"}, {"K", "table"}, {"?", "help"}}
	default:
		hints = []hint{{"s", "status"}, {"n", "note"}, {"v", "move"}, {"enter", "jump"}, {"K", "table"}, {"?", "help"}}
	}
	line := " "
	x := 1
	if s := m.statusText(); s != "" {
		s = truncate(s, max(m.width/2, 20))
		line += dimStyle.Render(s + " · ")
		x += lipgloss.Width(s) + 3
	}
	return keys + "\n" + line + m.buttons(1, x, hints, m.width-x-1)
}

// statusLegend is the numbered statuses, each a button for its digit on the
// footer's first line.
func (m *Model) statusLegend() string {
	var b strings.Builder
	x := 1
	for i, st := range m.board.Statuses {
		if i >= 9 {
			break
		}
		if i > 0 {
			b.WriteString(dimStyle.Render("  "))
			x += 2
		}
		text := fmt.Sprintf("%d %s", i+1, st.Label)
		if x+lipgloss.Width(text) > m.width {
			break
		}
		numbered := lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color))
		b.WriteString(numbered.Render(text))
		w := lipgloss.Width(text)
		m.addZone(zone{kind: zoneButton, y: 0, x0: x, x1: x + w, key: fmt.Sprintf("%d", i+1), footer: true})
		x += w
	}
	return b.String()
}

// statusText is the transient status line, with a live spinner in front while
// the work it names is still running.
func (m *Model) statusText() string {
	if m.status == "" {
		return ""
	}
	if m.isBusy() {
		return m.spinner.Frame() + " " + m.status
	}
	return m.status
}

// working shows text as the status while the work it names runs, with the
// spinner in front. Whatever replaces the status -- the result, an error --
// ends it; nothing has to remember to stop it.
func (m *Model) working(text string) tea.Cmd {
	m.status, m.busyText = text, text
	return m.spinner.Start()
}

func (m *Model) isBusy() bool {
	return m.busyText != "" && m.status == m.busyText && m.err == nil
}

func (m *Model) renderRow(i int) string {
	r := m.rows[i]
	selected := i == m.cursor

	switch r.kind {
	case rowEmpty:
		return dimStyle.Render("  no spaces yet — open a workspace in Herdr and it appears here")

	case rowHeader:
		arrow := "▾"
		if m.board.IsCollapsed(r.status.ID) && m.filter == "" {
			arrow = "▸"
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(r.status.Color)).Bold(true)
		line := fmt.Sprintf("%s %s", arrow, m.statusLabel(r.status))
		out := " " + style.Render(line) + dimStyle.Render(fmt.Sprintf(" (%d)", r.count))
		if selected {
			return cursorStyle.Render("❯") + out[1:]
		}
		return out
	}

	sp := r.space
	held := sp.Key == m.grabbed

	prefix := "   "
	switch {
	case held:
		prefix = grabStyle.Render(" ▌ ")
	case selected:
		prefix = cursorStyle.Render(" ❯ ")
	}

	name := m.spaceLabel(sp)
	if b := m.bellFor(sp.Key); b != "" {
		name = bellGlyph + " " + name
	}
	nameStyled := labelStyle.Render(pad(name, 22))
	switch {
	case held:
		nameStyled = grabStyle.Render(pad(name, 22))
	case !sp.Live:
		nameStyled = archivedStyle.Render(pad(name, 22))
	case sp.Focused:
		nameStyled = focusStyle.Render(pad(name, 22))
	}

	// The note is the point of the "waiting" status, so it wins the middle
	// column whenever there is one; otherwise show where the space lives.
	detail := abbreviate(sp.Key)
	detailStyle := dimStyle
	if sp.Note != "" {
		detail = sp.Note
		detailStyle = noteStyle
	}

	hint := agentHint(sp)
	// Rows share the width with the detail pane when it is open.
	body := m.rowWidth()
	room := body - 3 - 22 - lipgloss.Width(hint) - 2
	if room < 8 {
		room = 8
	}

	line := prefix + nameStyled + " " + detailStyle.Render(truncate(detail, room))
	return joinEnds(line, dimStyle.Render(hint+" "), body)
}

// Narrow rows keep a name readable before anything else earns room, and hold a
// column back on the right so nothing sits flush against the pane edge.
const (
	// A name near this length is the common case, so the pull request only
	// earns its place when the name would not have to give way for it.
	narrowNameFloor = 16
	narrowNoteFloor = 8
	narrowMargin    = " "
)

// renderNarrowRow draws one row for a docked board.
//
// The popup's row does not survive a dock's width. It spends 22 columns on a
// padded name whatever the width is, gives the remainder to a path that
// truncates to nothing readable, and drops its right-aligned agent hint whole
// the moment the two stop fitting -- so the narrower the pane, the less each
// row said. Here the name takes what is left after a one-glyph agent marker,
// and the note -- the reason a row is on the board at all -- takes any room
// after that.
func (m *Model) renderNarrowRow(i int) string {
	r := m.rows[i]
	selected := i == m.cursor
	width := m.rowWidth()

	switch r.kind {
	case rowEmpty:
		return dimStyle.Render(truncate("  no spaces yet", width))

	case rowHeader:
		arrow := "▾"
		if m.board.IsCollapsed(r.status.ID) && m.filter == "" {
			arrow = "▸"
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(r.status.Color)).Bold(true)
		// The cursor colours the arrow rather than taking a column of its own.
		// The popup prepends a ❯, which lands hard against the arrow -- legible
		// at 100 columns, but in a dock it is two glyphs of chrome where the
		// label wanted the room.
		marker := style
		if selected {
			marker = cursorStyle
		}
		head := " " + marker.Render(arrow) + " " + style.Render(m.statusLabel(r.status))
		count := dimStyle.Render(fmt.Sprintf("%d", r.count)) + narrowMargin
		return truncateStyled(joinEnds(head, count, width), width)
	}

	sp := r.space
	held := sp.Key == m.grabbed

	prefix := "  "
	switch {
	case held:
		prefix = grabStyle.Render(" ▌")
	case selected:
		prefix = cursorStyle.Render(" ❯")
	}

	// nameRoom is what the name is left with once a given right-hand cluster is
	// placed: the prefix, a space, the cluster and one column of gap.
	nameRoom := func(right string) int {
		return width - lipgloss.Width(prefix) - 1 - lipgloss.Width(right) - lipgloss.Width(narrowMargin) - 1
	}

	// The right cluster is built in order of how much it matters, and gives way
	// from the least important end as the row runs out -- so a narrow dock keeps
	// the alert and the agent glyph and drops the pull request, rather than
	// losing the lot the way the popup row does when its two columns stop
	// fitting.
	agent := agentGlyph(sp)
	bell := m.bellFor(sp.Key)
	right := joinMarks(bell, agent)
	if pr, ok := m.prFor(sp.Key); ok {
		if full := joinMarks(bell, prShort(pr), agent); nameRoom(full) >= narrowNameFloor {
			right = full
		}
	}
	if nameRoom(right) < narrowNameFloor {
		right = agent
	}

	room := nameRoom(right)
	if room < 4 {
		room = 4
	}

	nameStyled := labelStyle
	switch {
	case held:
		nameStyled = grabStyle
	case !sp.Live:
		nameStyled = archivedStyle
	case sp.Focused:
		nameStyled = focusStyle
	}

	name := truncate(sp.Label, room)
	left := prefix + " " + nameStyled.Render(name)

	// The room a name leaves goes to the note -- what you are waiting on is the
	// whole point of the status it is filed under. With no note the branch takes
	// it, which beats the popup's abbreviated path: a dock has no width for a
	// path, and the branch is what tells two worktrees of one repo apart.
	//
	// Neither is worth a stub. Below the floor the row stays a clean name rather
	// than three columns of a word.
	if rest := room - lipgloss.Width(name) - 1; rest >= narrowNoteFloor {
		switch {
		case sp.Note != "":
			left += " " + noteStyle.Render(truncate(sp.Note, rest))
		case m.branchFor(sp.Key) != "":
			left += " " + branchStyle.Render(truncate(m.branchFor(sp.Key), rest))
		}
	}

	return truncateStyled(joinEnds(left, right+narrowMargin, width), width)
}

// joinMarks spaces out the right-hand markers, skipping the ones that are not
// there so a missing alert does not leave a hole.
func joinMarks(marks ...string) string {
	var out []string
	for _, s := range marks {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

// agentGlyph is agentHint compressed to a single column. A dock cannot spend
// eight columns of every row on "·working", but the state is worth a mark.
func agentGlyph(sp *space) string {
	if !sp.Live {
		return dimStyle.Render("○")
	}
	switch sp.AgentStatus {
	case "working":
		return prPendingStyle.Render("◐")
	case "blocked":
		return prFailStyle.Render("◆")
	case "done":
		return prPassStyle.Render("✓")
	case "idle":
		return dimStyle.Render("·")
	}
	return ""
}

// agentHint is the dim secondary signal. It never groups or sorts anything --
// the board tracks the user's own work, not the agent's.
func agentHint(sp *space) string {
	if !sp.Live {
		return "offline"
	}
	switch sp.AgentStatus {
	case "working", "blocked", "done", "idle":
		return "·" + sp.AgentStatus
	default:
		return ""
	}
}

func (m *Model) viewManage() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" Statuses") + dimStyle.Render("  order here is the order on the board") + "\n\n")

	for i, st := range m.board.Statuses {
		cursor := "   "
		if i == m.manageIdx {
			cursor = cursorStyle.Render(" ❯ ")
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color))
		count := 0
		for _, e := range m.board.Entries {
			if e.Status == st.ID {
				count++
			}
		}
		noun := "spaces"
		if count == 1 {
			noun = "space"
		}
		marker := pad("", 9)
		if m.board.IsDefaultStatus(st.ID) {
			marker = keyStyle.Render(pad("default", 9))
		}
		b.WriteString(cursor + style.Render(pad(st.Label, 24)) + marker +
			dimStyle.Render(fmt.Sprintf("%d %s", count, noun)) + "\n")
	}

	b.WriteString("\n")
	switch m.mode {
	case modeManageAdd:
		b.WriteString(keyStyle.Render(" new status: ") + m.input.View())
	case modeManageRename:
		b.WriteString(keyStyle.Render(" rename to: ") + m.input.View())
	default:
		b.WriteString(dimStyle.Render(" a add · r rename · d delete · D set default · J/K reorder · esc back"))
		if m.status != "" {
			b.WriteString("\n" + dimStyle.Render(" "+m.status))
		}
	}
	return b.String()
}

// helpRow is one key and what it does, in a long form and a short one. The
// short form is not the long one truncated: a clipped sentence loses its verb
// and says nothing, so the narrow board gets phrasing written for it.
type helpRow struct {
	// actions are named in actions.go; the row shows their current keys, so a
	// binding in config.toml shows up here too.
	actions     []string
	long, short string
	// wide marks a key that only does something on a board wide enough for the
	// arrangements it switches between. Listing those in a dock, where they are
	// deliberately inert, spends its scarcest rows on things that will not
	// happen.
	wide bool
}

var helpRows = []helpRow{
	{actions: []string{"layout"}, long: "cycle the view: list → table → kanban", wide: true},
	{actions: []string{"sort"}, long: "table only: sort by status, name, or when it last changed", wide: true},
	{actions: []string{"detail"}, long: "list: show or hide the detail pane · elsewhere: detail modal", wide: true},
	{actions: []string{"down", "up"}, long: "move", short: "move"},
	{actions: []string{"top", "bottom"}, long: "first row · last row", short: "first · last"},
	{actions: []string{"open-pr"}, long: "open the pull request in a browser", short: "open the PR"},
	{actions: []string{"send-failure"}, long: "send the failing check, with the end of its log, to that space's agent", short: "send the failure"},
	{actions: []string{"left", "right"}, long: "kanban: move between columns · list: collapse / expand", short: "fold · unfold"},
	{actions: []string{"grab"}, long: "grab a row, then move it — leaving its group changes its status", short: "grab and move"},
	{actions: []string{"jump"}, long: "jump to space (reopens archived ones)", short: "jump to space"},
	{actions: []string{"set-status"}, long: "send to that status, numbered along the bottom", short: "set status"},
	{actions: []string{"status-picker"}, long: "status picker", short: "status picker"},
	{actions: []string{"note"}, long: "edit note — who or what you are waiting on", short: "edit note"},
	{actions: []string{"rename"}, long: "rename the space — renames the Herdr workspace too", short: "rename space"},
	{actions: []string{"message"}, long: "type a message into that space's agent, then go there to send it", short: "message agent"},
	{actions: []string{"fold"}, long: "collapse / expand group", short: "fold group"},
	{actions: []string{"status-only"}, long: "show only the status under the cursor — F or esc for all", short: "this status only"},
	{actions: []string{"reorder-spaces"}, long: "reorder Herdr's own Spaces sidebar to match this board", short: "reorder Spaces"},
	{actions: []string{"archived"}, long: "show or hide archived spaces", short: "archived"},
	{actions: []string{"filter"}, long: "filter by name, path or note", short: "filter"},
	{actions: []string{"statuses"}, long: "manage statuses (add, rename, reorder, delete)", short: "statuses"},
	{actions: []string{"forget"}, long: "forget the selected space", short: "forget space"},
	{actions: []string{"refresh"}, long: "refresh", short: "refresh"},
	{actions: []string{"quit"}, long: "quit", short: "quit"},
}

// helpKeys is a row's key column: each action's first current key. False
// when the screen in front has none of them (archived, on the computed board).
func (m *Model) helpKeys(r helpRow) (string, bool) {
	var parts []string
	for _, name := range r.actions {
		if k := m.hintKey(name); k != "?" {
			parts = append(parts, keys.Display([]string{k}))
		}
	}
	return strings.Join(parts, " / "), len(parts) > 0
}

// helpKeyColumn is how much room the keys get. "gg / G" is the longest, and in
// a dock the description needs every column the keys do not.
const (
	helpKeyColumn       = 10
	helpKeyColumnNarrow = 7
	// helpNarrowUnder is the width below which the help changes shape rather
	// than just clipping.
	helpNarrowUnder = 56
)

func (m *Model) viewHelp() string {
	narrow := m.width < helpNarrowUnder

	indent, keyCol := "   ", helpKeyColumn
	if narrow {
		indent, keyCol = " ", helpKeyColumnNarrow
	}
	room := m.width - lipgloss.Width(indent) - keyCol - 1

	lines := []string{titleStyle.Render(" Board"), ""}
	if m.pipelineOn() {
		lines = append(lines,
			dimStyle.Render(indent+truncate("Columns come from the run, "+m.pipe.CIName()+" and git; they are not moved by hand.", room+keyCol)),
			indent+keyStyle.Render(pad(m.hintKey("accept"), keyCol))+dimStyle.Render(truncate("accept a ticket in "+m.columns.Label(pipeline.ReadyQA)+": it moves to the Archive", room)),
			indent+keyStyle.Render(pad(m.hintKey("archive"), keyCol))+dimStyle.Render(truncate("Archive of accepted tickets (o Jira, p PR, u restore, / search)", room)),
			"")
	}
	for _, r := range helpRows {
		key, ok := m.helpKeys(r)
		if !ok {
			continue
		}
		text := r.long
		if narrow {
			// A key that does nothing here is not worth a row.
			if r.wide {
				continue
			}
			text = r.short
		}
		lines = append(lines, indent+keyStyle.Render(pad(key, keyCol))+
			dimStyle.Render(truncate(text, room)))
	}

	// The closing notes are the first thing to go: they explain the board
	// rather than drive it, and a dock has no rows to spare for prose.
	//
	// They are dropped whole rather than truncated, note by note: a clipped
	// sentence of prose is worse than no sentence, having taken a row to say
	// nothing.
	if !narrow {
		notes := []string{
			"   status is yours; the dim right column is Herdr's agent state",
			"   mouse: wheel scrolls · click selects · click again jumps · click a header folds",
			"   docked, one click jumps — and the board closes behind you either way",
		}
		var fit []string
		for _, n := range notes {
			if lipgloss.Width(n) <= m.width {
				fit = append(fit, dimStyle.Render(n))
			}
		}
		if len(fit) > 0 {
			lines = append(lines, "")
			lines = append(lines, fit...)
		}
	}

	// Clip rather than overflow. A help taller than the pane scrolls its own
	// first rows into the scrollback -- which is exactly where the keys you
	// opened it for have gone, since the list starts at the top.
	footer := dimStyle.Render(" " + truncate("any key to go back", max0(m.width-2)))
	if room := m.height - 2; room > 1 && len(lines) > room {
		lines = lines[:room-1]
		lines = append(lines, dimStyle.Render(indent+"…"))
	}

	lines = append(lines, "", footer)
	return strings.Join(lines, "\n")
}

// --- text helpers ---

func pad(s string, n int) string {
	s = truncate(s, n)
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

var truncate = look.Truncate

// truncateStyled clips an already-styled string without cutting through an
// escape sequence, which plain slicing would do.
func truncateStyled(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "")
}

// joinEnds puts left and right on one line, right-aligned to width.
func joinEnds(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// abbreviate shortens a home-relative path for display.
func abbreviate(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
