package screen

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
)

// ChooserRow is one choice of a chooser: what it says, and whether it can be
// chosen (Off rows are shown, dim, with why).
type ChooserRow struct {
	Text string // styled
	Off  bool
	Why  string // why it is off
}

// Chooser is a list of choices with a preview of the highlighted one beside
// it, in a box over the board: which agent to send to, with the last lines
// of its pane. It holds only what is drawn; what a choice does is the board's.
type Chooser struct {
	Title   string
	Rows    []ChooserRow
	Sel     int
	Preview []string // styled lines, the newest last
	Note    string   // under the list: loading, or why there is nothing
	// Text is a preview to read rather than a terminal's tail: its lines
	// wrap, and it shows from its top.
	Text bool
	// Use is the label of the button that chooses: "Use" when empty.
	Use string
	// Buttons replace the bottom row (Use and Cancel) when set; a click on
	// one presses its key.
	Buttons []Hint
}

// Move steps the highlight by delta, wrapping around.
func (c *Chooser) Move(delta int) {
	if n := len(c.Rows); n > 0 {
		c.Sel = ((c.Sel+delta)%n + n) % n
	}
}

// Chooser sizes.
const (
	chooserList    = 44 // the list's width, side by side
	chooserSide    = 90 // the least inner width that puts the preview beside the list
	ChooserPreview = 14 // preview lines
)

// Over draws the chooser centred over base, width × height, and records its
// zones: the box, a choice for each row, its buttons (enter and esc) and its
// ✕ (esc).
func (c Chooser) Over(z *Zones, base string, width, height int) string {
	inner := max(min(width-8, 140)-4, 30)
	side := inner >= chooserSide
	listW := inner
	if side {
		listW = chooserList
	}
	var list []string
	for i, r := range c.Rows {
		mark, text := "  ", r.Text
		switch {
		case r.Off:
			// Why it cannot be chosen is what matters: the name gives way.
			why := ""
			if r.Why != "" {
				why = " · " + r.Why
			}
			name := look.Truncate(Plain(ansiStrip(r.Text)), max(listW-2-lipgloss.Width(why), 4))
			text = look.Dim.Render(name + why)
		case i == c.Sel:
			mark = look.Cursor.Render("❯ ")
		}
		if i == c.Sel && r.Off {
			mark = look.Dim.Render("❯ ")
		}
		list = append(list, Pad(TruncateStyled(mark+text, listW), listW))
	}
	if c.Note != "" {
		list = append(list, look.Dim.Render(look.Truncate(c.Note, listW)))
	}

	previewW := inner
	if side {
		previewW = inner - listW - 3
	}
	lines := c.Preview
	from := max(len(lines)-ChooserPreview, 0)
	if c.Text {
		lines = nil
		for _, l := range c.Preview {
			if l == "" {
				lines = append(lines, "")
				continue
			}
			lines = append(lines, Wrap(l, previewW)...)
		}
		from = 0
	}
	preview := make([]string, 0, ChooserPreview)
	for _, l := range lines[from:min(from+ChooserPreview, len(lines))] {
		preview = append(preview, Pad(TruncateStyled(l, previewW), previewW)+"\x1b[0m")
	}
	for len(preview) < ChooserPreview {
		preview = append(preview, strings.Repeat(" ", previewW))
	}

	var body []string
	rule := look.Dim.Render
	if side {
		for i := 0; i < max(len(list), len(preview)); i++ {
			l, p := strings.Repeat(" ", listW), strings.Repeat(" ", previewW)
			if i < len(list) {
				l = list[i]
			}
			if i < len(preview) {
				p = preview[i]
			}
			body = append(body, l+rule(" │ ")+p)
		}
	} else {
		body = append(append(list, rule(strings.Repeat("─", inner))), preview...)
	}

	use := c.Use
	if use == "" {
		use = "Use"
	}
	var buttons Zones
	hints := c.Buttons
	if len(hints) == 0 {
		hints = []Hint{{Key: "enter", Label: use}, {Key: "esc", Label: "Cancel"}}
	}
	row := Buttons(&buttons, 0, 0, hints, inner)
	box := Modal(Closable(look.Title.Render(c.Title), inner), body, []string{row}, inner)
	x, y := Center(lipgloss.Width(box), lipgloss.Height(box), width, height)
	z.Add(Zone{Kind: OnBox, Y: y, H: lipgloss.Height(box), X0: x, X1: x + lipgloss.Width(box)})
	for i := range c.Rows {
		z.Add(Zone{Kind: OnChoice, Y: y + ModalBody + i, X0: x + 2, X1: x + 2 + listW, Choice: i})
	}
	buttons.Shift(0, x+2, y+ModalBody+len(body)+1)
	*z = append(*z, buttons...)
	z.Add(CloseZone(x, y, inner, "esc"))
	return Overlay(base, box, x, y, height)
}

// Terminal output read from a pane is drawn inside the board, so only its
// colours may pass: anything that moves the cursor, clears, sets a title or
// rings would draw over the board.
var (
	sgr     = regexp.MustCompile(`\x1b\[[0-9;:]*m`)
	escapes = regexp.MustCompile(`\x1b\[[0-9;:?<>=!]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[PX^_][^\x1b]*\x1b\\|\x1b[@-Z\\-_]`)
	allEsc  = regexp.MustCompile(`\x1b\[[0-9;:?<>=!]*[ -/]*[@-~]`)
)

// SafeLines splits terminal output into lines that keep only their colours,
// trailing blank lines dropped.
func SafeLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out []string
	for _, l := range strings.Split(text, "\n") {
		out = append(out, safeLine(l))
	}
	for len(out) > 0 && strings.TrimSpace(ansiStrip(out[len(out)-1])) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// safeLine keeps a line's SGR sequences and printable text; tabs become
// spaces and every other control character goes, a stray escape with them.
func safeLine(l string) string {
	l = escapes.ReplaceAllStringFunc(l, func(s string) string {
		if sgr.FindString(s) == s {
			return s
		}
		return ""
	})
	var b strings.Builder
	last := 0
	for _, at := range sgr.FindAllStringIndex(l, -1) {
		b.WriteString(printable(l[last:at[0]]))
		b.WriteString(l[at[0]:at[1]])
		last = at[1]
	}
	b.WriteString(printable(l[last:]))
	return b.String()
}

// printable is s without control characters, its tabs as spaces.
func printable(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString("    ")
		case r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ansiStrip(s string) string { return allEsc.ReplaceAllString(s, "") }
