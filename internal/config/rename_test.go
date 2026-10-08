package config

import (
	"slices"
	"strings"
	"testing"
)

// [lead] and keys' lead are the old names of [orchestrator] and its action:
// still read, with a nudge to rename them.
func TestOldLeadNamesStillWork(t *testing.T) {
	write(t, `[keys]
lead = "O"

[lead]
kind = "cursor"
prompt = "old"

[orchestrator]
prompt = "new"
`)
	s := LoadFor("")
	if s.Orchestrator.Kind != "cursor" || s.Orchestrator.Prompt != "new" {
		t.Fatalf("orchestrator %+v: want kind from [lead], prompt from [orchestrator]", s.Orchestrator)
	}
	if !slices.Equal(s.Keys["orchestrator"], []string{"O"}) || s.Keys["lead"] != nil {
		t.Fatalf("keys %v", s.Keys)
	}
	all := strings.Join(s.Problems, "\n")
	if !strings.Contains(all, "[lead] is now [orchestrator]") || !strings.Contains(all, "keys.lead is now keys.orchestrator") {
		t.Fatalf("no rename nudge in %q", all)
	}
}
