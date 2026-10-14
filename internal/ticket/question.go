package ticket

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ezemacchi/ekanban/internal/links"
)

// A run asks people things: each question is one line of its state file,
// under a section for the user ("Questions for you") or for others ("Open
// decisions"). The line may link a brief, a file in the run folder that holds
// the question's detail -- why it blocks, its options and sources, the chat
// to ask it in -- so the state file stays short. The board shows the line,
// takes the chat from the brief, and says when the two do not agree.

// Question is one open question of a run.
type Question struct {
	Line  string       // as written, without its "- "
	Words string       // the line without its links
	Links []links.Link // the web links on the line: a chat, in older runs
	// BriefPath is the brief the line links, relative to the run folder;
	// Brief is that brief read, nil when there is none or it cannot be read.
	BriefPath string
	Brief     *Brief
	// BriefFile is the brief's full path, and FromWorktree its path from
	// the worktree (.runs/<KEY>/questions/<slug>.md), the way a message to
	// the orchestrator names it; both "" without a brief.
	BriefFile, FromWorktree string
	// To is who answers it: the brief's to, else what [run] asked names in
	// the line, "you" for a question for the user, "" when unknown.
	To string
	// Problem says what is wrong with the question, for the board to show;
	// "" when nothing is.
	Problem string
}

// Chats are where the question can be answered: the links on its line, else
// its brief's chat.
func (q Question) Chats() []links.Link {
	if len(q.Links) > 0 {
		return q.Links
	}
	if q.Brief != nil && q.Brief.Chat != "" {
		label := "Chat"
		if q.To != "" && q.To != You {
			label = "Chat with " + q.To
		}
		return []links.Link{{Label: label, URL: q.Brief.Chat}}
	}
	return nil
}

// ForYou is whether the question is for the user.
func (q Question) ForYou() bool { return q.To == You }

// You is To for a question for the user, as a brief's to: says it.
const You = "you"

// Brief is a question's brief: its front matter and its sections.
type Brief struct {
	Path                                 string // as the line links it, relative to the run folder
	To, Role, Status, Asked, Chat, Needs string
	// Sections are the brief's "## " sections by heading, their text trimmed.
	Sections map[string]string
}

// Brief statuses.
const (
	BriefDraft  = "draft"
	BriefOpen   = "open"
	BriefClosed = "closed"
)

// ReadBrief reads the brief at rel inside the run folder dir. A path that
// leaves the folder is refused.
func ReadBrief(dir, rel string) (*Brief, error) {
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if !within(path, dir) {
		return nil, fmt.Errorf("%s is outside the run folder", rel)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := &Brief{Path: rel, Sections: map[string]string{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	front, heading := false, ""
	var text []string
	flush := func() {
		if heading != "" {
			b.Sections[heading] = strings.TrimSpace(strings.Join(text, "\n"))
		}
		text = nil
	}
	for n := 0; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case n == 0 && strings.TrimSpace(line) == "---":
			front = true
		case front && strings.TrimSpace(line) == "---":
			front = false
		case front:
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "to":
				b.To = v
			case "role":
				b.Role = v
			case "status":
				b.Status = strings.ToLower(v)
			case "asked":
				b.Asked = v
			case "chat":
				b.Chat = v
			case "needs":
				b.Needs = strings.ToLower(v)
			}
		case strings.HasPrefix(line, "## "):
			flush()
			heading = strings.TrimSpace(line[3:])
		default:
			text = append(text, line)
		}
	}
	flush()
	return b, sc.Err()
}

// newQuestion is a line of an open section, for the user or not.
func (l Layout) newQuestion(line string, forYou bool) Question {
	words, found := links.InText(line)
	q := Question{Line: line, Words: words, Links: found}
	if files := links.Files(line); len(files) > 0 {
		q.BriefPath = files[0].URL
	}
	switch {
	case forYou:
		q.To = You
	case l.asked != nil:
		if m := l.asked.FindStringSubmatch(words); len(m) > 1 {
			q.To = strings.TrimSpace(m[1])
		}
	}
	return q
}

// settledLine is whether a line of an open section is already answered. It
// reads the line's words, not its links: a brief named after "closed
// accounts" does not close its question.
func (l Layout) settledLine(q Question) bool {
	return l.settled.MatchString("- " + q.Words)
}

// readBriefs reads the brief of each question and says what is wrong with
// it: a brief that is missing, still a draft or already closed, an open one
// with no chat for someone other than the user, or a line the settled
// pattern would have hidden only because of a word in its link.
func (r *Run) readBriefs(qs []Question) {
	for i := range qs {
		q := &qs[i]
		if r.layout.settled.MatchString("- "+q.Line) && !r.layout.settledLine(*q) {
			q.Problem = "a word in its link reads as answered; rename the link"
		}
		if q.BriefPath == "" {
			continue
		}
		q.BriefFile = filepath.Join(r.Dir, filepath.FromSlash(q.BriefPath))
		q.FromWorktree = filepath.ToSlash(q.BriefPath)
		if rel, err := filepath.Rel(r.Worktree, q.BriefFile); r.Worktree != "" && err == nil {
			q.FromWorktree = filepath.ToSlash(rel)
		}
		b, err := ReadBrief(r.Dir, q.BriefPath)
		if err != nil {
			q.Problem = "its brief " + q.BriefPath + " cannot be read"
			if os.IsNotExist(err) {
				q.Problem = "its brief " + q.BriefPath + " does not exist"
			}
			continue
		}
		q.Brief = b
		if b.To != "" && !q.ForYou() {
			q.To = b.To
			if strings.EqualFold(b.To, You) {
				q.To = You
			}
		}
		switch {
		case b.Status == BriefDraft:
			q.Problem = "its brief is still a draft"
		case b.Status == BriefClosed:
			q.Problem = "its brief is closed; the line belongs in Decisions"
		case b.Chat == "" && len(q.Links) == 0 && !q.ForYou() && !strings.EqualFold(b.To, You):
			q.Problem = "its brief has no chat link"
		}
	}
}

// Problems are the run's questions that something is wrong with.
func (r *Run) Problems() []Question {
	var out []Question
	for _, q := range append(append([]Question(nil), r.ForYou...), r.Questions...) {
		if q.Problem != "" {
			out = append(out, q)
		}
	}
	return out
}

// NewQuestion is line read as an open question for someone else, with the
// default layout: what the parser makes of a line of "## Open decisions".
func NewQuestion(line string) Question { return DefaultLayout().newQuestion(line, false) }

// ByWho is qs grouped by who answers them, the unknown last, each group in
// the run's order: how the boards list the others' questions.
func ByWho(qs []Question) []Question {
	out := append([]Question(nil), qs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].To, out[j].To
		if (a == "") != (b == "") {
			return b == ""
		}
		return a < b
	})
	return out
}

// Open are the run's open questions in the order the boards list them: those
// for the user, then the others' by who answers them.
func (r *Run) Open() []Question {
	return append(append([]Question(nil), r.ForYou...), ByWho(r.Questions)...)
}
