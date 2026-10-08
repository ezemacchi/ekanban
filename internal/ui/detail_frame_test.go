package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Opening the detail box and closing it must not change how many lines the
// board draws. When the frame was one line taller with the box open, closing
// it made the terminal erase the board's bottom line, the row of key hints.
func TestDetailBoxKeepsTheFrameTheSameHeight(t *testing.T) {
	m := pipelineBoard(t)
	m.layout = layoutKanban
	m.width, m.height = 160, 52
	lines := func() []string { return strings.Split(ansi.Strip(m.View()), "\n") }

	before := lines()
	send(t, m, key("d"))
	if m.mode != modeDetail {
		t.Fatal("d did not open the detail box")
	}
	open := lines()
	send(t, m, key("esc"))
	after := lines()

	if len(open) != len(before) || len(after) != len(before) {
		t.Fatalf("frame heights differ: board %d, box open %d, closed again %d", len(before), len(open), len(after))
	}
	if last := after[len(after)-1]; !strings.Contains(last, "Detail") || !strings.Contains(last, "Help") {
		t.Fatalf("the hints are not the last row after closing: %q", last)
	}
}
