package ticketui

import (
	"strings"
	"testing"
)

// On a question r is reply, the question's own action, and refresh leaves
// the bottom row there; on a role r refreshes, and the bottom row says so.
func TestAQuestionsKeyHidesTheBoardsThere(t *testing.T) {
	m := questionBoard()
	if !strings.Contains(m.View(), "Refresh") || m.key("r") == nil {
		t.Fatal("on a role r refreshes, and the bottom row says so")
	}
	m.onQuestion, m.q = true, 0
	if strings.Contains(m.View(), "Refresh") {
		t.Fatal("on a question r is the question's: Refresh must leave the bottom row")
	}
	m.key("r")
	if m.isBusy() || !strings.Contains(m.status, "orchestrator") {
		t.Fatalf("r on a question did not reply: %q", m.status)
	}
}
