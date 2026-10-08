package screen

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/look"
)

// MenuItem is one entry of a menu: what it does, and the key that does the
// same from the board ("" when there is none, or it is not a key).
type MenuItem struct{ Label, Key string }

// Menu is a list of choices drawn in a box over the board: the view switcher,
// a card's actions. It holds only what is drawn; what a choice does is the
// board's.
type Menu struct {
	Title string
	Items []MenuItem
	Sel   int
}

// Move steps the highlight by delta, wrapping around.
func (mn *Menu) Move(delta int) {
	if n := len(mn.Items); n > 0 {
		mn.Sel = ((mn.Sel+delta)%n + n) % n
	}
}

// Key drives the menu from the keyboard. These keys are the menu's own and
// cannot be rebound, the way a picker's are not: chosen is the item picked,
// -1 for none; done is whether the menu closes.
func (mn *Menu) Key(k string) (chosen int, done bool) {
	switch k {
	case "j", "down", "tab":
		mn.Move(1)
	case "k", "up", "shift+tab":
		mn.Move(-1)
	case "enter", " ":
		return mn.Sel, true
	case "esc", "q":
		return -1, true
	}
	return -1, false
}

// MenuHelp is how to drive a menu from the keyboard.
var MenuHelp = Key("j") + "/" + Key("k") + " move · " + Key("enter") + " choose · " + Key("esc") + " close"

// box draws the menu: the title, then each item with its key at the right.
func (mn Menu) box() (string, int) {
	inner := lipgloss.Width(mn.Title)
	for _, it := range mn.Items {
		inner = max(inner, 2+lipgloss.Width(it.Label)+3+lipgloss.Width(KeyName(it.Key)))
	}
	var lines []string
	if mn.Title != "" {
		lines = append(lines, look.Dim.Render(mn.Title))
	}
	for i, it := range mn.Items {
		mark, style := "  ", lipgloss.NewStyle()
		if i == mn.Sel {
			mark, style = look.Cursor.Render("❯ "), look.Cursor
		}
		left := mark + style.Render(it.Label)
		lines = append(lines, JoinEnds(left, look.Key.Render(KeyName(it.Key)), inner))
	}
	return Box(lines, inner, look.CardBorder), len(lines) - len(mn.Items)
}

// Over draws the menu with its top left corner at x, y over frame, moved in
// so it fits width × height, and records its zones: the box, then a choice
// for each item.
func (mn Menu) Over(z *Zones, frame string, x, y, width, height int) string {
	box, head := mn.box()
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	x = max(min(x, width-w), 0)
	y = max(min(y, height-h), 0)
	z.Add(Zone{Kind: OnBox, Y: y, H: h, X0: x, X1: x + w})
	for i := range mn.Items {
		z.Add(Zone{Kind: OnChoice, Y: y + 1 + head + i, X0: x + 1, X1: x + w - 1, Choice: i})
	}
	return Overlay(frame, box, x, y, height)
}

// Box draws lines in a rounded box with one cell of padding, inner cells
// wide inside it, in color.
func Box(lines []string, inner int, color lipgloss.Color) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Width(inner + 2).
		Render(strings.Join(lines, "\n"))
}

// Modal draws a dialog box: a title, a blank line, the body, and, when there
// are any, a blank line and the bottom lines (its buttons or choices).
func Modal(title string, body, bottom []string, inner int) string {
	lines := append([]string{title, ""}, body...)
	if len(bottom) > 0 {
		lines = append(append(lines, ""), bottom...)
	}
	return Box(lines, inner, look.PillColor)
}

// ModalBody is the row of a modal's first body line, counted from its top
// border: under the border, the title and the blank line.
const ModalBody = 3

// closeMark is the ✕ at the right of a closable box's title.
var closeMark = look.Button.Render(" ✕ ")

// Closable is a modal's title with a ✕ at its right end, inner cells wide.
// The keys that close the box stay among its hints; the ✕ is for the mouse.
func Closable(title string, inner int) string {
	return JoinEnds(TruncateStyled(title, inner-4), closeMark, inner)
}

// CloseZone is where the ✕ of a closable modal inner cells wide lands when
// the box's top left corner is at x, y; a click on it presses key.
func CloseZone(x, y, inner int, key string) Zone {
	return Zone{Kind: OnButton, Key: key, Y: y + 1, X0: x + 2 + inner - 3, X1: x + 2 + inner}
}

// Center is where a box w × h sits in the middle of width × height.
func Center(w, h, width, height int) (x, y int) {
	return max((width-w)/2, 0), max((height-h)/2, 0)
}

// Overlay draws box over base with its top left corner at x, y, keeping the
// base visible on either side of each line it covers.
func Overlay(base, box string, x, y, height int) string {
	lines := strings.Split(base, "\n")
	// Pad only as far as the box needs. The board is one line shorter than the
	// screen; padding it to the full height made the frame one line taller
	// while a box was open, and when the box closed the frame shrank and the
	// terminal erased the bottom line of the board -- the row of key hints.
	// Keeping the frame the same height either way leaves that row alone.
	boxLines := strings.Split(box, "\n")
	for len(lines) < min(height, y+len(boxLines)) {
		lines = append(lines, "")
	}
	for i, bl := range boxLines {
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

// FieldLabel is how wide a field's label column is.
const FieldLabel = 11

// Field puts label before the first of lines, in a column of its own, and
// lines up the rest under the first: a detail reads as labelled facts rather
// than a run of lines that all weigh the same. Lines must fit width minus
// FieldLabel.
func Field(label string, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	indent := strings.Repeat(" ", FieldLabel)
	for i, l := range lines {
		lead := indent
		if i == 0 {
			lead = look.Label.Render(Pad(look.Truncate(strings.ToUpper(label), FieldLabel-1), FieldLabel))
		}
		out[i] = lead + l
	}
	return out
}

// ConfirmKeys are the keys a question answers to; like a picker's, they are
// its own and cannot be rebound. Either case counts.
func ConfirmKeys(k string) (yes, no bool) {
	switch k {
	case "y", "Y":
		return true, false
	case "n", "N", "esc":
		return false, true
	}
	return false, false
}

// answer is a question's button: the word with its key in brackets and in
// the key colour, "(Y)es"; a click presses key.
func answer(word, key string) Item {
	text := look.Button.Render(" (") + look.ButtonKey.Render(strings.ToUpper(key)) + look.Button.Render(")"+word[1:]+" ")
	return Item{Text: text, On: &Zone{Kind: OnButton, Key: key}}
}

// Confirm draws a yes-or-no question in a box over base, width × height:
// the title, the lines that say what yes does, and (Y)es and (N)o. It
// records its zones -- the box, its ✕ and the two answers, which press y
// and n -- so the board only has to act on y and n.
func Confirm(z *Zones, base, title string, lines []string, width, height int) string {
	inner := max(min(width-8, 64)-4, 24)
	var body []string
	for _, l := range lines {
		for _, w := range Wrap(l, inner) {
			body = append(body, w)
		}
	}
	var answers Zones
	row := Row(&answers, 0, 0, []Item{answer("Yes", "y"), answer("No", "n")}, 2, inner)
	box := Modal(Closable(look.Title.Render(title), inner), body, []string{row}, inner)
	x, y := Center(lipgloss.Width(box), lipgloss.Height(box), width, height)
	z.Add(Zone{Kind: OnBox, Y: y, H: lipgloss.Height(box), X0: x, X1: x + lipgloss.Width(box)})
	answers.Shift(0, x+2, y+ModalBody+len(body)+1)
	*z = append(*z, answers...)
	z.Add(CloseZone(x, y, inner, "n"))
	return Overlay(base, box, x, y, height)
}
