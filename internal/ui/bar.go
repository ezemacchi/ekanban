package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/screen"
)

// The board's first line is its controls: on the left the view switcher, the
// search field and whatever narrows the board, each with a ✕ that lifts it;
// on the right what asks for you and the archive. Nothing that changes what
// the board shows is hidden behind a key alone.

// viewHeader is the top bar.
func (m *Model) viewHeader() string {
	left := []screen.Item{screen.Control(titleStyle.Render(m.title()), "views")}
	m.input.Width = screen.SearchWidth - 6
	left = append(left, screen.Search(m.filter, m.searchPlaceholder(), m.hintKey("filter"),
		m.mode == modeFilter, m.input.View(), screen.SearchWidth)...)
	if m.filter != "" {
		left = append(left, screen.Text(dimStyle.Render(plural(m.found(), "found", "found"))))
	}
	if st, ok := m.board.StatusByID(m.statusFilter); ok {
		left = append(left, screen.Chip(st.Label+" only", st.Color, "clear-only"))
	}
	return " " + screen.Ends(&m.zones, 0, 1, left, m.headerRight(), m.width-2)
}

// headerRight is the top bar's right end.
func (m *Model) headerRight() []screen.Item {
	var right []screen.Item
	if n := len(m.needingYou()); n > 0 {
		verb := "need"
		if n == 1 {
			verb = "needs"
		}
		right = append(right, keyItem(look.Attention.Render(fmt.Sprintf("● %d %s you", n, verb)), m.hintKey("attention")))
	}
	if b := m.bellSummary(); b != "" {
		right = append(right, screen.Text(b))
	}
	if m.pipelineOn() {
		return append(right, screen.Button(screen.Hint{Key: m.hintKey("archive"), Label: fmt.Sprintf("Archive %d", m.acceptedCount())}))
	}
	live, archived := 0, 0
	for _, group := range m.groups {
		for _, sp := range group {
			if sp.Live {
				live++
			} else {
				archived++
			}
		}
	}
	right = append(right, screen.Text(dimStyle.Render(fmt.Sprintf("%d live", live))))
	label := "show archived"
	if m.showArchive {
		label = fmt.Sprintf("hide archived %d", archived)
	}
	return append(right, screen.Button(screen.Hint{Key: m.hintKey("archived"), Label: label}))
}

// found is how many spaces the board shows, a search narrowing them.
func (m *Model) found() int {
	n := 0
	for _, group := range m.groups {
		n += len(group)
	}
	return n
}

// noMatch is what an empty board says while a search narrows it, width
// cells wide: what was searched, and how to lift it.
func (m *Model) noMatch(width int) string {
	text := "  nothing matches “" + m.filter + "” — the ✕ in the search field lifts it"
	return screen.Say(text+m.press("quit", "does too"), dimStyle, width-1)
}

// more is how many rows of the list or the table are out of view, above and
// below, for the footer: "↑ 3 · ↓ 5 more", or "" when all of it shows.
func (m *Model) more() string {
	rows := m.listHeight()
	if m.layout == layoutTable {
		rows-- // the headings
	}
	if m.layout == layoutKanban || m.sidebar {
		return ""
	}
	above, below := m.offset, max(m.cursorLimit()-m.offset-rows, 0)
	var parts []string
	if above > 0 {
		parts = append(parts, fmt.Sprintf("↑ %d", above))
	}
	if below > 0 {
		parts = append(parts, fmt.Sprintf("↓ %d", below))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ") + " more"
}

// keyItem is text a click on presses key; with no key it is only text.
func keyItem(text, key string) screen.Item {
	if key == "" {
		return screen.Text(text)
	}
	return screen.Item{Text: text, On: &screen.Zone{Kind: screen.OnButton, Key: key}}
}

// startSearch puts the cursor in the search field.
func (m *Model) startSearch() {
	m.mode = modeFilter
	m.input.SetValue(m.filter)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *Model) searchPlaceholder() string {
	if m.pipelineOn() {
		return "Search tickets, titles, notes"
	}
	return "Search spaces, paths, notes"
}

// acceptedCount is how many of the repository's tickets are in the Archive.
func (m *Model) acceptedCount() int {
	n := 0
	for _, e := range m.board.Entries {
		if e.Accepted != nil && e.Accepted.Repo == m.scope {
			n++
		}
	}
	return n
}

// noteHelp is the footer while a note is written in the detail.
var noteHelp = "write the note · " + screen.Key("enter") + " save · " + screen.Key("esc") + " cancel"

// searchHelp is the footer while the search field takes keys.
var searchHelp = "type to search · " + screen.Key("enter") + " keep it · " + screen.Key("esc") + " clear"

// footer is the board's last two lines, as screen.Footer draws them: first
// (already drawn), then hints as buttons with help at the right end.
func (m *Model) footer(first string, hints []hint) string {
	if more := m.more(); more != "" {
		first = screen.JoinEnds(screen.TruncateStyled(first, m.width-lipgloss.Width(more)-3), dimStyle.Render(more+" "), m.width)
	}
	right := []screen.Item{screen.Button(screen.Hint{Key: m.hintKey("help"), Label: "Help"})}
	return screen.Footer(&m.zones, first, hints, right, m.width)
}
