package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/look"
)

func shortFacts() []fact {
	return []fact{
		{glyph: look.Jira, text: "Jira: In Review", style: dimStyle, weight: weightContext, short: said(look.Jira, "In Review", dimStyle)},
		{glyph: look.PullReq, text: "PR #180 · CI green", style: prPassStyle, weight: weightState, short: said(look.PullReq, "#180", prPassStyle)},
		{glyph: look.Check, text: "ready to merge", style: prPassStyle, weight: weightState, short: said(look.Check, "mergeable", prPassStyle)},
	}
}

// With glyphs, a card at rest says its heaviest fact short and the others as
// badges; without them it says the heaviest fact in words, as it did.
func TestStatusLineShortWithGlyphs(t *testing.T) {
	m := &Model{icons: true}
	got := ansi.Strip(m.statusLine(shortFacts(), 30))
	if !strings.HasPrefix(got, look.PullReq+" #180") || !strings.HasSuffix(got, look.Jira+" "+look.Check) {
		t.Fatalf("status line %q", got)
	}
	if strings.Contains(got, "CI green") || strings.Contains(got, "Jira:") {
		t.Fatalf("the glyphs did not replace the words: %q", got)
	}
	m.icons = false
	if got := ansi.Strip(m.statusLine(shortFacts(), 30)); got != "PR #180 · CI green" {
		t.Fatalf("without glyphs: %q", got)
	}
}

// Badges give way, the lightest first, before the heaviest fact is cut too
// short to read.
func TestStatusLineDropsBadgesThatDoNotFit(t *testing.T) {
	m := &Model{icons: true}
	got := ansi.Strip(m.statusLine(shortFacts(), minFactWidth+2))
	if strings.Contains(got, look.Jira) || !strings.Contains(got, look.Check) {
		t.Fatalf("the lightest badge did not go first: %q", got)
	}
	got = ansi.Strip(m.statusLine(shortFacts(), minFactWidth))
	if strings.Contains(got, look.Check) || !strings.HasPrefix(got, look.PullReq+" #180") {
		t.Fatalf("badges kept with no room: %q", got)
	}
}

// The selected card spells each fact out, short with glyphs.
func TestCardLineUsesTheShortForm(t *testing.T) {
	f := shortFacts()[0]
	if got := ansi.Strip((&Model{icons: true}).cardLine(f, 40)); got != look.Jira+" In Review" {
		t.Fatalf("with glyphs %q", got)
	}
	if got := ansi.Strip((&Model{}).cardLine(f, 40)); got != "Jira: In Review" {
		t.Fatalf("without glyphs %q", got)
	}
}
