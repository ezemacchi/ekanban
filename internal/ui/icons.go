package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/phin-tech/herdr-phin-board/internal/store"
)

// Nerd Font glyphs, drawn only when the icons setting is on.
var statusGlyph = map[string]string{
	"triage":      "\uf0b0", // funnel
	"todo":        "\uf10c", // empty circle
	"in_progress": "\uf013", // cog
	"waiting":     "\uf059", // question
	"done":        "\uf00c", // check
}

const (
	customStatusGlyph = "\uf02b" // tag, for statuses the user invented
	worktreeGlyph     = "\ue725" // git branch
	folderGlyph       = "\uf07b"
	noteGlyph         = "\uf040" // pencil
	agentGlyphIcon    = "\uf108" // monitor
	clockGlyph        = "\uf017"
	// Powerline half circles: the rounded ends of the modal's title pill.
	pillLeft  = "\ue0b6"
	pillRight = "\ue0b4"
)

var pillColor = lipgloss.Color("62")

// glyph prefixes text with a Nerd Font glyph when icons are on.
func (m *Model) glyph(g, text string) string {
	if !m.icons {
		return text
	}
	if text == "" {
		return g + " "
	}
	return g + " " + text
}

// SetIcons turns Nerd Font glyphs on or off.
func (m *Model) SetIcons(on bool) { m.icons = on }

func (m *Model) statusLabel(st store.Status) string {
	if !m.icons {
		return st.Label
	}
	g, ok := statusGlyph[st.ID]
	if !ok {
		g = customStatusGlyph
	}
	return g + " " + st.Label
}

// spaceLabel prefixes a space's name with what it is: a git worktree on a
// branch, or a plain folder.
func (m *Model) spaceLabel(sp *space) string {
	if !m.icons {
		return sp.Label
	}
	if m.branchFor(sp.Key) != "" {
		return worktreeGlyph + " " + sp.Label
	}
	return folderGlyph + " " + sp.Label
}
