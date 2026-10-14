package screen

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/look"
)

func plainLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func stacked() Card {
	return Card{
		Title: "ABC-1", Body: []string{"details"},
		Stack:   []StackLine{{Key: "ABC-10", Word: "failed", Attention: true}, {Key: "ABC-11", Word: "blocked"}},
		StackAt: -1,
	}
}

// The strips share the card's borders and every line is as wide as the card.
func TestStackDrawsOneStripPerRelatedTicket(t *testing.T) {
	lines, _ := stacked().Render(28)
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 28 {
			t.Fatalf("line %d is %d wide: %q", i, w, ansi.Strip(l))
		}
	}
	got := plainLines(lines)
	want := []string{"╭─ ABC-1 ", "│ details", "├", "│   ABC-10 failed", "├", "│   ABC-11 blocked", "╰"}
	if len(got) != len(want) {
		t.Fatalf("%d lines, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i, w := range want {
		if !strings.HasPrefix(got[i], w) {
			t.Errorf("line %d = %q, want it to start %q", i, got[i], w)
		}
	}
}

// A card with no stack draws exactly as it did before strips existed.
func TestNoStackDrawsAsBefore(t *testing.T) {
	c := stacked()
	c.Stack, c.StackAt = nil, 0
	lines, _ := c.Render(28)
	got := plainLines(lines)
	if len(got) != 3 || !strings.HasPrefix(got[2], "╰") || strings.Contains(strings.Join(got, ""), "├") {
		t.Fatalf("drew %q", got)
	}
}

// The cursor strip is marked, and the card's own box stops reading as the
// selected one.
func TestStackCursorMarksTheStripUnderIt(t *testing.T) {
	c := stacked()
	c.Selected, c.StackAt = true, 1
	lines, _ := c.Render(28)
	got := plainLines(lines)
	if !strings.Contains(got[3], "  ABC-10") || strings.Contains(got[3], "❯") {
		t.Errorf("first strip %q", got[3])
	}
	if !strings.Contains(got[5], "❯ ABC-11") {
		t.Errorf("second strip %q", got[5])
	}
	onCard := c
	onCard.StackAt = -1
	a, _ := onCard.Render(28)
	if strings.Contains(strings.Join(plainLines(a), ""), "❯") {
		t.Error("a strip is marked while the cursor is on the card")
	}
}

// Colour is checked at its source: a test run has no terminal, so lipgloss
// draws none. The cursor's row is the selected colour, a failed strip asks
// for you, and the card's box is no longer the selected one while a strip is.
func TestStackBordersFollowTheCursor(t *testing.T) {
	c := stacked()
	c.Selected, c.StackAt = true, 1
	head := c
	head.Selected = false
	if got := c.stackBorder(head, 1); got != look.CardSelected {
		t.Errorf("cursor strip border %v", got)
	}
	if got := c.stackBorder(head, 0); got != look.CardAttention {
		t.Errorf("failed strip border %v", got)
	}
	if got := c.stackBorder(head, -1); got != look.CardBorder {
		t.Errorf("the card's box with the cursor on a strip: %v", got)
	}
	c.StackAt = -1
	if got := c.stackBorder(c, -1); got != look.CardSelected {
		t.Errorf("the card's box with the cursor on it: %v", got)
	}
}

// A click lands on the strip under it, by index, on top of the card's zone.
func TestStackStripsAreZones(t *testing.T) {
	lines, zones := stacked().Render(28)
	var z Zones
	for _, zone := range zones {
		z.Add(zone)
	}
	for i, y := range []int{3, 5} {
		hit, ok := z.At(5, y)
		if !ok || hit.Kind != OnStack || hit.Choice != i {
			t.Fatalf("row %d: %+v, %v", y, hit, ok)
		}
	}
	if hit, ok := z.At(5, 1); !ok || hit.Kind != OnCard {
		t.Fatalf("the card's own line is %+v", hit)
	}
	if hit, ok := z.At(5, 2); !ok || hit.Kind == OnStack {
		t.Fatalf("the joint is %+v", hit)
	}
	if len(lines) != 7 {
		t.Fatalf("%d lines", len(lines))
	}
}

func TestStackSaysWhatWasLeftOutAndWhereEachKeyGoes(t *testing.T) {
	c := stacked()
	c.Selected, c.StackMore, c.Cue = true, 3, "↑ ABC-1 · ↓ ABC-11"
	got := plainLines(func() []string { l, _ := c.Render(28); return l }())
	text := strings.Join(got, "\n")
	if !strings.Contains(text, "+3 more") || !strings.Contains(text, "↑ ABC-1 · ↓ ABC-11") {
		t.Fatalf("drew:\n%s", text)
	}
	if !strings.HasPrefix(got[len(got)-1], "╰") {
		t.Fatalf("the last line is %q", got[len(got)-1])
	}
	// The cue is for the selected card: a card at rest does not carry it.
	c.Selected = false
	l, _ := c.Render(28)
	if strings.Contains(strings.Join(plainLines(l), ""), "ABC-1 · ") {
		t.Error("an unselected card drew the cue")
	}
}
