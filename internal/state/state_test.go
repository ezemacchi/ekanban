package state

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// A row carries the pipeline's classification, the orchestrator apart from
// the other agents, and the questions split by the section they sit in.
func TestRow(t *testing.T) {
	info := pipeline.Info{
		Stage: pipeline.OnReview, Key: "ABC-1", Title: "Add the export", Phase: "Reviewer",
		PR: 42, PROpen: true, Build: "SUCCESS",
		Orchestrator: ticket.Agent{Who: "Orchestrator", Pane: "w1:p1", Status: "idle"},
		Agents:       []ticket.Agent{{Who: "Orchestrator", Pane: "w1:p1", Status: "idle"}, {Who: "Reviewer", Pane: "w1:p2", Status: "working"}},
		ForYou:       []ticket.Question{{Words: "Which model?", To: ticket.You}},
		Questions:    []ticket.Question{{Words: "Which caption?", To: "Ana", FromWorktree: ".runs/ABC-1/questions/caption.md"}},
	}
	row := Row("/w/abc-1", "feature/abc-1", "w1", info, Branch{Target: "main", Behind: 3}, pipeline.DefaultColumns)

	if row.Column != pipeline.OnReview || row.ColumnLabel != "Reviewing" || row.PR == nil || row.PR.Number != 42 || !row.PR.Open {
		t.Fatalf("column or PR: %+v", row)
	}
	if row.Orchestrator == nil || row.Orchestrator.Pane != "w1:p1" || len(row.Agents) != 2 {
		t.Fatalf("agents: %+v", row)
	}
	if len(row.Questions.ForYou) != 1 || len(row.Questions.Others) != 1 || row.Questions.Others[0].Brief != ".runs/ABC-1/questions/caption.md" {
		t.Fatalf("questions: %+v", row.Questions)
	}
	if row.Git.Behind != 3 || row.Workspace != "w1" {
		t.Fatalf("git or workspace: %+v", row)
	}
}

// With nothing to say a row still has empty lists, so a reader can iterate
// without checking for null.
func TestRowEmptyListsAreArrays(t *testing.T) {
	row := Row("/w/abc-2", "", "", pipeline.Info{Key: "ABC-2", Stage: pipeline.ToDo}, Branch{}, nil)
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"agents":[]`, `"for_you":[]`, `"others":[]`} {
		if !strings.Contains(s, want) {
			t.Errorf("%s missing from %s", want, s)
		}
	}
	if strings.Contains(s, `"pr"`) || strings.Contains(s, `"orchestrator"`) {
		t.Errorf("no PR and no orchestrator should be left out: %s", s)
	}
}

func TestTargetOf(t *testing.T) {
	cases := []struct {
		run       string
		targets   []string
		want, src string
	}{
		{"develop", []string{"main"}, "develop", "run"},
		{" ", []string{"main", "develop"}, "main", "default"},
		{"", nil, "", ""},
	}
	for _, c := range cases {
		if got, src := TargetOf(c.run, c.targets); got != c.want || src != c.src {
			t.Errorf("TargetOf(%q, %v) = %q, %q; want %q, %q", c.run, c.targets, got, src, c.want, c.src)
		}
	}
}

// fakeGit answers by the first argument; anything else fails.
func fakeGit(answers map[string]string) Git {
	return func(dir string, args ...string) (string, error) {
		key := strings.Join(args, " ")
		for prefix, out := range answers {
			if strings.HasPrefix(key, prefix) {
				return out, nil
			}
		}
		return "", errors.New("no answer for " + key)
	}
}

func TestFacts(t *testing.T) {
	git := fakeGit(map[string]string{
		"rev-list --left-right --count origin/main...HEAD": "109\t2",
		"status --porcelain":                    " M a.go\n?? b.go",
		"rev-list --count HEAD --not --remotes": "1",
	})
	b := Facts(git, "/w", "main", "run")
	if b.Compared != "origin/main" || b.Behind != 109 || b.Ahead != 2 || b.Dirty != 2 || b.Unpushed != 1 || b.TargetFrom != "run" {
		t.Fatalf("facts: %+v", b)
	}
}

// A target with no remote-tracking branch is not compared, rather than
// reported as level with it.
func TestFactsWithoutRemoteTarget(t *testing.T) {
	b := Facts(fakeGit(map[string]string{"status --porcelain": ""}), "/w", "gone", "default")
	if b.Compared != "" || b.Behind != 0 || b.Ahead != 0 || b.Dirty != 0 {
		t.Fatalf("facts: %+v", b)
	}
}

func TestWorktrees(t *testing.T) {
	porcelain := "worktree /r/main\nHEAD abc\nbranch refs/heads/main\n\nworktree /r/wt/abc-1\nHEAD def\nbranch refs/heads/abc-1\n"
	got := Worktrees(porcelain)
	if len(got) != 2 || got[0] != "/r/main" || got[1] != "/r/wt/abc-1" {
		t.Fatalf("worktrees: %v", got)
	}
}

func TestCounts(t *testing.T) {
	if l, r, ok := counts("4\t0\n"); !ok || l != 4 || r != 0 {
		t.Fatalf("counts: %d %d %t", l, r, ok)
	}
	if _, _, ok := counts("fatal: bad revision"); ok {
		t.Fatal("an error message read as counts")
	}
}
