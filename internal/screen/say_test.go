package screen

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/look"
)

// Say draws a marked key in the key colour, the rest in the base style, and
// cuts the whole to width.
func TestSayColoursKeysAndCuts(t *testing.T) {
	msg := "no note — press " + Key("n") + " to add one"
	out := Say(msg, look.Dim, 0)
	if got := ansi.Strip(out); got != "no note — press n to add one" {
		t.Fatalf("text %q", got)
	}
	if !strings.Contains(out, look.Key.Render("n")) {
		t.Fatalf("the key is not in the key style: %q", out)
	}
	if w := lipgloss.Width(Say(msg, look.Dim, 12)); w > 12 {
		t.Fatalf("%d wide in 12", w)
	}
	if Key("") != "" || Plain(msg) != "no note — press n to add one" {
		t.Fatal("Key or Plain")
	}
}
