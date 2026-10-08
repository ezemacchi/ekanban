package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/screen"
)

// The detail view exists because a row can only show a truncated note. In the
// list it sits alongside as a pane, updating as you move; in kanban the columns
// already use the full width, so it opens as a modal instead.

const (
	detailPaneMin   = 32
	detailPaneMax   = 60
	detailPaneFloor = 76 // below this terminal width the pane is not worth it
)

// detailLine is one rendered line, optionally standing for a URL.
//
// Herdr opens URLs in pane content on click, but only in panes that have not
// taken the mouse -- and this board has, for the view switcher. So the links
// are ours to handle: the text can then be a short label rather than a raw URL
// that would wrap and break the click anyway.
type detailLine struct {
	text string
	url  string
	key  string // what a click on it presses, when it is a field to edit
}

func plain(lines ...string) []detailLine {
	out := make([]detailLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, detailLine{text: l})
	}
	return out
}

// texts drops the links, for the callers that only draw.
func texts(lines []detailLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.text)
	}
	return out
}

// detailPaneWidth is the room the list view gives the pane, separator
// included. Zero means no pane: either turned off, or the terminal is too
// narrow to split without crushing the list.
func (m *Model) detailPaneWidth() int {
	if m.sidebar || m.board.HideDetail || m.layout != layoutList || m.width < detailPaneFloor {
		return 0
	}
	// The pane carries the full note, so it earns more room than the rows it
	// sits beside -- those can truncate.
	w := m.width * 2 / 5
	if w < detailPaneMin {
		w = detailPaneMin
	}
	if w > detailPaneMax {
		w = detailPaneMax
	}
	return w
}

// bodyWidth is the width left for the list itself.
func (m *Model) bodyWidth() int {
	return m.width - m.detailPaneWidth()
}

// rowWidth is what a row may actually fill. With the pane open it stops a
// column short, so a right-aligned agent hint cannot touch the separator.
func (m *Model) rowWidth() int {
	if m.detailPaneWidth() > 0 {
		return m.bodyWidth() - 1
	}
	return m.bodyWidth()
}

// detailLines renders one space's full state, wrapped to width.
func (m *Model) detailLines(sp *space, width int) []detailLine {
	if sp == nil {
		return plain(dimStyle.Render(truncate("nothing selected", width)))
	}

	lines := plain(m.modalTitle(sp, width), dimStyle.Render(strings.Repeat("─", width)))
	return append(lines, m.detailBody(sp, width)...)
}

// detailBody is the detail without the name: what the card is about, then
// labelled fields -- status, note, what asks for you, the ticket, the agent,
// the pull request, the workspace. The modal shows the name in its title.
func (m *Model) detailBody(sp *space, width int) []detailLine {
	var lines []detailLine
	value := max(width-screen.FieldLabel, 8)
	field := func(label string, field []detailLine) {
		if len(field) == 0 {
			return
		}
		drawn := screen.Field(label, texts(field))
		for i := range field {
			field[i].text = drawn[i]
		}
		lines = append(lines, field...)
	}

	// What the card is about comes first, as on the card.
	if h := m.headline(sp); h != "" {
		for _, l := range screen.Wrap(h, width) {
			lines = append(lines, detailLine{text: titleStyle.Render(l)})
		}
		lines = append(lines, detailLine{})
	}

	if st, ok := m.board.StatusByID(sp.StatusID); ok {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color)).Bold(true)
		field("status", plain(style.Render(truncate(m.statusLabel(st), value))))
	}

	// What happened while you were away: the reason the card is calling.
	field("alerts", plain(m.alertLines(sp.Key, value)...))

	// The note is a field to write in, where it is read: a click on it, or
	// the note key, edits it right here.
	field("note", m.noteBox(sp, value))

	byKind := map[string][]detailLine{}
	for _, f := range m.facts(sp) {
		byKind[f.kind] = append(byKind[f.kind], detailLine{text: m.line(f, value), url: f.url})
	}
	key := m.ticketOf(sp)
	field("ticket", append(byKind[kindTicket], m.jiraTests(key, value)...))
	field("agent", byKind[kindAgent])
	if m.pipelineOn() {
		field("pull req", append(byKind[kindPR], m.bitbucketDetailLines(m.pipeInfo[sp.Key], value)...))
	} else if pr, ok := m.prFor(sp.Key); ok {
		field("pull req", prDetailLines(pr, value))
	}
	field("info", byKind[""])
	field("comment", m.jiraComment(key, value))

	if _, jiraOnly := jiraCardKey(sp.Key); jiraOnly {
		return lines // a ticket with no folder, branch or workspace yet
	}
	var where []string
	for _, line := range screen.Wrap(m.glyph(look.Folder, abbreviate(sp.Key)), value) {
		where = append(where, dimStyle.Render(line))
	}
	if branch := m.branchFor(sp.Key); branch != "" {
		mark := "⎇ " + branch
		if m.icons {
			mark = look.Branch + " " + branch
		}
		where = append(where, branchStyle.Render(truncate(mark, value)))
	}
	state := "archived"
	if sp.Live {
		state = "live"
		if ids := strings.Join(sp.WorkspaceIDs, ", "); ids != "" {
			state += " · " + ids
		}
		if sp.AgentStatus != "" {
			state += " · " + sp.AgentStatus
		}
	}
	where = append(where, dimStyle.Render(truncate(m.glyph(look.Monitor, state), value)))
	if !sp.UpdatedAt.IsZero() {
		where = append(where, dimStyle.Render(truncate(m.glyph(look.Clock, "changed "+humanAge(sp.UpdatedAt)), value)))
	}
	field("workspace", plain(where...))
	return lines
}

// Bounds for the modal's content width. It takes what its widest line needs
// between these, so a long note or branch is not wrapped into a narrow strip.
const (
	detailModalMin    = 40
	detailModalMax    = 84
	detailModalMargin = 8
)

// viewDetailModal draws the selected space's detail in a box over the board,
// which stays visible around it.
// viewDetailOverBoard is the detail modal over the board it was opened from.
// The board's links and zones sit under the box; only the box's count.
func (m *Model) viewDetailOverBoard() string {
	base := m.viewKanbanBoard()
	if m.layout == layoutTable {
		base = m.viewTable()
	}
	m.resetLinks()
	m.resetZones()
	return m.viewDetailModal(base)
}

// pickerHelp is how to drive the status picker, whose keys are its own and
// cannot be rebound.
var pickerHelp = screen.Key("s") + "/arrows move · " + screen.Key("enter") + " set · " + screen.Key("esc") + " cancel"

func (m *Model) viewDetailModal(base string) string {
	sp := m.selected()
	maxInner := max(min(m.width-detailModalMargin, detailModalMax)-4, detailModalMin)
	// The ✕ in the title closes it for the mouse; the hint stays for the keys.
	hints := append(m.detailButtons(sp), hint{Key: m.hintKey("quit"), Label: "Close"})
	picking := m.mode == modeStatusPick

	// Lay out at the widest allowed, then shrink to what the content uses and
	// lay out again so wrapped lines fill the final width.
	inner := lipgloss.Width(screen.Buttons(nil, 0, 0, hints, 1<<20))
	for _, l := range texts(m.modalContent(sp, maxInner)) {
		inner = max(inner, lipgloss.Width(l))
	}
	inner = max(min(inner, maxInner), detailModalMin)
	content := m.modalContent(sp, inner)

	// The buttons, or while picking the status choices, close the box. Their
	// zones are recorded from row 0 at column 0 and moved once the box is placed.
	mark := len(m.zones)
	var bottom []string
	if picking {
		bottom = m.pickerLines(inner)
		for i := range m.board.Statuses {
			m.addZone(zone{Kind: zoneStatus, Y: i + 1, X0: 0, X1: inner, Choice: i})
		}
		bottom = append(bottom, screen.Say(pickerHelp, dimStyle, inner))
	} else {
		bottom = screen.ButtonRows(&m.zones, 0, 0, hints, inner)
	}

	box := screen.Modal(screen.Closable(m.modalTitle(sp, inner-4), inner), texts(content), bottom, inner)

	// Record where each line landed so a click can find its URL: past the
	// border, the title and the blank line under it.
	boxW, boxH := lipgloss.Width(box), lipgloss.Height(box)
	originX, originY := screen.Center(boxW, boxH, m.width, m.height)
	m.trackLinks(content, originY+screen.ModalBody, originX+2, originX+2+inner)

	// Past the body and the blank line under it.
	m.zones.Shift(mark, originX+2, originY+screen.ModalBody+len(content)+1)
	// The box goes under its own zones: a click inside it on nothing does not
	// close it. Its ✕ does, at the right of the title.
	m.zones = append(m.zones[:mark], append([]zone{{Kind: zoneModal, Y: originY, H: boxH, X0: originX, X1: originX + boxW}}, m.zones[mark:]...)...)
	if k := m.hintKey("quit"); k != "" {
		m.addZone(screen.CloseZone(originX, originY, inner, k))
	}

	return screen.Overlay(base, box, originX, originY, m.height)
}

// noteBox is the note as a field width cells wide: the note, or what to write
// in it, behind a pencil; while it is being written, the input itself.
func (m *Model) noteBox(sp *space, width int) []detailLine {
	if m.mode == modeNote && sp == m.selected() {
		m.input.Width = max(width-6, 4)
		return plain(screen.Input("✎", "", "", true, m.input.View(), width))
	}
	key := m.hintKey("note")
	var lines []detailLine
	for _, l := range screen.TextBox("✎", screen.Wrap(sp.Note, max(width-4, 4)), "click to write a note: who or what you are waiting on", width) {
		lines = append(lines, detailLine{text: l, key: key})
	}
	return lines
}

// noteInDetail is whether the note being written is in a detail on screen,
// rather than in the footer.
func (m *Model) noteInDetail() bool {
	return m.mode == modeNote && (m.prevMode == modeDetail || m.layout == layoutList && m.detailPaneWidth() > 0)
}

func (m *Model) modalContent(sp *space, width int) []detailLine {
	if sp == nil {
		return plain(dimStyle.Render("nothing selected"))
	}
	return m.detailBody(sp, width)
}

// modalTitle is the space's name. With icons on it sits in a pill whose
// rounded ends are Nerd Font glyphs; without them, plain bold text.
func (m *Model) modalTitle(sp *space, width int) string {
	if sp == nil {
		return titleStyle.Render("detail")
	}
	// Whatever asks for you is said at the right, where the card has its red
	// dot; otherwise the ticket's type.
	right := ""
	if why := m.attention(sp); why != "" {
		right = look.Attention.Render(truncate("● "+why, width/2))
	} else if i, ok := m.jr.issues[m.ticketOf(sp)]; ok && i.Type != "" {
		right = dimStyle.Render(i.Type)
	}
	name := truncate(m.spaceLabel(sp), width-4-lipgloss.Width(right))
	title := titleStyle.Render(name)
	if m.icons {
		edge := lipgloss.NewStyle().Foreground(look.PillColor)
		fill := lipgloss.NewStyle().Background(look.PillColor).Foreground(lipgloss.Color("231")).Bold(true)
		title = edge.Render(look.PillLeft) + fill.Render(" "+name+" ") + edge.Render(look.PillRight)
	}
	return screen.JoinEnds(title, right, width)
}

func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Local().Format("2006-01-02")
	}
}
