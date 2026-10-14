package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// questionWorktree is a worktree with run ABC-1 asking three questions, one
// for the user and two for others, one of them with a brief.
func questionWorktree(t *testing.T) string {
	t.Helper()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", t.TempDir()) // no personal config.toml
	wt := t.TempDir()
	run := filepath.Join(wt, ".runs", "ABC-1")
	if err := os.MkdirAll(filepath.Join(run, "questions"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := "# STATE ABC-1\n\n## Questions for you\n- Which model runs the implementer?\n\n## Open decisions\n- Technical (Bo): which key?\n- Product (Ana): which caption? [brief](questions/which-caption.md)\n\n## Landed\n"
	brief := "---\nto: Ana\nstatus: open\nchat: https://chat.example.com/x\n---\n## Outcome\n"
	for name, text := range map[string]string{"STATE.md": state, "questions/which-caption.md": brief} {
		if err := os.WriteFile(filepath.Join(run, filepath.FromSlash(name)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return wt
}

// It lists the open questions in the boards' order: for you first, then by
// who answers them.
func TestQuestionsLists(t *testing.T) {
	wt := questionWorktree(t)
	var out strings.Builder
	if err := questionsCmd([]string{wt}, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "for you") || !strings.Contains(lines[1], "Ana") ||
		!strings.Contains(lines[1], "[which-caption]") || !strings.Contains(lines[2], "Bo") {
		t.Fatalf("list:\n%s", out.String())
	}
}

// close takes a number or a brief, and writes the run with no agent.
func TestQuestionsCloses(t *testing.T) {
	wt := questionWorktree(t)
	var out strings.Builder
	if err := questionsCmd([]string{"close", "which-caption", "--reason", "ticket already in QA", wt}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "not answered: ticket already in QA.") {
		t.Fatalf("output %q", out.String())
	}
	state, _ := os.ReadFile(filepath.Join(wt, ".runs", "ABC-1", "STATE.md"))
	if !strings.Contains(string(state), "## Decisions\n\n- Product (Ana): which caption? Closed ") {
		t.Fatalf("state:\n%s", state)
	}
	brief, _ := os.ReadFile(filepath.Join(wt, ".runs", "ABC-1", "questions", "which-caption.md"))
	if !strings.Contains(string(brief), "status: closed") {
		t.Fatalf("brief:\n%s", brief)
	}

	out.Reset()
	if err := questionsCmd([]string{"close", "1", wt}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Which model runs the implementer? Closed ") || !strings.Contains(out.String(), "not answered: no longer needed.") {
		t.Fatalf("output %q", out.String())
	}
}

// A number or a brief that names nothing is an error, with the list.
func TestQuestionsCloseNeedsAQuestion(t *testing.T) {
	wt := questionWorktree(t)
	var out strings.Builder
	if err := questionsCmd([]string{"close", "9", wt}, &out); err == nil || !strings.Contains(out.String(), "which caption") {
		t.Fatalf("err %v, output %q", err, out.String())
	}
	if err := questionsCmd([]string{"close", "nothing", wt}, &out); err == nil {
		t.Fatal("an unknown brief closed something")
	}
	if err := questionsCmd([]string{t.TempDir()}, &out); err == nil || !strings.Contains(err.Error(), "no run") {
		t.Fatalf("no run: %v", err)
	}
}
