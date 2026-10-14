package screen

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// What a pane prints is drawn inside the board, so only its colours pass:
// cursor moves, clears, titles and bells go, and so do trailing blank lines.
func TestSafeLinesKeepOnlyColours(t *testing.T) {
	got := SafeLines("\x1b[2J\x1b[H\x1b]0;title\x07\x1b[31mred\x1b[0m\tend\a\r\n\x1b[1;5Hmoved\n\n  \n")
	if len(got) != 2 {
		t.Fatalf("lines %q", got)
	}
	if got[0] != "\x1b[31mred\x1b[0m    end" || got[1] != "moved" {
		t.Fatalf("lines %q", got)
	}
}

// The chooser lists its rows beside the preview when there is room, an
// unusable row dim with why, and records a choice zone for each row.
func TestChooserDrawsRowsAndPreview(t *testing.T) {
	c := Chooser{
		Title:   "Which agent?",
		Rows:    []ChooserRow{{Text: "tab orchestrator"}, {Text: "tab 3", Off: true, Why: "waiting at a prompt"}},
		Preview: []string{"first line", "\x1b[32mlast line\x1b[0m"},
	}
	var z Zones
	out := ansi.Strip(c.Over(&z, strings.Repeat("\n", 40), 140, 40))
	for _, want := range []string{"Which agent?", "❯ tab orchestrator", "tab 3 · waiting at a prompt", "│ first line", "last line", "Use", "Cancel"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	choices := 0
	for _, zone := range z {
		if zone.Kind == OnChoice {
			choices++
		}
	}
	if choices != 2 {
		t.Fatalf("%d choice zones, want 2", choices)
	}
}
