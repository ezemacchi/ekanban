package ticket

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// questionRun parses state in a run folder holding the given briefs.
func questionRun(t *testing.T, state string, briefs map[string]string) *Run {
	t.Helper()
	dir := t.TempDir()
	for name, text := range briefs {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := &Run{Dir: dir, layout: DefaultLayout()}
	r.parse(state)
	r.ForYou, r.Questions, r.Closed = r.questions()
	r.readBriefs(r.ForYou)
	r.readBriefs(r.Questions)
	return r
}

const briefOpen = "---\nto: Ana\nrole: Product\nstatus: open\nasked: 2026-01-02\nchat: https://chat.example.com/new?to=ana\nneeds: small\n---\n## Question\nWhich caption?\n\n## Kickoff\nRead it.\n"

// Questions for the user are their own list, though their heading also says
// "question"; the others name who answers them.
func TestQuestionsForYouAreApart(t *testing.T) {
	r := questionRun(t, "## Questions for you\n- Which model runs the implementer?\n\n## Open decisions\n- Product (Ana): which caption?\n- Technical (Bo), after Ana answers: which key?\n", nil)
	if len(r.ForYou) != 1 || r.ForYou[0].Words != "Which model runs the implementer?" || !r.ForYou[0].ForYou() {
		t.Fatalf("for you: %+v", r.ForYou)
	}
	if len(r.Questions) != 2 || r.Questions[0].To != "Ana" || r.Questions[1].To != "Bo" {
		t.Fatalf("others: %+v", r.Questions)
	}
}

// The brief a line links gives the question its chat and who answers it; its
// link is not part of the words.
func TestABriefGivesItsChat(t *testing.T) {
	r := questionRun(t, "## Open decisions\n- Product: which caption? [brief](questions/which-caption.md)\n",
		map[string]string{"questions/which-caption.md": briefOpen})
	q := r.Questions[0]
	if q.Words != "Product: which caption?" || q.Brief == nil || q.To != "Ana" || q.Problem != "" {
		t.Fatalf("question %+v", q)
	}
	chats := q.Chats()
	if len(chats) != 1 || chats[0].URL != "https://chat.example.com/new?to=ana" || chats[0].Label != "Chat with Ana" {
		t.Fatalf("chats %+v", chats)
	}
	if q.Brief.Needs != "small" || q.Brief.Sections["Kickoff"] != "Read it." {
		t.Fatalf("brief %+v", q.Brief)
	}
}

// A word in the link does not settle a question; it is still said, so the
// link gets renamed.
func TestASettledWordInTheLinkDoesNotCloseIt(t *testing.T) {
	// "closed" in the question's own words would read as settled: that is the
	// pattern's business. Only the link must not count.
	r := questionRun(t, "## Open decisions\n- Product (Ana): which caption? [brief](questions/rules-closed.md)\n",
		map[string]string{"questions/rules-closed.md": briefOpen})
	if len(r.Questions) != 1 || r.Questions[0].Problem == "" {
		t.Fatalf("a settled word in the link: %+v", r.Questions)
	}
}

// Each thing wrong with a question is said on it.
func TestQuestionProblems(t *testing.T) {
	draft := "---\nto: Ana\nstatus: draft\n---\n"
	closed := "---\nto: Ana\nstatus: closed\nchat: https://chat.example.com/x\n---\n"
	noChat := "---\nto: Ana\nstatus: open\n---\n"
	mine := "---\nto: you\nstatus: open\n---\n"
	r := questionRun(t, "## Questions for you\n- Model? [brief](questions/mine.md)\n\n## Open decisions\n- A (Ana): one [brief](questions/missing.md)\n- A (Ana): two [brief](questions/draft.md)\n- A (Ana): three [brief](questions/closed.md)\n- A (Ana): four [brief](questions/no-chat.md)\n- A (Ana): five [brief](../outside.md)\n",
		map[string]string{"questions/draft.md": draft, "questions/closed.md": closed, "questions/no-chat.md": noChat, "questions/mine.md": mine})
	if r.ForYou[0].Problem != "" {
		t.Fatalf("a question for you needs no chat: %q", r.ForYou[0].Problem)
	}
	want := []string{"does not exist", "still a draft", "is closed", "no chat link", "cannot be read"}
	for i, w := range want {
		if p := r.Questions[i].Problem; p == "" || !strings.Contains(p, w) {
			t.Fatalf("question %d: problem %q, want %q", i, p, w)
		}
	}
	if len(r.Problems()) != len(want) {
		t.Fatalf("problems %d, want %d", len(r.Problems()), len(want))
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// A brief outside the run folder is not read.
func TestReadBriefStaysInTheRunFolder(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadBrief(filepath.Join(dir, "run"), "../other.md"); err == nil {
		t.Fatal("a brief outside the run folder was read")
	}
}
