package screen

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/look"
)

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

// Inline draws the inline marks a run's markdown uses: **bold** in bold and
// `code` in the key colour, the rest in base, cut to width cells (<= 0 does
// not cut). A mark left open runs to the end of the line.
func Inline(s string, base lipgloss.Style, width int) string {
	var b strings.Builder
	bold, code := false, false
	var run strings.Builder
	flush := func() {
		if run.Len() == 0 {
			return
		}
		style := base
		if bold {
			style = style.Bold(true)
		}
		if code {
			style = look.Key
		}
		b.WriteString(style.Render(run.String()))
		run.Reset()
	}
	for i := 0; i < len(s); i++ {
		switch {
		case strings.HasPrefix(s[i:], "**") && !code:
			flush()
			bold = !bold
			i++
		case s[i] == '`':
			flush()
			code = !code
		default:
			run.WriteByte(s[i])
		}
	}
	flush()
	return TruncateStyled(b.String(), width)
}
