// Package look is what both boards share about how they draw: the styles,
// the Nerd Font glyphs, text fitting, opening links, and the spinner. A board
// package holds only what is its own.
package look

import (
	"os/exec"
	"runtime"

	"github.com/charmbracelet/lipgloss"
)

// Styles.
var (
	Title  = lipgloss.NewStyle().Bold(true)
	Dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	Cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	Err    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	Key    = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
)

// Nerd Font glyphs. Draw them through Icons, so they disappear when the
// terminal has no Nerd Font.
const (
	Circle    = "\uf10c" // to do, pending
	Cog       = "\uf013" // working
	Question  = "\uf059" // waiting on someone
	Check     = "\uf00c" // done
	Funnel    = "\uf0b0" // triage
	Tag       = "\uf02b" // a status the user invented
	Branch    = "\ue725" // git branch, worktree
	Folder    = "\uf07b"
	Pencil    = "\uf040" // note
	Monitor   = "\uf108" // agent
	Clock     = "\uf017"
	Jira      = "\ue75a"
	Target    = "\uf419" // merge target
	Play      = "\uf04b" // current step
	Rocket    = "\uf135" // landed, published
	Warning   = "\uf071" // stuck, lost
	Sitemap   = "\uf0e8" // orchestrator
	Brush     = "\uf1fc" // prototype
	Team      = "\uf0c0" // phase of the team
	PullReq   = "\uf407"
	Wifi      = "\uf1eb" // partial data, offline source
	Archive   = "\uf187"
	Pause     = "\uf04c" // workspace closed
	PillLeft  = "\ue0b6" // rounded ends of a title pill
	PillRight = "\ue0b4"
)

// PillColor is the background of title pills and the modal border.
var PillColor = lipgloss.Color("62")

// Icons draws glyphs when On.
type Icons struct{ On bool }

// With prefixes text with glyph and a space. Off, or with no glyph, it is the
// text alone.
func (i Icons) With(glyph, text string) string {
	if !i.On || glyph == "" {
		return text
	}
	if text == "" {
		return glyph + " "
	}
	return glyph + " " + text
}

// Truncate fits s in n cells, ending in "…" when cut. It measures display
// width, so wide characters and glyphs are not overrun. One cell has no room
// for the ellipsis and keeps the first character.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	r := []rune(s)
	if n == 1 {
		return string(r[:1])
	}
	for len(r) > 0 && lipgloss.Width(string(r)) > n-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// OpenURL opens url in the default browser without waiting for it.
func OpenURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}
