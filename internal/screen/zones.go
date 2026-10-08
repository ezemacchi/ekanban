// Package screen holds what both boards draw the same way: kanban columns of
// cards, the footer's buttons, clickable zones, and text that fits a width.
// A board decides what its cards say; this decides how a board looks.
package screen

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Kind is what a click on a zone means.
type Kind int

const (
	OnCard   Kind = iota // a card: Col, Row
	OnColumn             // a column header: Col
	OnChoice             // a choice inside an expanded card or a modal: Choice
	OnButton             // a key hint or a clickable line: Key
	OnBox                // a modal's box: a click inside it lands on nothing
)

// Zone is a rectangle of the screen a click can act on. Every frame records
// where it drew them, so a click is tested against what is on screen now.
type Zone struct {
	Kind     Kind
	Y        int
	X0, X1   int // X1 exclusive
	H        int // rows covered, 1 when 0
	Col, Row int
	Choice   int
	Key      string
	// Footer means Y counts from the footer's first line until PlaceFooter.
	Footer bool
}

// Zones are a frame's zones. Later ones sit on top: a modal's win over the
// board under it.
type Zones []Zone

func (z *Zones) Reset() { *z = (*z)[:0] }

func (z *Zones) Add(zone Zone) { *z = append(*z, zone) }

// PlaceFooter turns the footer's zones into screen rows once the rows above
// the footer are known.
func (z Zones) PlaceFooter(top int) {
	for i := range z {
		if z[i].Footer {
			z[i].Y += top
			z[i].Footer = false
		}
	}
}

// At is the topmost zone under a point.
func (z Zones) At(x, y int) (Zone, bool) {
	for i := len(z) - 1; i >= 0; i-- {
		h := max(z[i].H, 1)
		if y >= z[i].Y && y < z[i].Y+h && x >= z[i].X0 && x < z[i].X1 {
			return z[i], true
		}
	}
	return Zone{}, false
}

// Scroll moves the zones from index from up by dy rows and drops those that
// end up outside rows [top, bottom).
func (z *Zones) Scroll(from, dy, top, bottom int) {
	kept := (*z)[:from]
	for _, zone := range (*z)[from:] {
		zone.Y -= dy
		if zone.Y >= top && zone.Y < bottom {
			kept = append(kept, zone)
		}
	}
	*z = kept
}

// LinesIn is how many rows s takes before its last line.
func LinesIn(s string) int { return strings.Count(s, "\n") }

// KeyMsg is the key event a button standing for k sends.
func KeyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "space", " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
