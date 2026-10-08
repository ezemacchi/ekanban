package ui

import (
	"strings"

	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/store"
)

// statusGlyph is the manual board's icon set; computed columns carry their
// own icon. The glyphs are in look.
var statusGlyph = map[string]string{
	"triage":      look.Funnel,
	"todo":        look.Circle,
	"in_progress": look.Cog,
	"waiting":     look.Question,
	"done":        look.Check,
}

// glyph prefixes text with a Nerd Font glyph when icons are on.
func (m *Model) glyph(g, text string) string { return look.Icons{On: m.icons}.With(g, text) }

// SetIcons turns Nerd Font glyphs on or off.
func (m *Model) SetIcons(on bool) { m.icons = on }

func (m *Model) statusLabel(st store.Status) string {
	if c, ok := m.columns.Find(st.ID); ok && c.Icon != "" {
		return m.glyph(c.Icon, st.Label)
	}
	g, ok := statusGlyph[st.ID]
	if !ok {
		g = look.Tag
	}
	return m.glyph(g, st.Label)
}

// ticketName is a ticket worktree's name on the pipeline board: its key, and
// when another worktree holds the same ticket, what its folder adds after the
// key ("ABC-1 dialog" for App-wt-ABC-1-dialog).
func (m *Model) ticketName(space, key string) string {
	shared := false
	for other, info := range m.pipeInfo {
		if other != space && info.Key == key {
			shared = true
			break
		}
	}
	if !shared {
		return key
	}
	folder := baseName(space)
	i := strings.Index(strings.ToUpper(folder), strings.ToUpper(key))
	if i < 0 {
		return key + " " + folder
	}
	if rest := strings.Trim(folder[i+len(key):], "-_. "); rest != "" {
		return key + " " + rest
	}
	return key
}

// spaceName is what a message calls a space: its ticket on the pipeline
// board, else its label, else its folder.
func (m *Model) spaceName(sp *space) string {
	if info, ok := m.pipeInfo[sp.Key]; ok && m.pipelineOn() && info.Key != "" {
		return m.ticketName(sp.Key, info.Key)
	}
	if sp.Label != "" {
		return sp.Label
	}
	return baseName(sp.Key)
}

// spaceLabel prefixes a space's name with what it is: a git worktree on a
// branch, or a plain folder. On the pipeline board a ticket worktree is
// named by its ticket key: the folder names of one repository's worktrees
// share a long prefix, so cut to a card's width they all read the same.
func (m *Model) spaceLabel(sp *space) string {
	if info, ok := m.pipeInfo[sp.Key]; ok && m.pipelineOn() && info.Key != "" {
		return m.glyph(look.Branch, m.ticketName(sp.Key, info.Key))
	}
	if m.branchFor(sp.Key) != "" {
		return m.glyph(look.Branch, sp.Label)
	}
	return m.glyph(look.Folder, sp.Label)
}
