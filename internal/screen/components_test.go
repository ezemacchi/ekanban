package screen

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/look"
)

// Buttons stay within the width, skip unbound keys, and a button that does
// not fit gives way to a shorter one after it. The footer's land on their
// row once it is placed.
func TestButtonsFitAndPlace(t *testing.T) {
	var z Zones
	hints := []Hint{{Key: "a", Label: "alpha"}, {Key: "", Label: "unbound"}, {Key: "b", Label: "a long beta"}, {Key: "c", Label: "g"}}
	out := Buttons(&z, 1, 1, hints, 18)
	if w := lipgloss.Width(out); w > 18 {
		t.Fatalf("buttons %d wide in 18", w)
	}
	plain := ansi.Strip(out)
	if strings.Contains(plain, "unbound") || strings.Contains(plain, "beta") || !strings.Contains(plain, "c g") {
		t.Fatalf("drew %q", plain)
	}
	if len(z) != 2 || z[0].Key != "a" || z[1].Key != "c" {
		t.Fatalf("zones %+v", z)
	}

	var f Zones
	Footer(&f, "state", []Hint{{Key: "a", Label: "alpha"}}, []Item{Button(Hint{Key: "?", Label: "Help"})}, 40)
	f.PlaceFooter(10)
	help, ok := f.At(37, 11)
	if !ok || help.Key != "?" {
		t.Fatalf("no help button at the right end of the footer: %+v", f)
	}
	if hit, ok := f.At(f[0].X0, 11); !ok || hit.Key != "a" {
		t.Fatalf("no a button under its own start: %+v", hit)
	}
}

// A card is exactly its width, its title sits in the top border, and the
// selected card's last button (its menu) survives a narrow card.
func TestCardDrawsItsTitleAndKeepsItsMenu(t *testing.T) {
	c := Card{
		Title: "ABC-1", Badge: "Story", Body: []string{"a styled line that is far too long for the card"},
		Selected: true,
		Actions:  []Hint{{Key: "enter", Label: "Go"}, {Key: "t", Label: "Ticket"}, {Key: "P", Label: "PR"}, {Key: ".", Label: "⋯"}},
	}
	lines, zones := c.Render(24)
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 24 {
			t.Fatalf("line %d is %d wide: %q", i, w, ansi.Strip(l))
		}
	}
	if top := ansi.Strip(lines[0]); !strings.Contains(top, "ABC-1") || !strings.Contains(top, "Story") {
		t.Fatalf("top border %q", top)
	}
	var keys []string
	for _, z := range zones {
		if z.Kind == OnButton {
			keys = append(keys, z.Key)
		}
	}
	if len(keys) == 0 || keys[len(keys)-1] != "." {
		t.Fatalf("the menu button gave way: %v", keys)
	}

	rest := Card{Title: "ABC-1", Actions: c.Actions}
	if lines, _ := rest.Render(24); len(lines) != 2 {
		t.Fatalf("a card at rest drew its buttons: %d lines", len(lines))
	}
	flat := Card{Title: "Reviewer", Flat: true, Body: []string{"2 rounds"}}
	if lines, _ := flat.Render(24); len(lines) != 1 || !strings.Contains(ansi.Strip(lines[0]), "Reviewer · 2 rounds") {
		t.Fatalf("flat card %q", lines)
	}
}

// A choice inside a card is on top of the card's own zone, so a click picks
// it rather than the card.
func TestCardChoicesAreOnTop(t *testing.T) {
	c := Card{Title: "x", Body: []string{"caption", "one", "two"}, Choices: 1, NChoices: 2}
	_, zones := c.Render(20)
	var z Zones = zones
	hit, ok := z.At(3, 3) // border, caption, one, two
	if !ok || hit.Kind != OnChoice || hit.Choice != 1 {
		t.Fatalf("the click on the second choice landed on %+v", hit)
	}
}

// A folded column is its header alone, narrow, with the arrow that opens it.
func TestFoldedColumnIsItsHeader(t *testing.T) {
	cols := []Column{
		{Label: "Open", Count: 1, Cards: []Card{{Title: "a"}}, Selected: -1},
		{Label: "Shut", Count: 2, Folded: true, Cards: []Card{{Title: "b"}, {Title: "c"}}, Selected: -1},
	}
	w := Widths(cols, 80)
	if w[1] != lipgloss.Width(cols[1].Header())+Gutter || !strings.HasPrefix(cols[1].Header(), "▸") {
		t.Fatalf("folded column %d wide, header %q", w[1], cols[1].Header())
	}
	var z Zones
	out := ansi.Strip(Draw(&z, cols, w, 0, 2, 0, Tallest(cols)))
	if strings.Contains(out, " b ") || !strings.Contains(out, " a ") {
		t.Fatalf("drew\n%s", out)
	}
	folds := 0
	for _, zone := range z {
		if zone.Kind == OnControl && zone.ID == "fold" {
			folds++
		}
	}
	if folds != 2 {
		t.Fatalf("%d fold arrows, want one per column with cards", folds)
	}
}

// A menu over a frame records a choice per item on its own row, inside a box
// that keeps clicks on it from reaching the board.
func TestMenuRecordsItsChoices(t *testing.T) {
	mn := Menu{Title: "ABC-1", Items: []MenuItem{{Label: "Go", Key: "enter"}, {Label: "Copy", Key: "y"}}}
	var z Zones
	frame := strings.Repeat(strings.Repeat(" ", 40)+"\n", 10)
	out := mn.Over(&z, frame, 5, 2, 40, 10)
	if !strings.Contains(ansi.Strip(out), "Copy") {
		t.Fatalf("menu not drawn:\n%s", ansi.Strip(out))
	}
	if z[0].Kind != OnBox || z[1].Choice != 0 || z[2].Choice != 1 || z[2].Y != z[1].Y+1 {
		t.Fatalf("zones %+v", z)
	}
	mn.Move(-1)
	if mn.Sel != 1 {
		t.Fatalf("moving up from the first did not wrap: %d", mn.Sel)
	}
	if chosen, done := mn.Key("enter"); !done || chosen != 1 {
		t.Fatalf("enter chose %d", chosen)
	}
}

// The search field says what can be searched, shows a query with a ✕ that
// clears it, and is clickable either way.
func TestSearchField(t *testing.T) {
	empty := Search("", "Search tickets", "/", false, "", 30)
	if len(empty) != 1 || !strings.Contains(ansi.Strip(empty[0].Text), "/ Search tickets") || empty[0].On.ID != "search" {
		t.Fatalf("empty field %+v", empty)
	}
	if w := lipgloss.Width(empty[0].Text); w != 30 {
		t.Fatalf("field %d wide, want 30", w)
	}
	full := Search("abc", "Search tickets", "/", false, "", 30)
	if len(full) != 2 || full[1].On.ID != "clear-search" {
		t.Fatalf("a query has no ✕: %+v", full)
	}
}

// Ends puts the right items flush with the right edge.
func TestEndsAlignsRight(t *testing.T) {
	var z Zones
	line := Ends(&z, 0, 1, []Item{Text("left")}, []Item{Control("right", "r")}, 30)
	if w := lipgloss.Width(line); w != 30 || !strings.HasSuffix(line, "right") {
		t.Fatalf("%d wide: %q", w, line)
	}
	if z[0].X1 != 31 {
		t.Fatalf("the right item ends at %d, want 31", z[0].X1)
	}
}

// Inline draws **bold** and `code` without their marks.
func TestInlineDropsTheMarks(t *testing.T) {
	out := Inline("a **bold** and `code` line", lipgloss.NewStyle(), 0)
	if got := ansi.Strip(out); got != "a bold and code line" {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(out, look.Key.Render("code")) {
		t.Fatalf("code is not in the key colour: %q", out)
	}
	if w := lipgloss.Width(Inline("**open bold that runs on", lipgloss.NewStyle(), 10)); w > 10 {
		t.Fatalf("%d wide in 10", w)
	}
}

// A field's label is in a column of its own, and the lines after the first
// line up under the first.
func TestFieldLinesUp(t *testing.T) {
	lines := Field("pull req", []string{"PR #5", "green"})
	if got := ansi.Strip(lines[0]); got != "PULL REQ   PR #5" {
		t.Fatalf("%q", got)
	}
	if got := ansi.Strip(lines[1]); got != strings.Repeat(" ", FieldLabel)+"green" {
		t.Fatalf("%q", got)
	}
}

// A column taller than its room says how many cards are cut off, above and
// below, rather than just stopping.
func TestColumnCountsWhatItCutsOff(t *testing.T) {
	var cards []Card
	for range 8 {
		cards = append(cards, Card{Title: "c", Body: []string{"x"}}) // 3 lines each
	}
	col := Column{Label: "Many", Count: 8, Cards: cards, Selected: 0}
	lines, _ := col.Render(0, 20, 11) // header, rule, three cards
	if got := ansi.Strip(lines[len(lines)-1]); !strings.Contains(got, "↓ 5 more") {
		t.Fatalf("last line %q", got)
	}
	col.Selected = 7
	lines, zones := col.Render(0, 20, 11)
	if got := ansi.Strip(lines[1]); !strings.Contains(got, "↑ 5 more") {
		t.Fatalf("rule %q", got)
	}
	for _, z := range zones {
		if z.Y >= len(lines) {
			t.Fatalf("a zone below the column: %+v", z)
		}
	}
}
