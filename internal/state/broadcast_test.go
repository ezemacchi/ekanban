package state

import "testing"

func tickets() []Ticket {
	orch := func(pane, status string) *Agent { return &Agent{Who: "Orchestrator", Pane: pane, Status: status} }
	return []Ticket{
		{Key: "ABC-1", Column: "on_review", ColumnLabel: "Reviewing", PR: &PR{Number: 1, Open: true}, Git: Branch{Target: "develop"}, Orchestrator: orch("w1:p1", "idle")},
		{Key: "ABC-2", Column: "in_progress", ColumnLabel: "Working", Orchestrator: orch("w2:p1", "working")},
		{Key: "ABC-3", Column: "in_progress", ColumnLabel: "Working", Orchestrator: orch("w3:p1", "blocked")},
		{Key: "ABC-4", Column: "ready_qa", ColumnLabel: "To QA", PR: &PR{Number: 4, MergedTo: "main"}},
	}
}

func keysOf(to []Recipient) []string {
	var out []string
	for _, r := range to {
		out = append(out, r.Key)
	}
	return out
}

// Everyone chosen gets it except the orchestrator at a prompt and the ticket
// with none, which are said, not dropped silently.
func TestRecipientsNeverBlocked(t *testing.T) {
	to, skipped := Recipients(tickets(), nil, "rebase onto origin/{target} for {key}", "main")
	if got := keysOf(to); len(got) != 2 || got[0] != "ABC-1" || got[1] != "ABC-2" {
		t.Fatalf("to: %v", got)
	}
	if len(skipped) != 2 || skipped[0].Key != "ABC-3" || skipped[1].Key != "ABC-4" {
		t.Fatalf("skipped: %+v", skipped)
	}
	if to[0].Message != "rebase onto origin/develop for ABC-1" || to[1].Message != "rebase onto origin/main for ABC-2" {
		t.Fatalf("messages: %q, %q", to[0].Message, to[1].Message)
	}
}

func TestFilters(t *testing.T) {
	cases := map[string][]string{
		"all":              {"ABC-1", "ABC-2"},
		"pr":               {"ABC-1"},
		"idle":             {"ABC-1"},
		"working":          {"ABC-2"},
		"column=Working":   {"ABC-2"},
		"column=on_review": {"ABC-1"},
		"key=abc-2,ABC-1":  {"ABC-1", "ABC-2"},
	}
	for spec, want := range cases {
		f, err := ParseFilter(spec)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		to, _ := Recipients(tickets(), []Filter{f}, "x", "main")
		got := keysOf(to)
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", spec, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: got %v, want %v", spec, got, want)
			}
		}
	}
}

// Filters narrow together.
func TestFiltersCombine(t *testing.T) {
	a, _ := ParseFilter("column=Working")
	b, _ := ParseFilter("idle")
	if to, _ := Recipients(tickets(), []Filter{a, b}, "x", ""); len(to) != 0 {
		t.Fatalf("to: %v", keysOf(to))
	}
}

func TestParseFilterRejects(t *testing.T) {
	for _, bad := range []string{"", "column=", "key=", "pr=1", "blocked", "everyone"} {
		if _, err := ParseFilter(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestExpandWithoutTarget(t *testing.T) {
	if got := Expand("onto {target}", Ticket{Key: "ABC-9"}, ""); got != "onto {target}" {
		t.Fatalf("got %q", got)
	}
}
