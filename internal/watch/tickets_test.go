package watch

import (
	"context"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/alert"
	"github.com/ezemacchi/ekanban/internal/herdrtest"
	"github.com/ezemacchi/ekanban/internal/state"
)

func ticketWith(orch string, forYou []string, pr *state.PR, ahead int) state.Ticket {
	t := state.Ticket{Key: "ABC-1", Git: state.Branch{Compared: "origin/main", Ahead: ahead}, PR: pr}
	if orch != "" {
		t.Orchestrator = &state.Agent{Who: "Orchestrator", Pane: "w1:p1", Status: orch}
	}
	for _, q := range forYou {
		t.Questions.ForYou = append(t.Questions.ForYou, state.Question{Text: q})
	}
	return t
}

func kinds(as []alert.Alert) []alert.Kind {
	var out []alert.Kind
	for _, a := range as {
		out = append(out, a.Kind)
	}
	return out
}

// A ticket first seen is the starting point, whatever it is doing.
func TestTicketNoticesFirstSeen(t *testing.T) {
	now := ticketWith("blocked", []string{"Which model?"}, &state.PR{Number: 3, MergedTo: "main"}, 0)
	if got := ticketNotices(now, nil, seenOf(now)); len(got) != 0 {
		t.Fatalf("first seen: %v", kinds(got))
	}
}

func TestTicketNotices(t *testing.T) {
	cases := []struct {
		name          string
		before, after state.Ticket
		want          []alert.Kind
	}{
		{"finished", ticketWith("working", nil, nil, 2), ticketWith("done", nil, nil, 2), []alert.Kind{alert.OrchestratorDone}},
		{"blocked", ticketWith("working", nil, nil, 2), ticketWith("blocked", nil, nil, 2), []alert.Kind{alert.OrchestratorBlocked}},
		{"done to idle is only you looking", ticketWith("done", nil, nil, 2), ticketWith("idle", nil, nil, 2), nil},
		{"a new question for you", ticketWith("idle", []string{"Which key?"}, nil, 2), ticketWith("idle", []string{"Which key?", "Which caption?"}, nil, 2), []alert.Kind{alert.QuestionForYou}},
		{"a question answered", ticketWith("idle", []string{"Which key?"}, nil, 2), ticketWith("idle", nil, nil, 2), nil},
		{"merged by anyone", ticketWith("idle", nil, &state.PR{Number: 9, Open: true}, 2), ticketWith("idle", nil, &state.PR{Number: 9, MergedTo: "main"}, 0), []alert.Kind{alert.Merged}},
		{"already in its target", ticketWith("idle", nil, nil, 3), ticketWith("idle", nil, nil, 0), []alert.Kind{alert.Contained}},
		{"a new worktree is not merged", ticketWith("idle", nil, nil, 0), ticketWith("idle", nil, nil, 0), nil},
	}
	for _, c := range cases {
		b := seenOf(c.before)
		got := kinds(ticketNotices(c.after, &b, seenOf(c.after)))
		if strings.Join(toStrings(got), ",") != strings.Join(toStrings(c.want), ",") {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func finishedTurn(status string, completion uint64, pane string) state.Ticket {
	t := ticketWith(status, nil, nil, 2)
	t.Orchestrator.Completion, t.Orchestrator.Pane = completion, pane
	return t
}

// Herdr's number for the finished turn says the orchestrator finished even
// when you looked first (idle, not done) or it finished twice between polls.
func TestTicketNoticesByFinishedTurn(t *testing.T) {
	cases := []struct {
		name          string
		before, after state.Ticket
		oldRecord     bool
		want          []alert.Kind
	}{
		{"finished and already looked at", finishedTurn("working", 0, "w1:p1"), finishedTurn("idle", 40, "w1:p1"), false, []alert.Kind{alert.OrchestratorDone}},
		{"two turns between polls", finishedTurn("idle", 33, "w1:p1"), finishedTurn("done", 40, "w1:p1"), false, []alert.Kind{alert.OrchestratorDone}},
		{"the same turn, now seen", finishedTurn("done", 40, "w1:p1"), finishedTurn("idle", 40, "w1:p1"), false, nil},
		{"working again", finishedTurn("idle", 40, "w1:p1"), finishedTurn("working", 0, "w1:p1"), false, nil},
		{"another orchestrator in another pane", finishedTurn("idle", 40, "w1:p1"), finishedTurn("idle", 7, "w1:p9"), false, nil},
		{"a record from before turns were kept", finishedTurn("idle", 0, "w1:p1"), finishedTurn("idle", 40, "w1:p1"), true, nil},
	}
	for _, c := range cases {
		b := seenOf(c.before)
		if c.oldRecord {
			b.V = 0
		}
		got := kinds(ticketNotices(c.after, &b, seenOf(c.after)))
		if strings.Join(toStrings(got), ",") != strings.Join(toStrings(c.want), ",") {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// Compared against another target is not the branch reaching its own.
func TestTicketNoticesTargetChanged(t *testing.T) {
	before := ticketWith("idle", nil, nil, 3)
	after := ticketWith("idle", nil, nil, 0)
	after.Git.Compared = "origin/develop"
	b := seenOf(before)
	if got := ticketNotices(after, &b, seenOf(after)); len(got) != 0 {
		t.Fatalf("got %v", kinds(got))
	}
}

func toStrings(ks []alert.Kind) []string {
	var out []string
	for _, k := range ks {
		out = append(out, string(k))
	}
	return out
}

// The watcher tells you an orchestrator finished with no GitHub at all, and
// says it once: the next poll, with nothing new, is silent.
func TestPollTicketsWithoutGitHub(t *testing.T) {
	space, key := tempSpace(t)
	stateDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)

	f := herdrtest.Start(t)
	f.Route(map[string]any{"session.snapshot": snapshotWith(space, "api")})

	status := "working"
	read := func(_ context.Context, dirs []string, _ bool) []Row {
		var out []Row
		for _, d := range dirs {
			if d == key {
				out = append(out, Row{Dir: d, Ticket: ticketWith(status, nil, nil, 1)})
			}
		}
		return out
	}
	for i, s := range []string{"working", "done", "done"} {
		status = s
		if err := poll(context.Background(), stateDir, true, nil, read, false); err != nil {
			t.Fatal(err)
		}
		want := map[int]int{0: 0, 1: 1, 2: 1}[i]
		if n := f.Called("notification.show"); n != want {
			t.Fatalf("poll %d (%s): %d toasts, want %d", i, s, n, want)
		}
	}
	params := herdrtest.Params(t, f.Last(t, "notification.show"))
	if params["title"] != "Orchestrator finished" || !strings.Contains(params["body"].(string), "ABC-1") {
		t.Fatalf("toast: %v", params)
	}
	if n := alert.Load(stateDir).Count(key); n != 1 {
		t.Fatalf("%d alerts recorded, want 1", n)
	}
}
