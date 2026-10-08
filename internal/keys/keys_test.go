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
