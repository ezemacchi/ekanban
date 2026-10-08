package pipeline

import (
	"fmt"
	"sort"
)

// Facts is everything a rule may look at about one ticket. Classify gathers
// it from the run, the build server and git; rules only read it, so a rule never does
// I/O and can be tested with a literal.
type Facts struct {
	Key        string
	HasRun     bool   // the worktree has a run for this ticket
	NotStarted bool   // the run exists but nothing was dispatched
	Landed     bool   // the run's landing check passed
	PR         int    // 0 when none is known
	PROpen     bool   // the build server still builds the pull request
	Build      string // last pull request build: SUCCESS, FAILURE, RUNNING, ...
	MergedTo   string // target branch the pull request was merged into, or ""
	Deployed   string // publish that includes the merge ("dev #212"), or ""
	Offline    string // why build server data is missing, or ""
}

// Decision is a rule's answer: the column, and an optional note shown on the
// card when the column was chosen on partial data.
type Decision struct {
	Stage string
	Note  string
}

// Rule decides a ticket's column, or passes. Rules run in order (a chain of
// responsibility); the first one that decides wins. To change how tickets
// move, implement Rule, Register it, and name it in config.toml's
// pipeline.rules -- the board and the facts are untouched.
type Rule interface {
	// Name is how config.toml refers to the rule.
	Name() string
	// Decide returns the column and true, or false to pass.
	Decide(f Facts) (Decision, bool)
}

var registry = map[string]Rule{}

// Register makes a rule available by name. Registering a name twice panics:
// two rules answering to one name would make the config ambiguous.
func Register(r Rule) {
	if _, dup := registry[r.Name()]; dup {
		panic("pipeline: rule registered twice: " + r.Name())
	}
	registry[r.Name()] = r
}

// DefaultRules is the order used when config.toml names none.
var DefaultRules = []string{"deployed", "merged", "not-started", "working", "landed"}

// Chain resolves rule names in order. Unknown names are skipped and reported,
// so a typo in the config costs one rule, not the board.
func Chain(names []string) ([]Rule, []string) {
	if len(names) == 0 {
		names = DefaultRules
	}
	var rules []Rule
	var problems []string
	for _, n := range names {
		r, ok := registry[n]
		if !ok {
			problems = append(problems, fmt.Sprintf("pipeline rule %q does not exist (known: %v)", n, RuleNames()))
			continue
		}
		rules = append(rules, r)
	}
	return rules, problems
}

// RuleNames lists the registered rules, sorted.
func RuleNames() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Decide runs the chain. A ticket no rule claims goes to To Do.
func Decide(rules []Rule, f Facts) Decision {
	for _, r := range rules {
		if d, ok := r.Decide(f); ok {
			return d
		}
	}
	return Decision{Stage: ToDo}
}
