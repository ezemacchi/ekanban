package ticket

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// sessionFile is where a run keeps its orchestrator's conversation id, so a
// new orchestrator can be started on the same conversation.
const sessionFile = "orchestrator-session.json"

type savedSession struct {
	Agent   string `json:"agent"`
	Session string `json:"session"`
}

// RememberSession records the id of the conversation the run's orchestrator is
// in. It writes only when the id changed, and a failure is ignored: the id is
// a convenience, and the run's files remain the memory.
func RememberSession(runDir, agent, id string) {
	if runDir == "" || id == "" {
		return
	}
	if a, s := SavedSession(runDir); a == agent && s == id {
		return
	}
	raw, err := json.Marshal(savedSession{Agent: agent, Session: id})
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(runDir, sessionFile), append(raw, '\n'), 0o644)
}

// SavedSession is the agent kind and conversation id last recorded for the
// run, or empty strings.
func SavedSession(runDir string) (agent, id string) {
	raw, err := os.ReadFile(filepath.Join(runDir, sessionFile))
	if err != nil {
		return "", ""
	}
	var s savedSession
	if json.Unmarshal(raw, &s) != nil {
		return "", ""
	}
	return s.Agent, s.Session
}
