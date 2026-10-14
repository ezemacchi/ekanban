package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Reply names the brief the way the skills teach the orchestrator to read:
// another's question by who answers it, one for the user as the user's own answer,
// and a question with no brief quoted.
func TestReplyText(t *testing.T) {
	var q Questions
	other := ticket.Question{Words: "Which caption?", To: "Ana", FromWorktree: ".runs/ABC-1/questions/which-caption.md"}
	if got := q.ReplyText(other); got != "Answer from Ana to the open question .runs/ABC-1/questions/which-caption.md: " {
		t.Fatalf("other %q", got)
	}
	mine := ticket.Question{Words: "Which model?", To: ticket.You, FromWorktree: ".runs/ABC-1/questions/model.md"}
	if got := q.ReplyText(mine); got != "My answer to the open question .runs/ABC-1/questions/model.md: " {
		t.Fatalf("mine %q", got)
	}
	old := ticket.Question{Words: "Which key?"}
	if got := q.ReplyText(old); got != `Answer from the person asked to the open question "Which key?": ` {
		t.Fatalf("no brief %q", got)
	}
	custom := Questions{Reply: "{to} answered {question} ({brief}): "}
	if got := custom.ReplyText(other); got != "Ana answered Which caption? (.runs/ABC-1/questions/which-caption.md): " {
		t.Fatalf("custom %q", got)
	}
}

func discussRun(t *testing.T, needs string) (*ticket.Run, ticket.Question) {
	t.Helper()
	r := run(t)
	brief := filepath.Join(r.Dir, "questions", "which-caption.md")
	if err := os.MkdirAll(filepath.Dir(brief), 0o755); err != nil {
		t.Fatal(err)
	}
	q := ticket.Question{Words: "Which caption?", BriefPath: "questions/which-caption.md", BriefFile: brief,
		Brief: &ticket.Brief{Needs: needs}}
	return r, q
}

// Discuss opens a tab, starts the agent the brief's needs asks for, and
// points it at a prompt file naming the brief.
func TestDiscussStartsTheAgentTheBriefNeeds(t *testing.T) {
	f := &fake{agents: []herdr.Agent{{Name: "abc-1-q-which-caption"}}}
	r, q := discussRun(t, "small")
	qs := Questions{Small: Agent{Kind: "cursor", Args: []string{"--model", "small-model"}}, Large: Agent{Kind: "cursor", Args: []string{"--model", "large-model"}}}
	text, err := Discuss(f, r, q, qs)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.tabs) != 1 || f.tabs[0] != "w|"+r.Worktree+"|question which-caption" {
		t.Fatalf("tabs %q", f.tabs)
	}
	if len(f.started) != 1 || f.started[0] != "abc-1-q-which-caption-2|cursor|w:p9|--model small-model" {
		t.Fatalf("started %q", f.started)
	}
	file := filepath.Join(r.Dir, "questions", "which-caption-discuss.md")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), filepath.ToSlash(q.BriefFile)) || !strings.Contains(string(data), "ABC-1") {
		t.Fatalf("prompt %q", data)
	}
	if len(f.prompts) != 1 || !strings.Contains(f.prompts[0], filepath.ToSlash(file)) {
		t.Fatalf("prompts %q", f.prompts)
	}
	if !strings.Contains(text, "small model") {
		t.Fatalf("text %q", text)
	}

	// Anything but small takes the large one.
	f = &fake{}
	r, q = discussRun(t, "")
	if _, err := Discuss(f, r, q, qs); err != nil || !strings.HasSuffix(f.started[0], "--model large-model") {
		t.Fatalf("%v %q", err, f.started)
	}
}

// With no agent set for the size, or no brief, it says so and starts nothing.
func TestDiscussRefuses(t *testing.T) {
	f := &fake{}
	r, q := discussRun(t, "small")
	if _, err := Discuss(f, r, q, Questions{}); err == nil || !strings.Contains(err.Error(), "[questions] small") {
		t.Fatalf("no kind: %v", err)
	}
	q.Brief = nil
	if _, err := Discuss(f, r, q, Questions{Small: Agent{Kind: "cursor"}}); err == nil {
		t.Fatal("no brief: no error")
	}
	if len(f.tabs) != 0 || len(f.started) != 0 {
		t.Fatalf("something was started: %+v", f)
	}
}
