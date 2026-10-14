package screen

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
)

// Pill is text on a filled ground: a name that heads what follows, or a state
// said once and loudly. Rounded, its ends are Nerd Font glyphs; without them
// it is the ground alone, square.
func Pill(text string, bg lipgloss.Color, rounded bool) string {
	fill := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("231")).Bold(true)
	if !rounded {
		return fill.Render(" " + text + " ")
	}
	edge := lipgloss.NewStyle().Foreground(bg)
	return edge.Render(look.PillLeft) + fill.Render(" "+text+" ") + edge.Render(look.PillRight)
}
