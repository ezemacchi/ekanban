package ticket

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// A question can be closed without an answer -- the ticket moved on and
// nobody needs it any more -- from the boards. With no orchestrator running
// there is no other writer of the run's files, so the board closes it itself:
// its line moves from its open section to the decisions, saying it was not
// answered and why, and its brief is closed with the same outcome. With an
// orchestrator running the boards ask it instead (orchestrator.Questions).

// ErrQuestionMoved is CloseQuestion finding the state file no longer holds
// the question's line as it was read: someone changed it since.
var ErrQuestionMoved = errors.New("the question is no longer in the state file as it was read; refresh and try again")

// OrchestratorReadsIt is what the question whether to close adds when the
// run's orchestrator is running: it is not told, and needs not be.
const OrchestratorReadsIt = "The orchestrator is running; it reads the question closed the next time it reads the state file. Nothing is sent to it."

// ClosedLine is the line a question closed without an answer takes in the
// decisions: its words, when and why, and its brief.
func ClosedLine(q Question, reason string, on time.Time) string {
	line := fmt.Sprintf("%s Closed %s by the user (board), not answered: %s.", strings.TrimRight(q.Words, " "), on.Format("2006-01-02"), strings.TrimRight(reason, ". "))
	if q.BriefPath != "" {
		line += " [brief](" + q.BriefPath + ")"
	}
	return line
}

// CloseQuestion closes q in run without an answer: its line moves to the
// first decisions section (one is added at the end when there is none) as
// ClosedLine, and its brief, if it has one, gets status: closed and the
// outcome. The files keep their line endings.
func CloseQuestion(run *Run, q Question, reason string, on time.Time) error {
	path := run.StatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text, eol := string(data), "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	at := -1
	for i, l := range lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "- ") && strings.TrimSpace(t[2:]) == q.Line {
			at = i
			break
		}
	}
	if at < 0 {
		return ErrQuestionMoved
	}
	lines = append(lines[:at], lines[at+1:]...)
	closed := "- " + ClosedLine(q, reason, on)

	// After the last bullet of the first decisions section, or right under
	// its heading when it has none.
	l, insert := run.Layout(), -1
	for i, line := range lines {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		if l.closed != nil && l.closed.MatchString(strings.TrimSpace(line[3:])) {
			insert = i + 1
			for j := i + 1; j < len(lines) && !strings.HasPrefix(lines[j], "## "); j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), "- ") {
					insert = j + 1
				}
			}
			break
		}
	}
	if insert < 0 {
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, "", "## Decisions", "", closed, "")
	} else {
		lines = append(lines[:insert], append([]string{closed}, lines[insert:]...)...)
	}
	if err := writeFile(path, strings.Join(lines, eol)); err != nil {
		return err
	}
	if q.BriefFile == "" {
		return nil
	}
	return closeBrief(q.BriefFile, fmt.Sprintf("Closed %s by the user from the board, not answered: %s.", on.Format("2006-01-02"), strings.TrimRight(reason, ". ")))
}

// closeBrief sets a brief's status to closed and writes outcome under its
// ## Outcome section, added at the end when it has none.
func closeBrief(path, outcome string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text, eol := string(data), "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	front, done := false, false
	for i, l := range lines {
		switch {
		case i == 0 && strings.TrimSpace(l) == "---":
			front = true
		case front && strings.TrimSpace(l) == "---":
			front = false
		case front && strings.HasPrefix(strings.ToLower(strings.TrimSpace(l)), "status:"):
			lines[i] = "status: " + BriefClosed
		case !done && strings.TrimSpace(l) == "## Outcome":
			lines = append(lines[:i+1], append([]string{"", outcome}, lines[i+1:]...)...)
			done = true
		}
	}
	if !done {
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, "", "## Outcome", "", outcome, "")
	}
	return writeFile(path, strings.Join(lines, eol))
}

// writeFile replaces path through a file beside it, so a reader never sees
// it half written.
func writeFile(path, text string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
