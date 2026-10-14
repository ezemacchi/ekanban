package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/config"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// `ekanban questions` lists a run's open questions, and `ekanban questions
// close` closes one without an answer, from a terminal: what x does on the
// boards, with no board, no Herdr and no agent. Closing a question nobody
// needs any more is moving one line; it is not worth an agent's tokens.

const questionsUsage = `usage:
  ekanban questions [worktree]
  ekanban questions close <number | brief> [--reason "why"] [worktree]

The worktree defaults to the current folder. <brief> is the brief's file
name without .md. The reason defaults to "no longer needed".`

// questionsCmd runs `ekanban questions`, writing to out.
func questionsCmd(args []string, out io.Writer) error {
	closing := len(args) > 0 && args[0] == "close"
	if closing {
		args = args[1:]
	}
	reason := "no longer needed"
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-h" || a == "--help":
			fmt.Fprintln(out, questionsUsage)
			return nil
		case a == "--reason" && i+1 < len(args):
			i++
			reason = args[i]
		case strings.HasPrefix(a, "--reason="):
			reason = strings.TrimPrefix(a, "--reason=")
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown option %s\n%s", a, questionsUsage)
		default:
			rest = append(rest, a)
		}
	}
	which := ""
	if closing {
		if len(rest) == 0 {
			return errors.New(questionsUsage)
		}
		which, rest = rest[0], rest[1:]
	}
	if len(rest) > 1 {
		return errors.New(questionsUsage)
	}
	worktree := ""
	if len(rest) == 1 {
		worktree = rest[0]
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		worktree = wd
	}

	run, err := ticket.Load(worktree, ticketSettings(config.LoadFor(worktree)).Options, ticket.Live{})
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no run in %s", worktree)
	}
	if err != nil {
		return err
	}
	open := run.Open()
	if !closing {
		listQuestions(out, run.Key, open)
		return nil
	}
	q, err := pickQuestion(open, which)
	if err != nil {
		listQuestions(out, run.Key, open)
		return err
	}
	now := time.Now()
	if err := ticket.CloseQuestion(run, q, strings.TrimSpace(reason), now); err != nil {
		return err
	}
	fmt.Fprintln(out, "closed: "+ticket.ClosedLine(q, strings.TrimSpace(reason), now))
	return nil
}

// listQuestions writes the open questions, numbered, with who answers each,
// its brief and what is wrong with it.
func listQuestions(out io.Writer, key string, open []ticket.Question) {
	if len(open) == 0 {
		fmt.Fprintf(out, "%s has no open question\n", key)
		return
	}
	for i, q := range open {
		who := q.To
		switch {
		case q.ForYou():
			who = "for you"
		case who == "":
			who = "-"
		}
		line := fmt.Sprintf("%2d  %-10s %s", i+1, who, q.Words)
		if s := briefSlug(q); s != "" {
			line += "  [" + s + "]"
		}
		if q.Problem != "" {
			line += "  ! " + q.Problem
		}
		fmt.Fprintln(out, line)
	}
}

// briefSlug is the question's brief's file name without .md, "" for none.
func briefSlug(q ticket.Question) string {
	if q.BriefPath == "" {
		return ""
	}
	return strings.TrimSuffix(path.Base(q.BriefPath), ".md")
}

// pickQuestion is the open question which names: its number in the list, or
// its brief's slug.
func pickQuestion(open []ticket.Question, which string) (ticket.Question, error) {
	if n, err := strconv.Atoi(which); err == nil {
		if n < 1 || n > len(open) {
			return ticket.Question{}, fmt.Errorf("no open question %d (there are %d)", n, len(open))
		}
		return open[n-1], nil
	}
	var found []ticket.Question
	for _, q := range open {
		if briefSlug(q) == which {
			found = append(found, q)
		}
	}
	switch len(found) {
	case 0:
		return ticket.Question{}, fmt.Errorf("no open question has the brief %q", which)
	case 1:
		return found[0], nil
	}
	return ticket.Question{}, fmt.Errorf("%d open questions have the brief %q; close one by its number", len(found), which)
}
