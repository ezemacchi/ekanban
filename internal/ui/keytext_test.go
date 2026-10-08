package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/screen"
)

// A rebound action is named by its new key everywhere: the footer's buttons,
// the detail modal's, and the messages that tell you what to press. The
// modal obeys the new key too.
func TestRebindingRenamesEveryMention(t *testing.T) {
	m := newTestModel(t)
	m.SetKeys(map[string][]string{"note": {"N"}, "layout": {"L"}})
	send(t, m, liveWorkspaces())
	m.layout = layoutKanban
	m.width = 140

	foot := ansi.Strip(m.View())
	if !strings.Contains(foot, "N note") || strings.Contains(foot, "n note") {
		t.Fatalf("the footer does not name N for note:\n%s", foot)
	}
	if !strings.Contains(foot, "L list") {
		t.Fatalf("the footer does not name L for the layout:\n%s", foot)
	}

	m.cycleSort()
	if got := screen.Plain(m.status); !strings.Contains(got, "press L to get there") {
		t.Fatalf("sorting outside the table names the old key: %q", got)
	}

	send(t, m, key("d"))
	if m.mode != modeDetail {
		t.Fatal("d did not open the detail modal")
	}
	modal := ansi.Strip(m.View())
	if !strings.Contains(modal, "N note") {
		t.Fatalf("the modal's buttons do not name N:\n%s", modal)
	}
	send(t, m, key("n"))
	if m.mode != modeDetail {
		t.Fatal("n still edits the note in the modal after note moved to N")
	}
	send(t, m, key("N"))
	if m.mode != modeNote {
		t.Fatalf("N did not edit the note from the modal; mode %v", m.mode)
	}
}

// The help's Archive line names the archive's current keys.
func TestArchiveHelpFollowsRebinding(t *testing.T) {
	m := newTestModel(t)
	m.SetKeys(map[string][]string{"open-pull-request": {"P"}, "restore": {}})
	got := screen.Plain(m.archiveKeysNote())
	if got != " (o tracker, P PR, / search)" {
		t.Fatalf("archive help %q", got)
	}
}

// An action with no key is left out of messages instead of naming a key that
// does something else.
func TestUnboundActionIsLeftOut(t *testing.T) {
	m := newTestModel(t)
	m.SetKeys(map[string][]string{"layout": {}})
	m.cycleSort()
	if got := screen.Plain(m.status); got != "sorting is a table thing" {
		t.Fatalf("status %q", got)
	}
}
