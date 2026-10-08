package ticketui

import (
	"github.com/ezemacchi/herdr-phin-board/internal/look"
	"github.com/ezemacchi/herdr-phin-board/internal/ticket"
)

// columnGlyph marks the ticket board's columns. Role icons come from the team
// definitions; the glyphs themselves are in package look.
var columnGlyph = map[ticket.Column]string{
	ticket.Pending: look.Circle,
	ticket.Working: look.Cog,
	ticket.Waiting: look.Question,
	ticket.Done:    look.Check,
}
