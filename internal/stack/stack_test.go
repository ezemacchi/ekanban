package stack

import (
	"strings"
	"testing"
	"time"
)

type fakeLookup map[string]Ticket

func (f fakeLookup) Ticket(key string) (Ticket, bool) {
	t, ok := f[key]
	return t, ok
}

type fixed struct {
	name  string
	items []Item
}

func (f fixed) Name() string              { return f.name }
func (f fixed) Needs(Card) bool           { return false }
func (f fixed) Items(Card, Lookup) []Item { return f.items }

func register(t *testing.T, name string, items ...Item) {
	t.Helper()
	Register(name, func(map[string]string) (Source, []string) { return fixed{name, items}, nil })
	t.Cleanup(func() { delete(registry, name) })
}

func TestNoSourcesIsOff(t *testing.T) {
	s, problems := New(Config{})
	if s != nil || len(problems) != 0 {
		t.Fatalf("New(empty) = %v, %v; want nil and no problems", s, problems)
	}
	if s.On() || s.Of(Card{Key: "ABC-1"}, nil) != nil || s.Step("a", 2, 1) || s.At("a", 2) != -1 || s.Column() != "" {
		t.Fatal("a nil Stacks must answer as off")
	}
}

func TestUnknownSourceIsReportedAndSkipped(t *testing.T) {
	register(t, "fake-a", Item{Key: "ABC-10"})
	s, problems := New(Config{Sources: []string{"nope", "fake-a"}, Source: map[string]map[string]string{"other": {}}})
	if !s.On() {
		t.Fatal("the known source should still run")
	}
	if len(problems) != 2 || !strings.Contains(problems[0], `"nope"`) || !strings.Contains(problems[1], "other") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestOfKeepsTheFirstSourcesItemAndSkipsTheCardItself(t *testing.T) {
	register(t, "fake-a", Item{Key: "ABC-10", Word: "failed"}, Item{Key: "ABC-1"})
	register(t, "fake-b", Item{Key: "abc-10", Word: "other"}, Item{Key: "ABC-11", Word: "blocked"})
	s, _ := New(Config{Sources: []string{"fake-a", "fake-b"}})
	got := s.Of(Card{Key: "ABC-1"}, nil)
	if len(got) != 2 || got[0].Key != "ABC-10" || got[0].Word != "failed" || got[1].Key != "ABC-11" {
		t.Fatalf("Of = %+v", got)
	}
}

func TestMaxItemsDefaultsAndReportsANegative(t *testing.T) {
	register(t, "fake-a", Item{Key: "ABC-10"})
	s, problems := New(Config{Sources: []string{"fake-a"}, MaxItems: -2})
	if s.Max() != DefaultMaxItems || len(problems) != 1 {
		t.Fatalf("Max = %d, problems = %v", s.Max(), problems)
	}
}

func TestStepWalksTheStackAndReportsLeavingIt(t *testing.T) {
	register(t, "fake-a", Item{Key: "ABC-10"})
	s, _ := New(Config{Sources: []string{"fake-a"}})
	if s.At("card", 2) != -1 {
		t.Fatal("a card starts on itself")
	}
	if s.Step("card", 2, -1) {
		t.Fatal("up from the card leaves the stack")
	}
	for want := 0; want < 2; want++ {
		if !s.Step("card", 2, 1) || s.At("card", 2) != want {
			t.Fatalf("down to strip %d: at %d", want, s.At("card", 2))
		}
	}
	if s.Step("card", 2, 1) || s.At("card", 2) != 1 {
		t.Fatal("down from the last strip leaves the stack and keeps the cursor")
	}
	if !s.Step("card", 2, -1) || s.At("card", 2) != 0 {
		t.Fatal("up goes back a strip")
	}
	if s.At("card", 3) != -1 {
		t.Fatal("a stack that changed length starts over on the card")
	}
	s.Set("card", 3, 2)
	if s.At("card", 3) != 2 {
		t.Fatal("Set puts the cursor on a strip")
	}
}

func TestNeighbours(t *testing.T) {
	items := []Item{{Key: "ABC-10"}, {Key: "ABC-11"}}
	for _, c := range []struct {
		at       int
		up, down string
	}{{-1, "", "ABC-10"}, {0, "ABC-1", "ABC-11"}, {1, "ABC-10", ""}} {
		up, down := Neighbours("ABC-1", items, c.at)
		if up != c.up || down != c.down {
			t.Errorf("at %d: %q, %q; want %q, %q", c.at, up, down, c.up, c.down)
		}
	}
}

func TestSummary(t *testing.T) {
	if got := Summary([]Item{{Word: "failed"}, {Word: "failed"}}, 0); got != "+2 failed" {
		t.Errorf("same words: %q", got)
	}
	if got := Summary([]Item{{Word: "failed"}, {Word: "blocked"}}, 0); got != "+2 related" {
		t.Errorf("mixed words: %q", got)
	}
	if got := Summary([]Item{{Word: "failed"}}, 4); got != "+5 failed" {
		t.Errorf("with the ones left out: %q", got)
	}
}

func story() Ticket {
	return Ticket{
		Key: "ABC-1",
		Tests: []Linked{
			{Key: "ABC-10", Summary: "[ABC-1][TC-1-01] the control is shown", URL: "https://example.com/ABC-10"},
			{Key: "ABC-11", Summary: "[ABC-1][TC-1-02] the control opens the workspace"},
			{Key: "ABC-12", Summary: "[ABC-1][TC-1-03] untouched"},
		},
	}
}

func failedSource(t *testing.T, opts map[string]string) *Stacks {
	t.Helper()
	s, problems := New(Config{Sources: []string{"failed-tests"}, Source: map[string]map[string]string{"failed-tests": opts}})
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	return s
}

func TestFailedTestsReadsQAComments(t *testing.T) {
	st := story()
	st.Notes = []Note{{Author: "QA", At: time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC), Body: strings.Join([]string{
		"The control is missing on the story ABC-1, NO GO.",
		"* Failed test: ABC-10 (TC-1-01)",
		"* Blocked test: TC-1-02 - blocked because the control is missing",
		"ABC-12 was not run.",
	}, "\r\n")}}
	got := failedSource(t, nil).Of(Card{Key: "ABC-1"}, fakeLookup{"ABC-1": st})
	if len(got) != 2 {
		t.Fatalf("items = %+v", got)
	}
	if got[0].Key != "ABC-10" || got[0].Word != "failed" || !got[0].Attention || got[0].URL == "" || !strings.Contains(got[0].Evidence, "Failed test") {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Key != "ABC-11" || got[1].Word != "blocked" || got[1].Attention {
		t.Errorf("second = %+v", got[1])
	}
}

func TestFailedTestsALaterPassTakesItOff(t *testing.T) {
	st := story()
	st.Notes = []Note{{Body: "ABC-10 failed"}, {Body: "Retest: ABC-10 passed"}}
	if got := failedSource(t, nil).Of(Card{Key: "ABC-1"}, fakeLookup{"ABC-1": st}); len(got) != 0 {
		t.Fatalf("items = %+v", got)
	}
}

func TestFailedTestsDoesNotMatchALongerKey(t *testing.T) {
	st := story()
	st.Notes = []Note{{Body: "ABC-100 failed"}}
	if got := failedSource(t, nil).Of(Card{Key: "ABC-1"}, fakeLookup{"ABC-1": st}); len(got) != 0 {
		t.Fatalf("items = %+v", got)
	}
}

func TestFailedTestsFromLinked(t *testing.T) {
	got := failedSource(t, map[string]string{"from": "linked"}).Of(Card{Key: "ABC-1"}, fakeLookup{"ABC-1": story()})
	if len(got) != 3 {
		t.Fatalf("items = %+v", got)
	}
}

func TestFailedTestsOwnWords(t *testing.T) {
	st := story()
	st.Notes = []Note{{Body: "ABC-10 fallido"}}
	got := failedSource(t, map[string]string{"failed": `(?i)fallid[oa]`}).Of(Card{Key: "ABC-1"}, fakeLookup{"ABC-1": st})
	if len(got) != 1 || got[0].Key != "ABC-10" {
		t.Fatalf("items = %+v", got)
	}
}

func TestFailedTestsBadSettingsFallBack(t *testing.T) {
	_, problems := New(Config{Sources: []string{"failed-tests"}, Source: map[string]map[string]string{
		"failed-tests": {"from": "xray", "failed": "(", "colour": "red"},
	}})
	if len(problems) != 3 {
		t.Fatalf("problems = %v", problems)
	}
}
