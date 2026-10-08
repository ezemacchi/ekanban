package screen

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Scrolling moves zones up and drops those that leave the visible rows, but
// leaves the zones before from alone.
func TestScrollShiftsAndDrops(t *testing.T) {
	z := Zones{{Y: 0, Key: "header"}, {Y: 3, Key: "a"}, {Y: 8, Key: "b"}, {Y: 20, Key: "c"}}
	z.Scroll(1, 5, 2, 10)
	var keys []string
	for _, zone := range z {
		keys = append(keys, zone.Key)
	}
	if got := strings.Join(keys, ","); got != "header,b" {
		t.Fatalf("kept %s, want header,b", got)
	}
	if z[1].Y != 3 {
		t.Fatalf("b at row %d, want 3", z[1].Y)
	}
}

// Buttons stop at the width rather than wrap, skip unbound keys, and land on
// their row once the footer is placed.
func TestButtonsFitAndPlace(t *testing.T) {
	var z Zones
	out := Buttons(&z, 1, 1, []Hint{{Key: "a", Label: "alpha"}, {Key: "", Label: "unbound"}, {Key: "b", Label: "beta"}, {Key: "c", Label: "gamma"}}, 18)
	if w := lipgloss.Width(out); w > 18 {
		t.Fatalf("buttons %d wide in 18", w)
	}
	if strings.Contains(out, "unbound") || strings.Contains(out, "gamma") {
		t.Fatalf("drew %q", out)
	}
	z.PlaceFooter(10)
	if len(z) != 2 || z[0].Y != 11 || z[1].Key != "b" {
		t.Fatalf("zones %+v", z)
	}
	if hit, ok := z.At(z[1].X0, 11); !ok || hit.Key != "b" {
		t.Fatalf("no b button under its own start: %+v", hit)
	}
}

// An empty column takes only its header; the ones with cards share the rest.
func TestWidthsShrinkEmptyColumns(t *testing.T) {
	cols := []Column{{Label: "Empty"}, {Label: "Full", Count: 2}, {Label: "Gone", Count: 1, Hidden: true}}
	w := Widths(cols, 80)
	if w[0] != lipgloss.Width(cols[0].Header())+Gutter {
		t.Fatalf("empty column %d wide", w[0])
	}
	if w[1] <= w[0] || w[1] > MaxColumnWidth {
		t.Fatalf("full column %d wide", w[1])
	}
	if w[2] != 0 {
		t.Fatalf("hidden column %d wide", w[2])
	}
}
