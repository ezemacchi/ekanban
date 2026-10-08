package screen

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/look"
)

// Hint is one clickable key hint in a footer: what to press and what it does.
type Hint struct{ Key, Label string }

// Buttons draws hints as "key label · key label" within width, recording each
// as an OnButton zone on footer row line, starting at column x. A hint with no
// key (an action unbound in [keys]) is left out.
func Buttons(z *Zones, line, x int, hints []Hint, width int) string {
	var b strings.Builder
	used := 0
	sep := look.Dim.Render(" · ")
	for i, h := range hints {
		if h.Key == "" {
			continue
		}
		text := look.Key.Render(h.Key) + look.Dim.Render(" "+h.Label)
		w := lipgloss.Width(text)
		gap := 0
		if i > 0 && used > 0 {
			gap = 3
		}
		if used+gap+w > width {
			break
		}
		if gap > 0 {
			b.WriteString(sep)
		}
		z.Add(Zone{Kind: OnButton, Y: line, X0: x + used + gap, X1: x + used + gap + w, Key: h.Key, Footer: true})
		b.WriteString(text)
		used += gap + w
	}
	return b.String()
}

// JoinEnds puts left and right on one line, right-aligned to width.
func JoinEnds(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// Pad pads a rendered cell to width, ignoring ANSI escapes.
func Pad(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// TruncateStyled cuts an already styled string to width.
func TruncateStyled(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "")
}

// Wrap breaks text on word boundaries, splitting words that are too long.
func Wrap(s string, width int) []string {
	if width < 4 {
		width = 4
	}
	var lines []string
	var current string

	for _, word := range strings.Fields(s) {
		for lipgloss.Width(word) > width {
			head := string([]rune(word)[:width])
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			lines = append(lines, head)
			word = string([]rune(word)[width:])
		}
		switch {
		case current == "":
			current = word
		case lipgloss.Width(current)+1+lipgloss.Width(word) <= width:
			current += " " + word
		default:
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
