package ticketui

import (
	"github.com/ezemacchi/herdr-phin-board/internal/look"
	"github.com/ezemacchi/herdr-phin-board/internal/ticket"
)

// roleGlyph is the only icon set of the ticket board's own; the rest are in
// package look.
var roleGlyph = map[string]string{
	"technical-lead": "\uf14e", // compass
	"evidence":       "\uf002", // search
	"implementer":    "\uf121", // code
	"reviewer":       "\uf06e", // eye
	"qa":             "\uf188", // bug
	"curator":        "\uf02d", // book
	"mechanic":       "\uf0ad", // wrench
	"inspector":      "\uf046", // checked box
}

var columnGlyph = map[ticket.Column]string{
	ticket.Pending: look.Circle,
	ticket.Working: look.Cog,
	ticket.Waiting: look.Question,
	ticket.Done:    look.Check,
}
