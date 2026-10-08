package screen

import (
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
	edge := lipgloss.NewStyle().Foreground(c.border())

	var lines []string
	var zones []Zone
	add := func(line string) {
		zones = append(zones, Zone{Kind: OnCard, Y: len(lines), X0: 0, X1: width})
		lines = append(lines, line)
	}
	body := func(l string) {
		if lipgloss.Width(l) > inner {
			l = ansi.Truncate(l, inner, "")
		}
		add(edge.Render("│") + " " + Pad(l, inner) + " " + edge.Render("│"))
	}

	add(c.top(width, edge))
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
	add(edge.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	return lines, zones
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
