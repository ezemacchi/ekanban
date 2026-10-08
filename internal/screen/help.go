package screen

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/look"
)

// HelpRow is one line of a help screen: the keys, what they do, and a
// shorter way to say it for a narrow pane.
type HelpRow struct{ Keys, Text, Short string }

// HelpGroup is a titled section of a help screen.
type HelpGroup struct {
	Title string
	Rows  []HelpRow
}

// Help widths: the key column, the narrowest a column of help reads well
// in, and the width below which the short texts are used.
const (
	helpKeys      = 10
	helpColumnMin = 40
	HelpNarrow    = 56
)

// Help lays groups out within width × height, each under its heading, keys in
// the key colour; in two columns when there is room for both. The notes go
// under them, each dropped whole when it does not fit: prose cut short says
// nothing. What does not fit the height ends in "…".
func Help(groups []HelpGroup, notes []string, width, height int) []string {
	// Words wrap when there is room for it; a pane too short for that cuts
	// them instead, so every key is still on screen.
	lines := helpLines(groups, notes, width, true)
	if height > 1 && len(lines) > height {
		lines = helpLines(groups, notes, width, false)
	}
	if height > 1 && len(lines) > height {
		lines = append(lines[:height-1], look.Dim.Render("   …"))
	}
	return lines
}

func helpLines(groups []HelpGroup, notes []string, width int, wrap bool) []string {
	cols := 1
	if width >= 2*helpColumnMin+3 {
		cols = 2
	}
	colW := (width - 1 - 3*(cols-1)) / cols
	blocks := make([][]string, len(groups))
	total := 0
	for i, g := range groups {
		blocks[i] = helpBlock(g, colW, width < HelpNarrow, wrap)
		total += len(blocks[i])
	}

	// The groups stay in order; the second column starts at the group that
	// leaves the two columns closest in height.
	split := len(blocks)
	if cols == 2 {
		best := total * 2
		for k := 1; k < len(blocks); k++ {
			left := 0
			for _, b := range blocks[:k] {
				left += len(b) + 1
			}
			if h := max(left, total+len(blocks)-left); h < best {
				best, split = h, k
			}
		}
	}
	var columns [2][]string
	for i, b := range blocks {
		c := 0
		if i >= split {
			c = 1
		}
		if len(columns[c]) > 0 {
			columns[c] = append(columns[c], "")
		}
		columns[c] = append(columns[c], b...)
	}
	var lines []string
	for i := 0; i < max(len(columns[0]), len(columns[1])); i++ {
		left, right := "", ""
		if i < len(columns[0]) {
			left = columns[0][i]
		}
		if i < len(columns[1]) {
			right = columns[1][i]
		}
		if cols == 2 {
			left = Pad(left, colW) + "   " + right
		}
		lines = append(lines, " "+strings.TrimRight(left, " "))
	}

	var fit []string
	for _, n := range notes {
		if lipgloss.Width(n)+3 <= width {
			fit = append(fit, look.Dim.Render("   "+n))
		}
	}
	if width >= HelpNarrow && len(fit) > 0 {
		lines = append(append(lines, ""), fit...)
	}
	return lines
}

// helpBlock is one group: its heading and its rows, width cells wide. A
// row's words wrap under themselves rather than lose their end.
func helpBlock(g HelpGroup, width int, narrow, wrap bool) []string {
	out := []string{look.Head.Render(g.Title)}
	keyW := 7
	if !narrow {
		for _, r := range g.Rows {
			keyW = max(keyW, lipgloss.Width(r.Keys)+2)
		}
		keyW = min(keyW, helpKeys+6)
	}
	room := max(width-2-keyW, 4)
	for _, r := range g.Rows {
		text := r.Text
		if narrow && r.Short != "" {
			text = r.Short
		}
		lead := "  " + look.Key.Render(Pad(look.Truncate(r.Keys, keyW-1), keyW))
		texts := []string{look.Truncate(text, room)}
		if wrap {
			texts = Wrap(text, room)
		}
		for i, l := range texts {
			if i > 0 {
				lead = strings.Repeat(" ", 2+keyW)
			}
			out = append(out, lead+l)
		}
	}
	return out
}

// HelpSection names the actions of one section of a help screen, in reading
// order. A row is one action, or several whose keys read together ("j / k").
type HelpSection struct {
	Title string
	Rows  [][]string
}

// HelpFor builds a help screen's groups from a key map, so its words are each
// action's own help, written once beside its keys, and its keys are the ones
// bound now. A row whose first action the map lacks, or skip refuses, or that
// has no key, is left out, and so is a section left empty.
func HelpFor(km *keys.Map, sections []HelpSection, skip func(name string) bool) []HelpGroup {
	byName := map[string]keys.Action{}
	for _, a := range km.Actions() {
		byName[a.Name] = a
	}
	var groups []HelpGroup
	for _, s := range sections {
		group := HelpGroup{Title: s.Title}
		for _, names := range s.Rows {
			first, ok := byName[names[0]]
			if !ok || skip != nil && skip(names[0]) {
				continue
			}
			var shown []string
			for _, n := range names {
				if a, ok := byName[n]; ok && len(a.Keys) > 0 {
					shown = append(shown, keys.Display(a.Keys[:1]))
				}
			}
			if len(shown) == 0 {
				continue
			}
			group.Rows = append(group.Rows, HelpRow{Keys: strings.Join(shown, " / "), Text: first.Help, Short: first.Short})
		}
		if len(group.Rows) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

// helpModalMax is the widest a help box gets: two columns of keys.
const helpModalMax = 124

// HelpOver draws the help in a box over base, width × height: the same box
// as a detail's, so help is a window over the board rather than another
// screen. Any key or click closes it; the board says how.
func HelpOver(base string, groups []HelpGroup, notes []string, width, height int) string {
	inner := max(min(width-8, helpModalMax)-4, 20)
	// The border, the title, blank lines and the way out take eight rows.
	body := Help(groups, notes, inner, max(height-8, 1))
	way := look.Dim.Render(look.Truncate("any key or a click to go back", inner))
	box := Modal(Closable(look.Title.Render("Help"), inner), body, []string{way}, inner)
	x, y := Center(lipgloss.Width(box), lipgloss.Height(box), width, height)
	return Overlay(base, box, x, y, height)
}
