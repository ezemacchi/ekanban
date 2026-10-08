package screen

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
)

const (
	MinColumnWidth = 18
	MaxColumnWidth = 34
	Gutter         = 1
)

// Column is one kanban column. Widths needs only the header (Label, Count,
// Hidden); Cards are drawn once the column's width is known.
type Column struct {
	Label  string // the header, without its count
	Color  string // the header's colour
	Count  int
	Hidden bool // left out, as a status filter does
	Cards  []Card
	// Selected is the index of the selected card, -1 for none. The column
	// scrolls to keep it in view.
	Selected int
}

// Card is a boxed card (look.Card) and where its choices are, if it holds a
// picker: Choices is the card line of the first, -1 when there are none.
type Card struct {
	Lines    []string
	Choices  int
	NChoices int
}

// Header is the column's title with its card count.
func (c Column) Header() string { return c.Label + fmt.Sprintf(" %d", c.Count) }

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
		case c.Count == 0:
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

// HeaderRow is Owner.Row for the two lines of a column's header.
const HeaderRow = -2

// Owner is what one line of a column belongs to: its header, a card (Row),
// a choice inside a card, or nothing (Row -1).
type Owner struct{ Col, Row, Choice int }

// Render draws column col width cells wide and at most height lines tall:
// the coloured header and its rule, then the cards, scrolled so the selected
// one is in view.
func (c Column) Render(col, width, height int) ([]string, []Owner) {
	inner := width - Gutter
	head := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Color)).Bold(true)
	count := fmt.Sprintf(" %d", c.Count)
	lines := []string{
		head.Render(look.Truncate(c.Label, inner-len(count))) + look.Dim.Render(count),
		look.Dim.Render(strings.Repeat("─", inner)),
	}
	header := Owner{Col: col, Row: HeaderRow, Choice: -1}
	owners := []Owner{header, header}
	if len(c.Cards) == 0 {
		lines = append(lines, look.Dim.Render(look.Truncate("—", inner)))
		owners = append(owners, Owner{Col: col, Row: -1, Choice: -1})
	}

	selectedEnd := -1
	for i, card := range c.Cards {
		for j := range card.Lines {
			o := Owner{Col: col, Row: i, Choice: -1}
			if card.Choices >= 0 && j >= card.Choices && j < card.Choices+card.NChoices {
				o.Choice = j - card.Choices
			}
			owners = append(owners, o)
		}
		lines = append(lines, card.Lines...)
		if i == c.Selected {
			selectedEnd = len(lines)
		}
	}

	if selectedEnd >= 0 && len(lines) > height {
		// Keep the header visible where possible, otherwise follow the card.
		if overflow := selectedEnd - height; overflow > 0 {
			lines = append(append([]string{}, lines[:2]...), lines[2+overflow:]...)
			owners = append(append([]Owner{}, owners[:2]...), owners[2+overflow:]...)
		}
	}
	if len(lines) > height {
		lines, owners = lines[:height], owners[:height]
	}
	return lines, owners
}

// Tallest is how many lines the columns need to show every card.
func Tallest(cols []Column) int {
	h := 3 // header, rule, "—"
	for _, c := range cols {
		n := 2
		for _, card := range c.Cards {
			n += len(card.Lines)
		}
		h = max(h, n)
	}
	return h
}

// Draw lays cols[from:end] out side by side, height lines tall, starting at
// screen row top, and records a zone for every header, card and choice line.
// Each column must already have its Cards for widths[i].
func Draw(z *Zones, cols []Column, widths []int, from, end, top, height int) string {
	type drawn struct {
		lines  []string
		owners []Owner
		width  int
	}
	var shown []drawn
	for i := from; i < end; i++ {
		if widths[i] == 0 {
			continue
		}
		lines, owners := cols[i].Render(i, widths[i], height)
		shown = append(shown, drawn{lines, owners, widths[i]})
	}

	x := 1
	for _, c := range shown {
		for line, o := range c.owners {
			zone := Zone{Y: top + line, X0: x, X1: x + c.width - Gutter, Col: o.Col, Row: o.Row}
			switch {
			case o.Choice >= 0:
				zone.Kind, zone.Choice = OnChoice, o.Choice
			case o.Row == HeaderRow:
				zone.Kind = OnColumn
			case o.Row >= 0:
				zone.Kind = OnCard
			default:
				continue
			}
			z.Add(zone)
		}
		x += c.width
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
