package keys

import "testing"

var screen = []Action{
	{Name: "accept", Keys: []string{"a"}},
	{Name: "archive", Keys: []string{"A"}},
	{Name: "down", Keys: []string{"j", "down"}},
	{Name: "open-pr", Keys: []string{"gp"}, Fixed: true},
}

func TestDefaultsPassThrough(t *testing.T) {
	m, problems := New(screen, nil)
	for pressed, want := range map[string]string{"a": "a", "down": "j", "x": "x"} {
		if got := m.Resolve(pressed); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", pressed, got, want)
		}
	}
	if len(problems) != 0 {
		t.Fatal(problems)
	}
}

func TestReboundKeyActsAsTheDefaultAndTheOldKeyStops(t *testing.T) {
	m, _ := New(screen, map[string][]string{"accept": {"y"}})
	if got := m.Resolve("y"); got != "a" {
		t.Fatalf("y resolves to %q, want a", got)
	}
	if got := m.Resolve("a"); got != "" {
		t.Fatalf("a still resolves to %q after accept moved to y", got)
	}
	if m.Key("accept") != "y" {
		t.Fatalf("Key(accept) = %q", m.Key("accept"))
	}
}

func TestSwappingTwoActions(t *testing.T) {
	m, problems := New(screen, map[string][]string{"accept": {"A"}, "archive": {"a"}})
	if m.Resolve("A") != "a" || m.Resolve("a") != "A" || len(problems) != 0 {
		t.Fatalf("A->%q a->%q %v", m.Resolve("A"), m.Resolve("a"), problems)
	}
}

func TestConflictsAndFixedAreReported(t *testing.T) {
	_, problems := New(screen, map[string][]string{"accept": {"j"}})
	if len(problems) != 1 {
		t.Fatalf("a key on two actions should be reported: %v", problems)
	}
	m, _ := New(screen, map[string][]string{"open-pr": {"p"}})
	if m.Resolve("p") != "p" || m.Key("open-pr") != "gp" {
		t.Fatal("a fixed action must not be rebound")
	}
}

func TestUnknownNames(t *testing.T) {
	got := Unknown(map[string][]string{"accept": {"y"}, "nope": {"z"}}, screen)
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestDisplay(t *testing.T) {
	if got := Display([]string{"j", " "}); got != "j / space" {
		t.Fatalf("got %q", got)
	}
}

// ticket is a screen with actions of its own on a selected question.
var ticket = []Action{
	{Name: "refresh", Keys: []string{"r"}},
	{Name: "down", Keys: []string{"j", "down"}},
	{Name: "reply", Keys: []string{"r"}, Scope: "question"},
	{Name: "discuss", Keys: []string{"D"}, Scope: "question"},
	{Name: "go", Keys: []string{"enter"}, Scope: "role"},
	{Name: "open-link", Keys: []string{"enter"}, Scope: "question"},
}

func TestAScopedKeyWinsOnlyInItsScope(t *testing.T) {
	m, problems := New(ticket, nil)
	if len(problems) != 0 {
		t.Fatalf("a scope sharing a board-wide key is no conflict: %v", problems)
	}
	for _, c := range []struct {
		pressed string
		scopes  []string
		want    string
	}{
		{"r", nil, "r"},
		{"r", []string{"role"}, "r"},
		{"r", []string{"question"}, "reply"},
		{"j", []string{"question"}, "j"},
		{"enter", []string{"role"}, "go"},
		{"enter", []string{"question"}, "open-link"},
		{"D", nil, "D"}, // out of its scope a key is nobody's
	} {
		if got := m.Resolve(c.pressed, c.scopes...); got != c.want {
			t.Errorf("Resolve(%q, %v) = %q, want %q", c.pressed, c.scopes, got, c.want)
		}
	}
}

func TestHintsFollowTheScope(t *testing.T) {
	m, _ := New(ticket, nil)
	if m.KeyIn("refresh") != "r" || m.KeyIn("refresh", "question") != "" {
		t.Fatalf("refresh: %q board-wide, %q on a question", m.KeyIn("refresh"), m.KeyIn("refresh", "question"))
	}
	if m.KeyIn("reply", "question") != "r" || m.KeyIn("reply") != "" {
		t.Fatal("reply's key belongs to a question only")
	}
	if got := m.Scopes(); len(got) != 2 || got[0] != "question" || got[1] != "role" {
		t.Fatalf("Scopes() = %v", got)
	}
}

func TestBindingAScopedActionByName(t *testing.T) {
	m, problems := New(ticket, map[string][]string{"reply": {"x"}})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	if m.Resolve("x", "question") != "reply" || m.Resolve("r", "question") != "r" || m.Resolve("x") != "x" {
		t.Fatal("reply moved to x: x replies on a question, r refreshes there again")
	}
}

func TestConflictsAreWithinOneScope(t *testing.T) {
	_, problems := New(ticket, map[string][]string{"discuss": {"r"}})
	if len(problems) != 1 {
		t.Fatalf("two question actions on r should be reported once: %v", problems)
	}
}

func TestABindingThatHidesABoardWideActionIsReported(t *testing.T) {
	_, problems := New(ticket, map[string][]string{"discuss": {"j", "down"}})
	if len(problems) != 1 {
		t.Fatalf("down has no key on a question any more: %v", problems)
	}
	if _, problems := New(ticket, map[string][]string{"discuss": {"j"}}); len(problems) != 0 {
		t.Fatalf("down keeps the arrow: %v", problems)
	}
}
