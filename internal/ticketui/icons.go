package ticketui

import "github.com/ezemacchi/herdr-phin-board/internal/ticket"

// glyphs are Nerd Font code points; they are shown only when icons are on.
type glyphs struct {
	on bool
}

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
	ticket.Pending: "\uf10c", // empty circle
	ticket.Working: "\uf013", // cog
	ticket.Waiting: "\uf059", // question
	ticket.Done:    "\uf00c", // check
}

const (
	glyphJira      = "\ue75a"
	glyphBranch    = "\ue725"
	glyphTarget    = "\uf419"
	glyphNow       = "\uf04b"
	glyphLanded    = "\uf135"
	glyphStuck     = "\uf071"
	glyphOrch      = "\uf0e8"
	glyphQuestion  = "\uf059"
	glyphPrototype = "\uf1fc" // paint brush
)

// g prefixes text with a glyph and a space, or returns the text alone.
func (g glyphs) g(glyph, text string) string {
	if !g.on || glyph == "" {
		return text
	}
	return glyph + " " + text
}
