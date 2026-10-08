package screen

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
)

const (
	MinColumnWidth = 18
	MaxColumnWidth = 36
	Gutter         = 1
)

// Column is one kanban column. Widths needs only the header (Label, Count,
// Hidden, Folded); Cards are drawn once the column's width is known.
type Column struct {
	Label  string // the header, without its count
	Color  string // the header's colour
	Count  int
	Hidden bool // left out, as a status filter does
	// Folded columns show their header alone, narrow; a click on the arrow
	// opens them again.
	Folded bool
	Cards  []Card
	// Selected is the index of the selected card, -1 for none. The column
	// scrolls to keep it in view.
	Selected int
}

// Heading is a group's or a column's title, the same on every board and in
// every view: its arrow, its name in its colour and its count, cut to width.
// An empty one is faint and closed, since it has nothing to open.
func Heading(label, color string, count int, folded bool, width int) string {
	head := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Faint(count == 0)
	n := fmt.Sprintf(" %d", count)
	return head.Render(Arrow(folded || count == 0)+" "+look.Truncate(label, width-len(n)-2)) + look.Dim.Render(n)
}

// closed is whether the column shows no cards: folded, or empty.
func (c Column) closed() bool { return c.Folded || c.Count == 0 }

// Header is the column's title with its card count.
func (c Column) Header() string {
	return Arrow(c.closed()) + " " + c.Label + fmt.Sprintf(" %d", c.Count)
}

// Widths gives each column its width, 0 for a hidden one. An empty column
// takes only what its header needs, so the columns holding cards get the
// room; those share what is left, within sane bounds.
func Widths(cols []Column, width int) []int {
	usable := width - 1
	widths := make([]int, len(cols))
	full, used := 0, 0
	for i, c := range cols {
		switch {
		case c.Hidden:
		case c.closed():
			widths[i] = lipgloss.Width(c.Header()) + Gutter
			used += widths[i]
		default:
			full++
		}
	}
	if full == 0 {
		return widths
	}
	share := MinColumnWidth
	if room := usable - used; room/full > MinColumnWidth {
		share = min(room/full, MaxColumnWidth)
	}
	for i, c := range cols {
		if widths[i] == 0 && !c.Hidden {
			widths[i] = share
		}
	}
	return widths
}

// ScrollColumns keeps column selected on screen when the columns do not all
// fit width: it returns the first column to draw and the end of the range.
func ScrollColumns(widths []int, offset, selected, width int) (from, end int) {
	usable := width - 1
	span := func(from, to int) int {
		total := 0
		for _, w := range widths[from:to] {
			total += w
		}
		return total
	}
	offset = min(max(offset, 0), max(len(widths)-1, 0))
	if selected < offset {
		offset = selected
	}
	for offset < selected && span(offset, selected+1) > usable {
		offset++
	}
	// Pull earlier columns back in while they fit.
	for offset > 0 && span(offset-1, len(widths)) <= usable {
		offset--
	}
	end = offset
	for end < len(widths) && (end == offset || span(offset, end+1) <= usable) {
		end++
	}
	return offset, max(end, min(selected+1, len(widths)))
}

// Render draws column col width cells wide and at most height lines tall:
// the coloured header and its rule, then the cards, scrolled so the selected
// one is in view. Its zones are relative to the column's top left corner.
//
// A click on the header's arrow folds the column (OnControl "fold"); one on
// the rest of the header is OnColumn.
func (c Column) Render(col, width, height int) ([]string, []Zone) {
	inner := width - Gutter
	lines := []string{
		Heading(c.Label, c.Color, c.Count, c.Folded, inner),
		look.Dim.Render(strings.Repeat("─", inner)),
	}
	zones := []Zone{
		{Kind: OnColumn, Y: 0, X0: 0, X1: inner, Col: col},
		{Kind: OnColumn, Y: 1, X0: 0, X1: inner, Col: col},
	}
	if c.Count > 0 {
		zones = append(zones, Zone{Kind: OnControl, ID: "fold", Y: 0, X0: 0, X1: 2, Col: col})
	}
	if c.closed() {
		return lines, zones
	}

	selectedEnd := -1
	var starts, ends []int // each card's first line and the line after its last
	for i, card := range c.Cards {
		starts = append(starts, len(lines))
		cl, cz := card.Render(inner)
		for _, z := range cz {
			z.Y += len(lines)
			z.Col, z.Row = col, i
			zones = append(zones, z)
		}
		lines = append(lines, cl...)
		ends = append(ends, len(lines))
		if i == c.Selected {
			selectedEnd = len(lines)
		}
	}

	// Cards cut off above or below are counted where they would be, so a
	// column never just stops: its rule says how many are above, and its last
	// line how many are below.
	overflow := 0
	if selectedEnd >= 0 && len(lines) > height {
		// Keep the header visible where possible, otherwise follow the card.
		overflow = max(selectedEnd-height, 0)
	}
	if overflow > 0 {
		lines = append(append([]string{}, lines[:2]...), lines[2+overflow:]...)
		kept := zones[:0:0]
		for _, z := range zones {
			switch {
			case z.Y < 2:
			case z.Y < 2+overflow:
				continue
			default:
				z.Y -= overflow
			}
			kept = append(kept, z)
		}
		zones = kept
	}
	above, below := 0, 0
	for i := range starts {
		if overflow > 0 && ends[i] <= 2+overflow {
			above++
		}
		if starts[i]-overflow >= height-1 && len(lines) > height {
			below++
		}
	}
	if len(lines) > height {
		last := height
		if below > 0 {
			last = height - 1 // the line that says how many are below
		}
		lines = lines[:height]
		kept := zones[:0:0]
		for _, z := range zones {
			if z.Y < last {
				kept = append(kept, z)
			}
		}
		zones = kept
		if below > 0 {
			lines[height-1] = look.Dim.Render(Pad(look.Truncate(fmt.Sprintf(" ↓ %d more", below), inner), inner))
		}
	}
	if above > 0 {
		mark := fmt.Sprintf(" ↑ %d more ", above)
		lines[1] = look.Dim.Render("─" + mark + strings.Repeat("─", max(inner-1-lipgloss.Width(mark), 0)))
	}
	return lines, zones
}

// Tallest is how many lines the columns need to show every card.
func Tallest(cols []Column) int {
	h := 2 // header, rule
	for _, c := range cols {
		n := 2
		if !c.closed() {
			for _, card := range c.Cards {
				lines, _ := card.Render(MaxColumnWidth)
				n += len(lines)
			}
		}
		h = max(h, n)
	}
	return h
}

// Draw lays cols[from:end] out side by side, height lines tall, starting at
// screen row top, and records the zones of every header, card and button.
// Each column must already have its Cards.
func Draw(z *Zones, cols []Column, widths []int, from, end, top, height int) string {
	type drawn struct {
		lines []string
		width int
	}
	var shown []drawn
	x := 1
	for i := from; i < end; i++ {
		if widths[i] == 0 {
			continue
		}
		lines, zones := cols[i].Render(i, widths[i], height)
		for _, zone := range zones {
			zone.Y += top
			zone.X0 += x
			zone.X1 += x
			z.Add(zone)
		}
		shown = append(shown, drawn{lines, widths[i]})
		x += widths[i]
	}

	var b strings.Builder
	for line := 0; line < height; line++ {
		var row strings.Builder
		for _, c := range shown {
			cell := ""
			if line < len(c.lines) {
				cell = c.lines[line]
			}
			row.WriteString(Pad(cell, c.width))
		}
		b.WriteString(" " + strings.TrimRight(row.String(), " ") + "\n")
	}
	return b.String()
}
