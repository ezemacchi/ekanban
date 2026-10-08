package screen

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
)

// Item is a piece of a line: what it draws and, when On is set, what a click
// on it does. Rows of items are how both boards draw their bars, so a button
// looks and behaves the same wherever it is.
type Item struct {
	Text string
	On   *Zone // Kind, Key, ID, Col; its place is filled in when it is drawn
}

// Text is an item a click does nothing on.
func Text(text string) Item { return Item{Text: text} }

// Control is text a click on hands to the board by id (an OnControl zone).
func Control(text, id string) Item {
	return Item{Text: text, On: &Zone{Kind: OnControl, ID: id}}
}

// Row lays items out left to right from column x, gap cells apart, within
// width, and records a zone on row y for each clickable one. An item that
// does not fit is skipped and the next ones are still tried, so a short item
// at the end (a card's ⋯) survives a long one before it. z may be nil to
// measure.
func Row(z *Zones, y, x int, items []Item, gap, width int) string {
	var b strings.Builder
	used := 0
	for _, it := range items {
		if it.Text == "" {
			continue
		}
		w := lipgloss.Width(it.Text)
		g := 0
		if used > 0 {
			g = gap
		}
		if used+g+w > width {
			continue
		}
		b.WriteString(strings.Repeat(" ", g))
		if it.On != nil && z != nil {
			zone := *it.On
			zone.Y, zone.X0, zone.X1 = y, x+used+g, x+used+g+w
			z.Add(zone)
		}
		b.WriteString(it.Text)
		used += g + w
	}
	return b.String()
}

// Ends draws left from column x and right flush with x+width on row y. The
// left side has the room first; the right gets what is left after a gap.
func Ends(z *Zones, y, x int, left, right []Item, width int) string {
	l := Row(z, y, x, left, 2, width)
	room := width - lipgloss.Width(l) - 2
	if room <= 0 || len(right) == 0 {
		return l
	}
	w := lipgloss.Width(Row(nil, 0, 0, right, 2, room))
	r := Row(z, y, x+width-w, right, 2, room)
	return l + strings.Repeat(" ", width-lipgloss.Width(l)-w) + r
}

// Hint is one action a button stands for: its key and what it does.
type Hint struct{ Key, Label string }

// KeyName is how a key reads on a button.
func KeyName(k string) string {
	switch k {
	case "enter":
		return "⏎"
	case " ":
		return "space"
	}
	return k
}

// Button is a hint drawn as a button, its key then its label on a shaded
// ground; a click presses the key. An action with no key has no button.
func Button(h Hint) Item {
	if h.Key == "" {
		return Item{}
	}
	text := look.ButtonKey.Render(" "+KeyName(h.Key)) + look.Button.Render(" "+h.Label+" ")
	return Item{Text: text, On: &Zone{Kind: OnButton, Key: h.Key}}
}

// ButtonItems are hints as buttons.
func ButtonItems(hints []Hint) []Item {
	out := make([]Item, 0, len(hints))
	for _, h := range hints {
		out = append(out, Button(h))
	}
	return out
}

// Buttons draws hints as a row of buttons on row y from column x.
func Buttons(z *Zones, y, x int, hints []Hint, width int) string {
	return Row(z, y, x, ButtonItems(hints), 1, width)
}

// ButtonRows draws hints as buttons in as many rows as they need within
// width, recording zones from row y at column x.
func ButtonRows(z *Zones, y, x int, hints []Hint, width int) []string {
	var rows []string
	for rest := hints; len(rest) > 0; {
		fit := 1
		for fit < len(rest) && lipgloss.Width(Buttons(nil, 0, 0, rest[:fit+1], 1<<20)) <= width {
			fit++
		}
		rows = append(rows, Buttons(z, y+len(rows), x, rest[:fit], width))
		rest = rest[fit:]
	}
	return rows
}

// Chip is a narrowing in force, "Reviewing only ✕", in its colour; a click
// on it lifts it.
func Chip(label, color, id string) Item {
	style := lipgloss.NewStyle().Background(look.ButtonBg).Foreground(lipgloss.Color(color)).Bold(true)
	return Control(style.Render(" "+label+" ")+look.Button.Render("✕ "), id)
}

// SearchWidth is how wide the search field is drawn.
const SearchWidth = 34

// placeholder is how a field's hint reads: dim on the field's ground.
var placeholder = look.Button.Foreground(lipgloss.Color("244"))

// Input is a text field width cells wide, drawn as one: on a shaded ground,
// its lead (a glyph, or the key that focuses it), then text, or hint dimmed
// when there is none. While typing it shows input, the text input's own view.
func Input(lead, text, hint string, typing bool, input string, width int) string {
	lead = " " + lead + " "
	room := max(width-lipgloss.Width(lead)-1, 4)
	var body string
	switch {
	case typing:
		body = input
	case text != "":
		body = look.Button.Render(look.Truncate(text, room))
	default:
		body = placeholder.Render(look.Truncate(hint, room))
	}
	if gap := room - lipgloss.Width(body); gap > 0 {
		body += look.Button.Render(strings.Repeat(" ", gap))
	}
	return look.ButtonKey.Render(lead) + body + look.Button.Render(" ")
}

// TextBox is a field holding text of several lines, width cells wide: the
// lines on a shaded ground, the first after lead, the rest under it. With no
// lines it is one line holding hint.
func TextBox(lead string, lines []string, hint string, width int) []string {
	if len(lines) == 0 {
		return []string{Input(lead, "", hint, false, "", width)}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if i == 0 {
			out[i] = Input(lead, l, "", false, "", width)
			continue
		}
		out[i] = Input(strings.Repeat(" ", lipgloss.Width(lead)), l, "", false, "", width)
	}
	return out
}

// Search is the search field. At rest it shows the query, or what can be
// searched for; while typing it shows input, the text input's own view. A
// click on it starts a search ("search"), and a ✕ after a query clears it
// ("clear-search"). key is the search key, shown in the field when it has
// one.
func Search(query, hint, key string, typing bool, input string, width int) []Item {
	lead := " "
	if key != "" {
		lead = KeyName(key)
	}
	items := []Item{Control(Input(lead, query, hint, typing, input, width), "search")}
	if query != "" && !typing {
		items = append(items, Control(look.Button.Render(" ✕ "), "clear-search"))
	}
	return items
}

// Arrow is how a foldable heading shows whether it is open.
func Arrow(folded bool) string {
	if folded {
		return "▸"
	}
	return "▾"
}

// Section is a foldable heading, "▾ Open questions 2"; a click on it folds or
// unfolds it. count < 0 leaves the count out.
func Section(title string, count int, folded bool, id string) Item {
	text := look.Head.Render(Arrow(folded) + " " + title)
	if count >= 0 {
		text += look.Dim.Render(" " + strconv.Itoa(count))
	}
	return Control(text, id)
}

// Footer is a board's last two lines: state (already styled) and its
// buttons, with right flush to the right edge. Its zones count from the
// footer's first line until PlaceFooter.
func Footer(z *Zones, state string, hints []Hint, right []Item, width int) string {
	mark := len(*z)
	line := Ends(z, 1, 1, ButtonItems(hints), right, width-2)
	z.MarkFooter(mark)
	return state + "\n " + line
}
