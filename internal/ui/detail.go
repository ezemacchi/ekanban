package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/links"
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

	name := sp.Label
	if !sp.Live {
		name = archivedStyle.Render(truncate(name, width))
	} else {
		name = titleStyle.Render(truncate(name, width))
	}

	lines := plain(name, dimStyle.Render(strings.Repeat("─", width)))
	return append(lines, m.detailBody(sp, width)...)
}

// detailBody is the detail without the name: status, note, alerts, pull
// request and machine facts. The modal shows the name in its title instead.
func (m *Model) detailBody(sp *space, width int) []detailLine {
	var lines []detailLine
	if st, ok := m.board.StatusByID(sp.StatusID); ok {
		lines = append(lines, plain(lipgloss.NewStyle().Foreground(lipgloss.Color(st.Color)).Bold(true).Render(truncate(m.statusLabel(st), width)))...)
	}
	lines = append(lines, plain("")...)

	// The note is the reason this view exists, so it gets the room it needs.
	if sp.Note != "" {
		noteWidth := width - lipgloss.Width(m.glyph(look.Pencil, ""))
		for i, line := range screen.Wrap(sp.Note, noteWidth) {
			lead := m.glyph(look.Pencil, "")
			if i > 0 {
				lead = strings.Repeat(" ", lipgloss.Width(lead))
			}
			lines = append(lines, plain(noteStyle.Render(lead+line))...)
		}
	} else {
		lines = append(lines, plain(dimStyle.Render(truncate(m.glyph(look.Pencil, "no note — press n to add one"), width)))...)
	}
	lines = append(lines, plain("")...)

	// What happened while you were away comes first: it is the reason the row
	// was calling for attention.
	if alerts := m.alertLines(sp.Key, width); len(alerts) > 0 {
		lines = append(lines, plain(alerts...)...)
		lines = append(lines, plain("")...)
	}

	// PR context sits between the note and the machine facts: it is about the
	// work, but unlike the note it is not something you wrote.
	if m.pipelineOn() {
		if info, ok := m.pipeInfo[sp.Key]; ok && info.Title != "" {
			for _, line := range screen.Wrap(info.Title, width) {
				lines = append(lines, plain(line)...)
			}
			lines = append(lines, plain("")...)
		}
		facts := m.pipelineLines(sp, width)
		if info := m.pipeInfo[sp.Key]; info.PR > 0 {
			// The pull request line opens Bitbucket on click.
			for i, l := range facts {
				if strings.Contains(l, fmt.Sprintf("PR #%d", info.PR)) {
					lines = append(lines, detailLine{text: l, url: links.PullRequest(info.PR)})
					facts = append(facts[:i:i], facts[i+1:]...)
					break
				}
			}
		}
		lines = append(lines, plain(facts...)...)
		lines = append(lines, plain("")...)
	} else if pr, ok := m.prFor(sp.Key); ok {
		lines = append(lines, prDetailLines(pr, width)...)
		lines = append(lines, plain("")...)
	}

	for _, line := range screen.Wrap(m.glyph(look.Folder, abbreviate(sp.Key)), width) {
		lines = append(lines, plain(dimStyle.Render(line))...)
	}
	if branch := m.branchFor(sp.Key); branch != "" {
		mark := "⎇ " + branch
		if m.icons {
			mark = look.Branch + " " + branch
		}
		lines = append(lines, plain(branchStyle.Render(truncate(mark, width)))...)
	}

	where := "archived"
	if sp.Live {
		where = "live"
		if ids := strings.Join(sp.WorkspaceIDs, ", "); ids != "" {
			where += " · " + ids
		}
		if sp.AgentStatus != "" {
			where += " · " + sp.AgentStatus
		}
	}
	lines = append(lines, plain(dimStyle.Render(truncate(m.glyph(look.Monitor, where), width)))...)

	if !sp.UpdatedAt.IsZero() {
		lines = append(lines, plain(dimStyle.Render(truncate(m.glyph(look.Clock, "changed "+humanAge(sp.UpdatedAt)), width)))...)
	}
	return lines
}

// Bounds for the modal's content width. It takes what its widest line needs
// between these, so a long note or branch is not wrapped into a narrow strip.
const (
	detailModalMin    = 40
	detailModalMax    = 110
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

func (m *Model) viewDetailModal(base string) string {
	sp := m.selected()
	maxInner := min(m.width-detailModalMargin, detailModalMax) - 4
	if maxInner < detailModalMin {
		maxInner = detailModalMin
	}
	hints := []hint{{"n", "note"}, {"s", "status"}, {"enter", "jump"}, {"esc", "close"}}
	if m.pipelineOn() {
		hints = []hint{{"n", "note"}, {"enter", "jump"}, {"esc", "close"}}
	}
	picking := m.mode == modeStatusPick

	// Lay out at the widest allowed, then shrink to what the content uses and
	// lay out again so wrapped lines fill the final width.
	inner := 0
	for _, h := range hints {
		inner += lipgloss.Width(h.key+" "+h.label) + 3
	}
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
		bottom = append(bottom, dimStyle.Render(truncate("s/arrows move · enter set · esc cancel", inner)))
	} else {
		bottom = []string{m.buttons(0, 0, hints, inner)}
		for i := mark; i < len(m.zones); i++ {
			m.zones[i].Footer = false
		}
	}

	title := m.modalTitle(sp, inner)
	body := title + "\n\n" + strings.Join(texts(content), "\n") + "\n\n" + strings.Join(bottom, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Width(inner + 2).
		Render(body)

	// Record where each line landed so a click can find its URL: past the
	// border, the title and the blank line under it.
	boxW, boxH := lipgloss.Width(box), lipgloss.Height(box)
	originX := max0((m.width - boxW) / 2)
	originY := max0((m.height - boxH) / 2)
	m.trackLinks(content, originY+3, originX+2, originX+2+inner)

	// Past the border, the title, the blank line, the content and the blank.
	bottomY := originY + 1 + 2 + len(content) + 1
	for i := mark; i < len(m.zones); i++ {
		m.zones[i].Y += bottomY
		m.zones[i].X0 += originX + 2
		m.zones[i].X1 += originX + 2
	}
	// The box goes under its own zones: a click inside it on nothing does not
	// close it.
	m.zones = append(m.zones[:mark], append([]zone{{Kind: zoneModal, Y: originY, H: boxH, X0: originX, X1: originX + boxW}}, m.zones[mark:]...)...)

	return overlay(base, box, originX, originY, m.width, m.height)
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
	name := truncate(m.spaceLabel(sp), width-4)
	if !m.icons {
		return titleStyle.Render(name)
	}
	edge := lipgloss.NewStyle().Foreground(look.PillColor)
	fill := lipgloss.NewStyle().Background(look.PillColor).Foreground(lipgloss.Color("231")).Bold(true)
	return edge.Render(look.PillLeft) + fill.Render(" "+name+" ") + edge.Render(look.PillRight)
}

// overlay draws box over base with its top-left corner at x, y, keeping the
// base visible on either side of each line it covers.
func overlay(base, box string, x, y, width, height int) string {
	lines := strings.Split(base, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, bl := range strings.Split(box, "\n") {
		row := y + i
		if row < 0 || row >= len(lines) {
			continue
		}
		l := lines[row]
		left := ansi.Truncate(l, x, "")
		if gap := x - ansi.StringWidth(left); gap > 0 {
			left += strings.Repeat(" ", gap)
		}
		right := ansi.TruncateLeft(l, x+ansi.StringWidth(bl), "")
		lines[row] = left + "\x1b[0m" + bl + "\x1b[0m" + right
	}
	return strings.Join(lines, "\n")
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
