package look

import "github.com/charmbracelet/lipgloss"

// Card border colours: at rest, under the cursor, picked up to move, and
// asking for you. screen.Card chooses between them.
var (
	CardBorder    = lipgloss.Color("240")
	CardSelected  = lipgloss.Color("212")
	CardGrabbed   = lipgloss.Color("213")
	CardAttention = lipgloss.Color("203")
)
