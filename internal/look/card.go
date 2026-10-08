package look

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Card border colors: at rest, under the cursor, and picked up to move.
var (
	CardBorder    = lipgloss.Color("240")
	CardSelected  = lipgloss.Color("212")
	CardGrabbed   = lipgloss.Color("213")
	cardPadding   = 1
	cardFrameCost = 2 + 2*cardPadding
)

// CardInner is the text width a card of the given outer width has.
func CardInner(width int) int { return max(width-cardFrameCost, 1) }

// Card draws lines in a rounded box exactly width cells wide. Lines may be
// styled; they are clipped or padded to the inner width.
func Card(lines []string, width int, border lipgloss.Color) []string {
	inner := CardInner(width)
	edge := lipgloss.NewStyle().Foreground(border)
	pad := strings.Repeat(" ", cardPadding)
	out := make([]string, 0, len(lines)+2)
	out = append(out, edge.Render("╭"+strings.Repeat("─", inner+2*cardPadding)+"╮"))
	for _, l := range lines {
		if lipgloss.Width(l) > inner {
			l = ansi.Truncate(l, inner, "")
		}
		l += strings.Repeat(" ", inner-lipgloss.Width(l))
		out = append(out, edge.Render("│")+pad+l+pad+edge.Render("│"))
	}
	out = append(out, edge.Render("╰"+strings.Repeat("─", inner+2*cardPadding)+"╯"))
	return out
}
