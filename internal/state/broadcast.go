package state

import (
	"fmt"
	"strings"
)

// The broadcast from a terminal: the same rule as the board's box -- one
// message to each chosen ticket's orchestrator, never to one waiting at a
// dialog, {target} and {key} filled per ticket -- chosen by filters instead
// of ticks.

// Filter picks the tickets a broadcast goes to.
type Filter struct {
	Name  string // as given, for messages
	match func(Ticket) bool
}

// ParseFilter reads one filter:
//
//	all            every ticket
//	pr             a pull request still open
//	idle, working, done
//	               what the orchestrator is doing
//	column=<x>     the column, by id or name
//	key=<K1,K2>    those tickets
func ParseFilter(s string) (Filter, error) {
	f := Filter{Name: s}
	name, value, hasValue := strings.Cut(strings.TrimSpace(s), "=")
	name = strings.ToLower(name)
	switch {
	case name == "all" && !hasValue:
		f.match = func(Ticket) bool { return true }
	case name == "pr" && !hasValue:
		f.match = func(t Ticket) bool { return t.PR != nil && t.PR.Open && t.PR.MergedTo == "" }
	case (name == "idle" || name == "working" || name == "done") && !hasValue:
		f.match = func(t Ticket) bool { return t.Orchestrator != nil && t.Orchestrator.Status == name }
	case name == "column" && value != "":
		f.match = func(t Ticket) bool {
			return strings.EqualFold(t.Column, value) || strings.EqualFold(t.ColumnLabel, value)
		}
	case name == "key" && value != "":
		keys := map[string]bool{}
		for _, k := range strings.Split(value, ",") {
			keys[strings.ToUpper(strings.TrimSpace(k))] = true
		}
		f.match = func(t Ticket) bool { return keys[strings.ToUpper(t.Key)] }
	default:
		return f, fmt.Errorf("unknown filter %q (want: all, pr, idle, working, done, column=<id or name>, key=<KEY,...>)", s)
	}
	return f, nil
}

// Recipient is one orchestrator a broadcast reaches, with its message.
type Recipient struct {
	Key     string
	Pane    string
	Status  string
	Message string
}

// Skipped is a chosen ticket the broadcast does not reach, and why.
type Skipped struct {
	Key, Why string
}

// Recipients are the tickets every filter matches, split into the
// orchestrators that get text and those left out: one waiting at a dialog
// (its enter could answer the dialog) or a ticket with none open. fallback is
// the target {target} becomes when a ticket has none.
func Recipients(tickets []Ticket, filters []Filter, text, fallback string) (to []Recipient, skipped []Skipped) {
	for _, t := range tickets {
		if !matchesAll(t, filters) {
			continue
		}
		switch {
		case t.Orchestrator == nil:
			skipped = append(skipped, Skipped{t.Key, "no orchestrator open"})
		case t.Orchestrator.Status == "blocked":
			skipped = append(skipped, Skipped{t.Key, "orchestrator waiting at a prompt: answer it there first"})
		default:
			to = append(to, Recipient{Key: t.Key, Pane: t.Orchestrator.Pane, Status: t.Orchestrator.Status, Message: Expand(text, t, fallback)})
		}
	}
	return to, skipped
}

func matchesAll(t Ticket, filters []Filter) bool {
	for _, f := range filters {
		if !f.match(t) {
			return false
		}
	}
	return true
}

// Expand is text as t gets it: {target} its target branch, else fallback,
// and {key} its key.
func Expand(text string, t Ticket, fallback string) string {
	target := t.Git.Target
	if target == "" {
		target = fallback
	}
	if target == "" {
		target = "{target}"
	}
	return strings.NewReplacer("{target}", target, "{key}", t.Key).Replace(text)
}
