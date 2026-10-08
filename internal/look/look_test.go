package look

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTruncateMeasuresWidth(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "h"},
		{"hello", 0, ""},
		// Wide characters take two cells; cutting by rune count would overrun.
		{"日本語テキスト", 5, "日本…"},
	}
	for _, c := range cases {
		got := Truncate(c.in, c.n)
		if got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
		if c.n > 0 && lipgloss.Width(got) > c.n {
			t.Errorf("Truncate(%q, %d) is %d cells wide", c.in, c.n, lipgloss.Width(got))
		}
	}
}

func TestIconsOffShowsTextOnly(t *testing.T) {
	if got := (Icons{}).With(Check, "done"); got != "done" {
		t.Fatalf("off: %q", got)
	}
	if got := (Icons{On: true}).With(Check, "done"); got != Check+" done" {
		t.Fatalf("on: %q", got)
	}
	if got := (Icons{On: true}).With("", "done"); got != "done" {
		t.Fatalf("no glyph: %q", got)
	}
}

func TestSpinnerStopsWhenIdle(t *testing.T) {
	s := NewSpinner()
	if s.Start() == nil {
		t.Fatal("first Start must tick")
	}
	if s.Start() != nil {
		t.Fatal("a running spinner must not start a second clock")
	}
	first := s.Frame()
	if s.Update(SpinMsg{id: s.id}, true) == nil || s.Frame() == first {
		t.Fatal("busy spinner must advance and keep ticking")
	}
	if s.Update(SpinMsg{id: s.id}, false) != nil {
		t.Fatal("idle spinner must stop")
	}
	if s.Update(SpinMsg{id: s.id + 99}, true) != nil {
		t.Fatal("another spinner's tick must be ignored")
	}
}
