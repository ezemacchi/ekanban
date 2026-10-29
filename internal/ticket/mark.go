package ticket

import (
	"strings"

	"github.com/ezemacchi/ekanban/internal/herdr"
)

// A pane marked with a ticket and a role is found by that mark, whatever its
// agent is called and whatever its tab says. Herdr clears an agent's name when
// the agent restarts, and keeps a pane's tokens until they are replaced or the
// pane closes, so the mark is what lasts. Whoever starts an agent marks its
// pane -- the board an orchestrator or a discussion it opens, a team's own
// scripts the roles they start -- and the watcher marks an agent it found by
// name or tab that has no mark yet.
//
// A mark is display metadata to Herdr: it changes no agent state, and the
// sidebar shows the agent as "KEY · Label".
const (
	// TokenKey holds the ticket key, TokenRole the role: a team role's id,
	// RoleOrchestrator, or RoleQuestion.
	TokenKey  = "ekanban_key"
	TokenRole = "ekanban_role"
	// MarkSource is who ekanban reports a mark as.
	MarkSource = "plugin:ekanban"

	RoleOrchestrator = "orchestrator"
	RoleQuestion     = "question"
)

// Marker reports a pane's metadata (herdr.Client.ReportPaneMetadata).
type Marker interface {
	ReportPaneMetadata(paneID, source, display string, tokens map[string]string) error
}

// Mark marks pane as the agent of key in role, labelled label in the sidebar.
func Mark(m Marker, pane, key, role, label string) error {
	display := key
	if label != "" {
		display += " · " + label
	}
	return m.ReportPaneMetadata(pane, MarkSource, display, map[string]string{TokenKey: key, TokenRole: role})
}

// Marked is whether a's pane carries a mark, for any ticket.
func Marked(a herdr.Agent) bool { return strings.TrimSpace(a.Tokens[TokenKey]) != "" }

// markOf is the role a's pane is marked with for key, in lower case; "" when
// its pane carries no mark for key.
func markOf(a herdr.Agent, key string) string {
	if !strings.EqualFold(strings.TrimSpace(a.Tokens[TokenKey]), key) {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(a.Tokens[TokenRole]))
}

// markedElsewhere is whether a's pane is marked for another ticket than key.
func markedElsewhere(a herdr.Agent, key string) bool {
	return Marked(a) && !strings.EqualFold(strings.TrimSpace(a.Tokens[TokenKey]), key)
}
