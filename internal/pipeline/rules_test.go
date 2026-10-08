package pipeline

import (
	"strings"
	"testing"
)

func TestDefaultChainPlacesEveryStage(t *testing.T) {
	rules, problems := Chain(nil)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	cases := []struct {
		name string
		f    Facts
		want string
	}{
		{"no run", Facts{Key: "ABC-1"}, ToDo},
		{"nothing dispatched", Facts{Key: "ABC-1", HasRun: true, NotStarted: true}, ToDo},
		{"working", Facts{Key: "ABC-1", HasRun: true}, InProgress},
		{"landed", Facts{Key: "ABC-1", HasRun: true, Landed: true, PR: 5, PROpen: true}, OnReview},
		{"merged", Facts{Key: "ABC-1", HasRun: true, Landed: true, PR: 5, MergedTo: "main"}, ToDeploy},
		{"deployed", Facts{Key: "ABC-1", HasRun: true, Landed: true, PR: 5, MergedTo: "main", Deployed: "dev #3"}, ReadyQA},
		// A merge wins over the run: the run may never have been marked landed.
		{"merged without landing", Facts{Key: "ABC-1", HasRun: true, PR: 5, MergedTo: "main"}, ToDeploy},
	}
	for _, c := range cases {
		if got := Decide(rules, c.f).Stage; got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestOfflineNoteRidesOnTheDecision(t *testing.T) {
	rules, _ := Chain(nil)
	d := Decide(rules, Facts{Key: "ABC-1", HasRun: true, Landed: true, Offline: "Jenkins no responde"})
	if d.Stage != OnReview || d.Note == "" {
		t.Fatalf("landed with Jenkins offline and no PR: %+v", d)
	}
}

// The order in config.toml is the order of the chain.
func TestChainFollowsConfiguredOrder(t *testing.T) {
	rules, _ := Chain([]string{"working", "merged"})
	f := Facts{Key: "ABC-1", HasRun: true, MergedTo: "main"}
	if got := Decide(rules, f).Stage; got != InProgress {
		t.Fatalf("working first must win over merged, got %s", got)
	}
}

func TestUnknownRuleIsReportedNotFatal(t *testing.T) {
	rules, problems := Chain([]string{"merged", "nope"})
	if len(rules) != 1 || len(problems) != 1 || !strings.Contains(problems[0], "nope") {
		t.Fatalf("rules %d, problems %v", len(rules), problems)
	}
}

// A rule added from outside the built-ins takes part like any other.
type alwaysReview struct{}

func (alwaysReview) Name() string                  { return "test-always-review" }
func (alwaysReview) Decide(Facts) (Decision, bool) { return Decision{Stage: OnReview}, true }

func TestRegisteredRuleJoinsTheChain(t *testing.T) {
	Register(alwaysReview{})
	defer delete(registry, "test-always-review")
	rules, problems := Chain([]string{"test-always-review"})
	if len(problems) > 0 || Decide(rules, Facts{}).Stage != OnReview {
		t.Fatalf("registered rule not used: %v", problems)
	}
}
