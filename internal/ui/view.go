package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/screen"
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

	if m.mode == modeHandoff {
		// The handoff box sits over the board it was opened from.
		m.mode = modeNormal
		base := m.viewFrame()
		m.mode = modeHandoff
		m.resetZones()
		return m.viewHandoff(base)
	}

	if m.mode == modeConfirm {
		// The question sits over the board it was asked from.
		m.mode = modeNormal
		base := m.viewFrame()
		m.mode = modeConfirm
		m.resetZones()
		return screen.Confirm(&m.zones, base, m.ask.title, m.ask.lines, m.width, m.height)
	}

	if m.archiveView && m.mode == modeNormal {
		return m.viewArchive()
	}

	switch m.mode {
	case modeHelp:
		if m.sidebar {
			return m.viewHelp() // a dock has no room for a box: the help takes it
		}
		// The help is a box over the board it was opened from.
		m.mode = modeNormal
		base := m.viewFrame()
		m.mode = modeHelp
		m.resetZones()
		return m.viewHelpOver(base)
	case modeManage, modeManageAdd, modeManageRename:
		return m.viewManage()
	case modeDetail:
		return m.viewDetailOverBoard()
	case modeNote:
		// The note is written in the detail it was opened from.
		if m.prevMode == modeDetail {
			return m.viewDetailOverBoard()
		}
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
	// at without needing to open anything. Its buttons sit at its foot.
	inner := paneWidth - 3
	body := m.bodyWidth()
	var buttons []string
	if sp := m.selected(); sp != nil {
		mark := len(m.zones)
		buttons = screen.ButtonRows(&m.zones, 0, 0, m.detailButtons(sp), inner)
		m.zones.Shift(mark, body+1, 2+height-len(buttons))
	}
	right := m.detailLines(m.selected(), inner)
	if room := height - len(buttons) - 1; len(right) > room && room > 0 {
		right = right[:room]
	}
	for len(right) < height-len(buttons) {
		right = append(right, detailLine{})
	}
	right = append(right, plain(buttons...)...)

	// The pane starts two rows down (header, blank) and to the right of the
	// separator, which is where its links are drawn.
	m.trackLinks(right, 2, body+1, m.width-1)

	for i := 0; i < height; i++ {
		cell := ""
		if i < len(right) {
			cell = right[i].text
		}
		b.WriteString(screen.Pad(screen.TruncateStyled(left[i], body-1), body-1))
		b.WriteString(dimStyle.Render("│ "))
		b.WriteString(strings.TrimRight(cell, " "))
		b.WriteString("\n")
	}

	footer := m.viewFooter()
	m.placeFooter(linesIn(b.String()))
	b.WriteString(footer)
	return b.String()
}

func (m *Model) viewFooter() string {
	if m.err != nil {
		return errStyle.Render(" " + truncate(m.err.Error(), m.width-2))
	}

	switch m.mode {
	case modeNote:
		if m.noteInDetail() {
			return screen.Say(" "+noteHelp, dimStyle, m.width-1)
		}
		return keyStyle.Render(" note: ") + m.input.View()
	case modeRename:
		return keyStyle.Render(" rename: ") + m.input.View()
	case modeMessage:
		return keyStyle.Render(" to agent: ") + m.input.View()
	case modeFilter:
		// A dock has no top bar: the field is down here.
		if m.sidebar {
			return keyStyle.Render(" filter: ") + m.input.View()
		}
		return screen.Say(" "+searchHelp, dimStyle, m.width-1)
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
		return screen.Say(" "+strings.Join(parts, " · "), dimStyle, m.width-1)
	}

	k := m.hintKey
	if m.pipelineOn() {
		hints := []hint{
			{Key: k("filter"), Label: "Search"}, {Key: k("detail"), Label: "Detail"},
			{Key: k("orchestrator"), Label: "Orchestrator"}, {Key: k("yank"), Label: "Copy"},
			{Key: k("refresh"), Label: "Refresh"},
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
		return m.footer(screen.Say(" "+state, dimStyle, m.width-1), hints)
	}

	// The picker in a list or table has no card to open in, so it takes the
	// footer: a bar of choices, then how to drive it.
	if m.mode == modeStatusPick {
		help := " " + pickerHelp + " · or click one"
		if m.layout == layoutKanban {
			return " " + m.statusLegend() + "\n" + screen.Say(help, dimStyle, m.width-1)
		}
		return m.pickerBar(0, m.width) + "\n" + screen.Say(help, dimStyle, m.width-1)
	}

	pair := func(a, b string) string {
		if k(a) == "" || k(b) == "" {
			return ""
		}
		return k(a) + "/" + k(b)
	}
	var hints []hint
	switch {
	case m.grabbed != "" && m.layout == layoutKanban:
		hints = []hint{{Key: pair("left", "right"), Label: "Retag"}, {Key: pair("down", "up"), Label: "Reorder"}, {Key: k("jump"), Label: "Drop"}}
	case m.grabbed != "":
		hints = []hint{{Key: pair("down", "up"), Label: "Move (across a group changes status)"}, {Key: k("jump"), Label: "Drop"}}
	case m.layout == layoutKanban:
		hints = []hint{{Key: k("filter"), Label: "Search"}, {Key: k("status-picker"), Label: "Status"}, {Key: k("note"), Label: "Note"}, {Key: k("grab"), Label: "Move"}, {Key: k("detail"), Label: "Detail"}}
	case m.layout == layoutTable:
		hints = []hint{{Key: k("filter"), Label: "Search"}, {Key: k("status-picker"), Label: "Status"}, {Key: k("note"), Label: "Note"}, {Key: k("sort"), Label: "Sort"}, {Key: k("detail"), Label: "Detail"}}
	default:
		hints = []hint{{Key: k("filter"), Label: "Search"}, {Key: k("status-picker"), Label: "Status"}, {Key: k("note"), Label: "Note"}, {Key: k("grab"), Label: "Move"}, {Key: k("jump"), Label: "Go"}, {Key: k("detail"), Label: "Detail pane"}}
	}
	// The numbered statuses are the fastest way to file something, so show the
	// actual mapping rather than a generic "1-9". Each is a button too. A
	// message takes their line while it is fresh.
	first := screen.Say(" "+m.statusText(), dimStyle, m.width-1)
	if m.statusText() == "" {
		first = " " + m.statusLegend()
	}
	return m.footer(first, hints)
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
		m.addZone(zone{Kind: zoneButton, Y: 0, X0: x, X1: x + w, Key: fmt.Sprintf("%d", i+1), Footer: true})
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

// listName and listFact are how much of a list row the name and the fact at
// its right end take.
const (
	listName = 22
	listFact = 34
)

// renderRow is one row of the list: a group's heading, the same as a kanban
// column's, or a space as a one-line card -- its name, what it is about, and
// at the right the fact that says most about it.
func (m *Model) renderRow(i int) string {
	r := m.rows[i]
	selected := i == m.cursor
	body := m.rowWidth()

	switch r.kind {
	case rowEmpty:
		if m.filter != "" {
			return m.noMatch(body)
		}
		return dimStyle.Render("  no spaces yet — open a workspace in Herdr and it appears here")

	case rowHeader:
		lead := " "
		if selected {
			lead = cursorStyle.Render("❯")
		}
		folded := m.board.IsCollapsed(r.status.ID) && m.filter == ""
		return lead + screen.Heading(m.statusLabel(r.status), r.status.Color, r.count, folded, body-1)
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

	name := truncate(m.spaceLabel(sp), listName)
	style := look.Title
	switch {
	case held:
		style = grabStyle
	case selected:
		style = cursorStyle
	case !sp.Live:
		style = archivedStyle
	case sp.Focused:
		style = focusStyle
	}
	named := m.marked(sp, style.Render(name))
	named += strings.Repeat(" ", max(listName+2-lipgloss.Width(named), 1))

	right := ""
	if f, ok := m.heaviest(sp); ok {
		right = m.line(f, min(listFact, body/3))
	}
	room := max(body-lipgloss.Width(prefix)-lipgloss.Width(named)-lipgloss.Width(right)-2, 8)
	return screen.JoinEnds(prefix+named+m.about(sp, room), right+" ", body)
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
		if m.filter != "" {
			return m.noMatch(width)
		}
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
		return screen.TruncateStyled(screen.JoinEnds(head, count, width), width)
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

	return screen.TruncateStyled(screen.JoinEnds(left, right+narrowMargin, width), width)
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
		k := screen.Key
		b.WriteString(screen.Say(" "+k("a")+" add · "+k("r")+" rename · "+k("d")+" delete · "+k("D")+" set default · "+k("J/K")+" reorder · "+k("esc")+" back", dimStyle, m.width-1))
		if m.status != "" {
			b.WriteString("\n" + screen.Say(" "+m.status, dimStyle, m.width-1))
		}
	}
	return b.String()
}

// helpLayout is how the help groups the actions, in reading order; the
// words are each action's own help, in actions.go.
var helpLayout = []screen.HelpSection{
	{Title: "Move", Rows: [][]string{{"down", "up"}, {"left", "right"}, {"top", "bottom"}, {"page-down", "page-up"}, {"attention"}}},
	{Title: "The board", Rows: [][]string{
		{"filter"}, {"status-only"}, {"fold"}, {"layout"}, {"sort"}, {"archive"}, {"archived"},
		{"statuses"}, {"reorder-spaces"}, {"refresh"}, {"help"}, {"quit"},
	}},
	{Title: "The selected card", Rows: [][]string{
		{"jump"}, {"detail"}, {"menu"}, {"open-issue"}, {"prototype"}, {"open-pull-request"}, {"open-pr"},
		{"orchestrator"}, {"yank"}, {"note"}, {"accept"}, {"handoff"}, {"status-picker"}, {"set-status"},
		{"grab"}, {"rename"}, {"message"}, {"send-failure"}, {"forget"},
	}},
}

// wideOnly are actions that only do something on a board wide enough for the
// arrangements they switch between. A dock leaves them out of its help rather
// than spend its scarcest rows on things that will not happen.
var wideOnly = map[string]bool{"layout": true, "sort": true, "detail": true}

// notComputed are actions the computed board refuses: its columns are not
// set by hand, and its pull requests do not come from GitHub.
var notComputed = map[string]bool{"status-picker": true, "set-status": true, "statuses": true, "open-pr": true, "send-failure": true}

// helpGroups are the help's sections for the screen in front.
func (m *Model) helpGroups() []screen.HelpGroup {
	km := m.keyMap()
	if km == nil {
		km, _ = keys.New(m.defaultActions(), nil)
	}
	return screen.HelpFor(km, helpLayout, func(name string) bool {
		return m.sidebar && wideOnly[name] || m.pipelineOn() && notComputed[name]
	})
}

// helpNotes explain the board rather than drive it.
func (m *Model) helpNotes() []string {
	notes := []string{
		"mouse: click selects · click again jumps · every button does what its key does",
		"a column's ▾ folds it · its name shows it alone · ✕ lifts a search or a filter",
	}
	if m.pipelineOn() {
		return append([]string{
			"columns come from the run, " + m.pipe.CIName() + " and git; they are not moved by hand",
			"in the Archive" + screen.Plain(m.archiveKeysNote()),
		}, notes...)
	}
	return append([]string{"status is yours; the dim right column is Herdr's agent state"}, notes...)
}

// viewHelpOver is the help in a box over the board, like the detail.
func (m *Model) viewHelpOver(base string) string {
	return screen.HelpOver(base, m.helpGroups(), m.helpNotes(), m.width, m.height)
}

func (m *Model) viewHelp() string {
	lines := []string{" " + titleStyle.Render("Help"), ""}
	// The way out always stays on screen: two rows are kept for it.
	body := screen.Help(m.helpGroups(), m.helpNotes(), m.width, max(m.height-len(lines)-2, 1))
	lines = append(lines, body...)
	lines = append(lines, "", dimStyle.Render(" "+truncate("any key to go back", max0(m.width-2))))
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
