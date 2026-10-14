package screen

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/look"
)

// Card is one card of a column. A board fills in what it says and what state
// it is in; how that looks (the border's colour, the title's style, where the
// buttons go) is decided here, the same for both boards.
type Card struct {
	Title string   // in the top border
	Badge string   // styled, at the right of the top border
	Body  []string // styled lines, cut to the card's width
	// Actions are buttons under a rule. Only the selected card shows them, so
	// what can be done is next to what it is done to.
	Actions []Hint

	Selected  bool // under the cursor
	Grabbed   bool // picked up to move
	Attention bool // asks for you: the border says so from across the board
	Dim       bool // closed or archived

	// Flat draws the card as one line without a box: Badge, Title, then the
	// first Body line. For a card with nothing more to say, such as a
	// finished role.
	Flat bool

	// Choices is the Body line of the first of NChoices choices (a picker
	// inside the card), each an OnChoice zone.
	Choices, NChoices int

	// Stack is a strip under the card for each related ticket, drawn inside
	// the card's own borders. StackAt is the strip under the cursor while the
	// card is Selected: -1 is the card itself, so a board that sets a Stack
	// must set it. StackMore counts the strips left out for want of room, and
	// Cue is a styled line under the last strip saying where each way leads.
	Stack     []StackLine
	StackAt   int
	StackMore int
	Cue       string
}

// StackLine is one strip of a card's stack.
type StackLine struct {
	Key       string // the related ticket
	Word      string // what it is to the card: "failed"
	Attention bool   // the strip's border and word say it asks for you
}

// cardFrame is what the border and padding take from a card's width.
const cardFrame = 4

// CardInner is the text width of a card width cells wide.
func CardInner(width int) int { return max(width-cardFrame, 1) }

func (c Card) border() lipgloss.Color {
	switch {
	case c.Grabbed:
		return look.CardGrabbed
	case c.Selected:
		return look.CardSelected
	case c.Attention:
		return look.CardAttention
	}
	return look.CardBorder
}

func (c Card) titleStyle() lipgloss.Style {
	switch {
	case c.Grabbed:
		return lipgloss.NewStyle().Foreground(look.CardGrabbed).Bold(true)
	case c.Selected:
		return look.Cursor
	case c.Dim:
		return look.Dim.Bold(true)
	}
	return look.Title
}

// Render draws the card width cells wide. Its zones are relative to its top
// left corner: every line is OnCard, choices OnChoice, buttons OnButton.
func (c Card) Render(width int) ([]string, []Zone) {
	if c.Flat {
		return c.flat(width)
	}
	inner := CardInner(width)
	// With a stack and a strip under the cursor, the card's own box is not
	// the selected one: its border and title say so only when the cursor is
	// on the card.
	head := c
	if len(c.Stack) > 0 && c.Selected && c.StackAt >= 0 {
		head.Selected = false
	}
	edge := lipgloss.NewStyle().Foreground(head.border())

	var lines []string
	var zones []Zone
	add := func(line string) {
		zones = append(zones, Zone{Kind: OnCard, Y: len(lines), X0: 0, X1: width})
		lines = append(lines, line)
	}
	row := func(l string, e lipgloss.Style) {
		if lipgloss.Width(l) > inner {
			l = ansi.Truncate(l, inner, "")
		}
		add(e.Render("│") + " " + Pad(l, inner) + " " + e.Render("│"))
	}
	body := func(l string) { row(l, edge) }

	add(head.top(width, edge))
	for i, l := range c.Body {
		y := len(lines)
		body(l)
		// After the line's own OnCard zone, so the choice is on top.
		if c.NChoices > 0 && i >= c.Choices && i < c.Choices+c.NChoices {
			zones = append(zones, Zone{Kind: OnChoice, Y: y, X0: 0, X1: width, Choice: i - c.Choices})
		}
	}
	if c.Selected && len(c.Actions) > 0 {
		body(look.Dim.Render(strings.Repeat("─", inner)))
		// The last button (the card's menu) always shows: what does not fit
		// gives way from before it, and is still in that menu.
		actions := c.Actions
		for len(actions) > 1 && lipgloss.Width(Buttons(nil, 0, 0, actions, 1<<20)) > inner {
			actions = append(actions[:len(actions)-2:len(actions)-2], actions[len(actions)-1])
		}
		var buttons Zones
		row := Buttons(&buttons, len(lines), 2, actions, inner)
		body(row)
		zones = append(zones, buttons...)
	}
	if len(c.Stack) == 0 {
		add(edge.Render("╰" + strings.Repeat("─", width-2) + "╯"))
		return lines, zones
	}

	// The strips share the card's borders: a joint where the card ends, one
	// between strips, and the card's last line closes the lot. The cursor's
	// strip wins the colour of the lines above and below it.
	style := func(i int) lipgloss.Style { return lipgloss.NewStyle().Foreground(c.stackBorder(head, i)) }
	joint := func(above, below int) {
		e := style(above)
		if c.rowSelected(below) {
			e = style(below)
		}
		add(e.Render("├" + strings.Repeat("─", width-2) + "┤"))
	}
	last := -1
	for i, s := range c.Stack {
		joint(last, i)
		mark, name := "  ", look.Title
		if c.rowSelected(i) {
			mark, name = look.Cursor.Render("❯ "), look.Cursor
		}
		text := mark + name.Render(s.Key)
		if s.Word != "" {
			word := look.Dim
			if s.Attention {
				word = look.Attention
			}
			text += " " + word.Render(s.Word)
		}
		zones = append(zones, Zone{Kind: OnStack, Y: len(lines), X0: 0, X1: width, Choice: i})
		row(text, style(i))
		// The strip's own click zone sits on top of its card zone.
		zones[len(zones)-1], zones[len(zones)-2] = zones[len(zones)-2], zones[len(zones)-1]
		last = i
	}
	if c.StackMore > 0 {
		joint(last, last)
		row(look.Dim.Render(fmt.Sprintf("  +%d more", c.StackMore)), style(last))
	}
	if c.Cue != "" && c.Selected {
		joint(last, last)
		row(c.Cue, style(last))
	}
	add(style(last).Render("╰" + strings.Repeat("─", width-2) + "╯"))
	return lines, zones
}

// rowSelected is whether the cursor is on row i of the card: -1 the card
// itself, 0 and up the strips.
func (c Card) rowSelected(i int) bool { return c.Selected && c.StackAt == i }

// stackBorder is the colour of row i's border: the card's own for -1.
func (c Card) stackBorder(head Card, i int) lipgloss.Color {
	switch {
	case i < 0:
		return head.border()
	case c.Grabbed && c.rowSelected(i):
		return look.CardGrabbed
	case c.rowSelected(i):
		return look.CardSelected
	case c.Stack[i].Attention:
		return look.CardAttention
	}
	return look.CardBorder
}

// top is the top border with the title in it, and the badge at its right.
func (c Card) top(width int, edge lipgloss.Style) string {
	badge := ""
	if c.Badge != "" {
		badge = " " + c.Badge + " "
	}
	// ╭─ title ─…─ badge ─╮
	room := width - 2 - 2 - 1 - lipgloss.Width(badge) - 1
	title := ""
	if room > 0 {
		title = " " + c.titleStyle().Render(look.Truncate(c.Title, room-1)) + " "
	}
	fill := width - 2 - 1 - lipgloss.Width(title) - lipgloss.Width(badge) - 1
	if fill < 0 {
		badge, fill = "", width-2-1-lipgloss.Width(title)-1
	}
	return edge.Render("╭─") + title + edge.Render(strings.Repeat("─", max(fill, 0))) + badge + edge.Render("─╮")
}

// flat is the card on one line: a marker, the badge, the title and the first
// body line.
func (c Card) flat(width int) ([]string, []Zone) {
	mark := "  "
	if c.Selected {
		mark = look.Cursor.Render("❯ ")
	}
	line := mark
	if c.Badge != "" {
		line += c.Badge + " "
	}
	line += c.titleStyle().UnsetBold().Render(c.Title)
	if len(c.Body) > 0 && c.Body[0] != "" {
		line += look.Dim.Render(" · ") + c.Body[0]
	}
	return []string{Pad(TruncateStyled(line, width), width)}, []Zone{{Kind: OnCard, X0: 0, X1: width}}
}
