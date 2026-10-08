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
