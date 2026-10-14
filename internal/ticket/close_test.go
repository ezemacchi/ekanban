package ticket

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var closedOn = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Closing moves the question's line to the decisions, saying it was not
// answered and why, and closes its brief with the same outcome.
func TestCloseQuestionMovesItToTheDecisions(t *testing.T) {
	state := "## Open decisions\n- Product (Ana): which caption? [brief](questions/which-caption.md)\n- Technical (Bo): which key?\n\n## Decisions\n- Product (Ana): show the count? Closed 2026-01-02 by Ana (chat): yes.\n\n## Landed\n"
	r := questionRun(t, state, map[string]string{"questions/which-caption.md": briefOpen + "\n## Outcome\n\n## Kickoff\nRead it.\n"})
	r.layout = DefaultLayout()
	if err := os.WriteFile(r.StatePath(), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CloseQuestion(r, r.Questions[0], "no longer needed", closedOn); err != nil {
		t.Fatal(err)
	}
	got := read(t, r.StatePath())
	want := "## Open decisions\n- Technical (Bo): which key?\n\n## Decisions\n- Product (Ana): show the count? Closed 2026-01-02 by Ana (chat): yes.\n" +
		"- Product (Ana): which caption? Closed 2026-10-09 by the user (board), not answered: no longer needed. [brief](questions/which-caption.md)\n\n## Landed\n"
	if got != want {
		t.Fatalf("state:\n%s\nwant:\n%s", got, want)
	}
	brief := read(t, filepath.Join(r.Dir, "questions", "which-caption.md"))
	if !strings.Contains(brief, "status: closed") || !strings.Contains(brief, "## Outcome\n\nClosed 2026-10-09 by the user from the board, not answered: no longer needed.") {
		t.Fatalf("brief:\n%s", brief)
	}

	// Read again, the question is closed, not open.
	r2 := questionRun(t, got, nil)
	if len(r2.Questions) != 1 || len(r2.Closed) != 2 {
		t.Fatalf("after closing: open %+v, closed %q", r2.Questions, r2.Closed)
	}
}

// With no decisions section one is added at the end; Windows line endings
// stay.
func TestCloseQuestionAddsTheDecisions(t *testing.T) {
	state := "## Open decisions\r\n- Technical (Bo): which key?\r\n"
	r := questionRun(t, strings.ReplaceAll(state, "\r\n", "\n"), nil)
	if err := os.WriteFile(r.StatePath(), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CloseQuestion(r, r.Questions[0], "ticket already in QA.", closedOn); err != nil {
		t.Fatal(err)
	}
	want := "## Open decisions\r\n\r\n## Decisions\r\n\r\n- Technical (Bo): which key? Closed 2026-10-09 by the user (board), not answered: ticket already in QA.\r\n"
	if got := read(t, r.StatePath()); got != want {
		t.Fatalf("state %q\nwant  %q", got, want)
	}
}

// A line changed since the board read it is not touched.
func TestCloseQuestionRefusesAChangedLine(t *testing.T) {
	r := questionRun(t, "## Open decisions\n- Technical (Bo): which key?\n", nil)
	if err := os.WriteFile(r.StatePath(), []byte("## Open decisions\n- Technical (Bo): which key, and where?\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CloseQuestion(r, r.Questions[0], "no longer needed", closedOn); !errors.Is(err, ErrQuestionMoved) {
		t.Fatalf("err = %v", err)
	}
}
