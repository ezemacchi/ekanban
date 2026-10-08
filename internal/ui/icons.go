package ui

import (
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/store"
)

// statusGlyph is the global board's own icon set; the glyphs are in look.
var statusGlyph = map[string]string{
	"triage":      look.Funnel,
	"todo":        look.Circle,
	"in_progress": look.Cog,
	"waiting":     look.Question,
	"done":        look.Check,
	"on_review":   look.PullReq,
	"to_deploy":   look.Rocket,
	"ready_qa":    look.Check,
}

// glyph prefixes text with a Nerd Font glyph when icons are on.
func (m *Model) glyph(g, text string) string { return look.Icons{On: m.icons}.With(g, text) }

// SetIcons turns Nerd Font glyphs on or off.
func (m *Model) SetIcons(on bool) { m.icons = on }

func (m *Model) statusLabel(st store.Status) string {
	g, ok := statusGlyph[st.ID]
	if !ok {
		g = look.Tag
	}
	return m.glyph(g, st.Label)
}

// spaceLabel prefixes a space's name with what it is: a git worktree on a
// branch, or a plain folder.
func (m *Model) spaceLabel(sp *space) string {
	if m.branchFor(sp.Key) != "" {
		return m.glyph(look.Branch, sp.Label)
	}
	return m.glyph(look.Folder, sp.Label)
}
