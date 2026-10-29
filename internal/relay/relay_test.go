package relay

import (
	"errors"
	"testing"
	"time"

	"github.com/ezemacchi/ekanban/internal/alert"
	"github.com/ezemacchi/ekanban/internal/herdr"
)

func agentIn(pane, status string) herdr.Agent {
	kind := "cursor"
	return herdr.Agent{PaneID: pane, Agent: &kind, AgentStatus: status}
}

func raised(texts ...string) []alert.Alert {
	var out []alert.Alert
	for _, t := range texts {
		out = append(out, alert.Alert{Kind: alert.OrchestratorDone, Text: t})
	}
	return out
}

type sink struct {
	to, text []string
	err      error
}

func (s *sink) submit(pane, text string) error {
	if s.err != nil {
		return s.err
	}
	s.to, s.text = append(s.to, pane), append(s.text, text)
	return nil
}

// With nobody asking for notices, none are kept for later.
func TestQueueWithoutARelayKeepsNothing(t *testing.T) {
	r := Load(t.TempDir())
	if r.Queue(raised("ABC-1 · orchestrator finished")) || len(r.Pending) != 0 {
		t.Fatalf("kept %v with no relay", r.Pending)
	}
}

// Everything that waited goes in one message once the agent is free.
func TestDeliverSendsOneMessageWhenIdle(t *testing.T) {
	r := Load(t.TempDir())
	r.To("w2:p1", time.Now())
	r.Queue(raised("ABC-1 · orchestrator finished", "#9 merged — ABC-2"))

	for _, status := range []string{"idle", "done"} {
		s := &sink{}
		r.Pending = []Notice{{Text: "ABC-1 · orchestrator finished"}, {Text: "#9 merged — ABC-2"}}
		changed, err := r.Deliver([]herdr.Agent{agentIn("w2:p1", status)}, nil, s.submit)
		if err != nil || !changed {
			t.Fatalf("%s: changed %v, err %v", status, changed, err)
		}
		want := "[ekanban] ABC-1 · orchestrator finished; #9 merged — ABC-2"
		if len(s.text) != 1 || s.to[0] != "w2:p1" || s.text[0] != want {
			t.Fatalf("%s: sent %v to %v, want %q", status, s.text, s.to, want)
		}
		if len(r.Pending) != 0 {
			t.Fatalf("%s: %d notices still pending after delivery", status, len(r.Pending))
		}
	}
}

// An agent that is working, or asking the person something, is not
// interrupted; what waits is kept for later.
func TestDeliverWaitsForABusyAgent(t *testing.T) {
	for _, status := range []string{"working", "blocked", "unknown"} {
		r := Load(t.TempDir())
		r.To("w2:p1", time.Now())
		r.Queue(raised("ABC-1 · orchestrator finished"))
		s := &sink{}
		if changed, _ := r.Deliver([]herdr.Agent{agentIn("w2:p1", status)}, nil, s.submit); changed || len(s.text) != 0 {
			t.Fatalf("%s: sent %v", status, s.text)
		}
		if len(r.Pending) != 1 {
			t.Fatalf("%s: lost what was waiting", status)
		}
	}
}

// Herdr refusing because a dialog came up meanwhile is waiting, not failing.
func TestDeliverKeepsNoticesWhenRefusedForADialog(t *testing.T) {
	r := Load(t.TempDir())
	r.To("w2:p1", time.Now())
	r.Queue(raised("ABC-1 · orchestrator finished"))
	s := &sink{err: &herdr.APIError{Method: "agent.prompt", Code: "agent_blocked"}}
	changed, err := r.Deliver([]herdr.Agent{agentIn("w2:p1", "idle")}, nil, s.submit)
	if err != nil || changed || len(r.Pending) != 1 {
		t.Fatalf("changed %v, err %v, pending %d", changed, err, len(r.Pending))
	}

	s.err = errors.New("socket gone")
	if _, err := r.Deliver([]herdr.Agent{agentIn("w2:p1", "idle")}, nil, s.submit); err == nil || len(r.Pending) != 1 {
		t.Fatalf("a failed send must be reported and keep the notices; err %v", err)
	}
}

// A pane whose agent is restarting is waited for; a pane that is gone ends
// the relay.
func TestDeliverFollowsThePane(t *testing.T) {
	r := Load(t.TempDir())
	r.To("w2:p1", time.Now())
	r.Queue(raised("ABC-1 · orchestrator finished"))
	s := &sink{}
	shell := herdr.Agent{PaneID: "w2:p1", AgentStatus: "idle"}

	if changed, _ := r.Deliver([]herdr.Agent{shell}, []herdr.Pane{{PaneID: "w2:p1"}}, s.submit); changed || !r.On() {
		t.Fatal("a pane between agents ended the relay")
	}
	if changed, _ := r.Deliver(nil, []herdr.Pane{{PaneID: "w9:p1"}}, s.submit); !changed || r.On() {
		t.Fatal("a pane that is gone kept the relay")
	}
	if len(s.text) != 0 {
		t.Fatalf("sent %v to no agent", s.text)
	}
}

// The watcher saving a delivery does not undo the person turning the relay
// off, or pointing it elsewhere, in the meantime.
func TestSaveDoesNotUndoAChangeMadeMeanwhile(t *testing.T) {
	dir := t.TempDir()
	mine := Load(dir)
	mine.To("w2:p1", time.Now())
	if err := mine.Replace(); err != nil {
		t.Fatal(err)
	}

	watcher := Load(dir)
	watcher.Queue(raised("ABC-1 · orchestrator finished"))

	mine.Off()
	if err := mine.Replace(); err != nil {
		t.Fatal(err)
	}
	if err := watcher.Save(); err != nil {
		t.Fatal(err)
	}
	if Load(dir).On() {
		t.Fatal("the watcher's save turned the relay back on")
	}
}
