package stack

import (
	"fmt"
	"regexp"
	"strings"
)

// The failed-tests source stacks the tests linked to a card's ticket that QA
// failed or blocked. Which ones depends on how a team records it:
//
//	from = "comment"  a test named, by its key or by the [code] in its
//	                  summary, on a comment line that says failed or blocked;
//	                  a later line that says passed takes it off again
//	from = "linked"   every linked test, for teams that only link a test
//	                  once it failed
//
// The words are regular expressions, so a team writing in another language
// or another way sets its own.
func init() { Register("failed-tests", newFailedTests) }

const (
	fromComment = "comment"
	fromLinked  = "linked"
)

// Default words of the failed-tests source.
const (
	DefaultFailed  = `(?i)\bfail(ed|s|ing|ure)?\b`
	DefaultBlocked = `(?i)\bblock(ed|s|ing)?\b`
	DefaultPassed  = `(?i)\bpass(ed|es)?\b`
)

type failedTests struct {
	from                    string
	failed, blocked, passed *regexp.Regexp
}

// testCode is the code a test's summary carries in brackets, [TC-01] say,
// which QA often writes instead of its key.
var testCode = regexp.MustCompile(`\[([A-Za-z]+-[A-Za-z0-9-]+)\]`)

func newFailedTests(opts map[string]string) (Source, []string) {
	var problems []string
	s := &failedTests{from: fromComment}
	switch from := strings.TrimSpace(opts["from"]); from {
	case "", fromComment:
	case fromLinked:
		s.from = fromLinked
	default:
		problems = append(problems, fmt.Sprintf("from %q is not %q or %q — using %q", from, fromComment, fromLinked, fromComment))
	}
	word := func(name, def string) *regexp.Regexp {
		v := opts[name]
		if v == "" {
			return regexp.MustCompile(def)
		}
		re, err := regexp.Compile(v)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s %q is not a regular expression — using the default", name, v))
			return regexp.MustCompile(def)
		}
		return re
	}
	s.failed, s.blocked, s.passed = word("failed", DefaultFailed), word("blocked", DefaultBlocked), word("passed", DefaultPassed)
	for name := range opts {
		switch name {
		case "from", "failed", "blocked", "passed":
		default:
			problems = append(problems, fmt.Sprintf("%s is not a setting of this source (known: from, failed, blocked, passed) — ignored", name))
		}
	}
	return s, problems
}

func (s *failedTests) Name() string { return "failed-tests" }

func (s *failedTests) Needs(c Card) bool { return c.Key != "" }

func (s *failedTests) Items(c Card, l Lookup) []Item {
	if c.Key == "" || l == nil {
		return nil
	}
	t, ok := l.Ticket(c.Key)
	if !ok || len(t.Tests) == 0 {
		return nil
	}
	if s.from == fromLinked {
		out := make([]Item, 0, len(t.Tests))
		for _, test := range t.Tests {
			out = append(out, item(test, "failed", ""))
		}
		return out
	}

	// Later comments win: a test failed, then fixed and passed, is off.
	word := map[string]string{}
	evidence := map[string]string{}
	for _, n := range t.Notes {
		for _, line := range strings.Split(strings.ReplaceAll(n.Body, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			w := s.verdict(line)
			if w == "" {
				continue
			}
			for _, test := range t.Tests {
				if names(line, test, c.Key) {
					word[test.Key] = w
					evidence[test.Key] = noteLine(n, line)
				}
			}
		}
	}
	var out []Item
	for _, test := range t.Tests {
		if w := word[test.Key]; w == "failed" || w == "blocked" {
			out = append(out, item(test, w, evidence[test.Key]))
		}
	}
	return out
}

// verdict is what line says of the tests it names: the word of whichever of
// failed, blocked and passed comes first in it, "" for none.
func (s *failedTests) verdict(line string) string {
	best, at := "", len(line)+1
	for _, c := range []struct {
		word string
		re   *regexp.Regexp
	}{{"failed", s.failed}, {"blocked", s.blocked}, {"passed", s.passed}} {
		if loc := c.re.FindStringIndex(line); loc != nil && loc[0] < at {
			best, at = c.word, loc[0]
		}
	}
	return best
}

// names reports whether line names test: by its key, or by the code in its
// summary, neither as part of a longer one. Summaries often carry the
// story's key in brackets too ("[ABC-1][TC-01] ..."); that code names the
// story, not the test, so it is skipped.
func names(line string, test Linked, story string) bool {
	upper := strings.ToUpper(line)
	if within(upper, strings.ToUpper(test.Key)) {
		return true
	}
	for _, m := range testCode.FindAllStringSubmatch(test.Summary, -1) {
		if strings.EqualFold(m[1], story) {
			continue
		}
		if within(upper, strings.ToUpper(m[1])) {
			return true
		}
	}
	return false
}

// within reports whether word is in s with no letter or digit glued to
// either end, so ABC-1 is not found in ABC-12.
func within(s, word string) bool {
	if word == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(s[from:], word)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(word)
		if (i == 0 || !alnum(s[i-1])) && (end == len(s) || !alnum(s[end])) {
			return true
		}
		from = i + 1
	}
}

func alnum(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func noteLine(n Note, line string) string {
	who := n.Author
	if !n.At.IsZero() {
		who = strings.TrimSpace(who + " · " + n.At.Local().Format("2006-01-02"))
	}
	if who == "" {
		return line
	}
	return who + ": " + line
}

func item(t Linked, word, evidence string) Item {
	return Item{Key: t.Key, Word: word, Summary: t.Summary, Status: t.Status, URL: t.URL, Evidence: evidence, Attention: word == "failed"}
}
