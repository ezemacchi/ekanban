package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/config"
	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/state"
)

// `ekanban broadcast` is the board's broadcast (B) from a terminal, and
// `ekanban update-branches` its U: what an agent working for the user runs
// instead of prompting every orchestrator by hand.

const broadcastUsage = `usage:
  ekanban broadcast --message "text" [--filter <f>]... [--yes] [repo]

Sends the message to the orchestrator of each ticket every filter matches,
each as a message of its own. {target} and {key} become the ticket's target
branch and key. An orchestrator waiting at a prompt never gets it.

Filters: all (the default), pr (a pull request still open), idle, working,
done (what the orchestrator is doing), column=<id or name>, key=<KEY,...>.

Without --yes it lists who gets what and asks before sending.`

const updateBranchesUsage = `usage:
  ekanban update-branches [repo]

Fetches the [[pipeline.target]] branches and fast-forwards each local one
that is behind its remote. A branch with commits of its own, or checked out
where there are changes, is left alone and said so.`

// broadcastCmd runs `ekanban broadcast`: in answers the confirmation.
func broadcastCmd(args []string, in io.Reader, out io.Writer) error {
	var text string
	var filters []state.Filter
	yes := false
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func(name string) (string, error) {
			if v, ok := strings.CutPrefix(a, name+"="); ok {
				return v, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value\n%s", name, broadcastUsage)
			}
			i++
			return args[i], nil
		}
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(out, broadcastUsage)
			return nil
		case a == "--yes" || a == "-y":
			yes = true
		case a == "--message" || strings.HasPrefix(a, "--message="):
			v, err := value("--message")
			if err != nil {
				return err
			}
			text = v
		case a == "--filter" || strings.HasPrefix(a, "--filter="):
			v, err := value("--filter")
			if err != nil {
				return err
			}
			f, err := state.ParseFilter(v)
			if err != nil {
				return err
			}
			filters = append(filters, f)
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown option %s\n%s", a, broadcastUsage)
		default:
			rest = append(rest, a)
		}
	}
	text = strings.TrimSpace(text)
	if text == "" || len(rest) > 1 {
		return errors.New(broadcastUsage)
	}
	repo, err := repoArg(rest)
	if err != nil {
		return err
	}
	client, err := herdr.New()
	if err != nil {
		return fmt.Errorf("a broadcast needs Herdr: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	report, err := readState(ctx, repo)
	if err != nil {
		return err
	}
	if report.Herdr != "" {
		return errors.New(report.Herdr)
	}
	fallback := ""
	if len(report.Targets) > 0 {
		fallback = report.Targets[0]
	}
	to, skipped := state.Recipients(report.Tickets, filters, text, fallback)
	for _, s := range skipped {
		fmt.Fprintf(out, "not sent to %s: %s\n", s.Key, s.Why)
	}
	if len(to) == 0 {
		return errors.New("no orchestrator to send to")
	}
	for _, r := range to {
		fmt.Fprintf(out, "%-10s %-8s %s\n", r.Key, r.Status, r.Message)
	}
	if !yes && !confirm(in, out, fmt.Sprintf("Send to %d orchestrator(s)? [y/N] ", len(to))) {
		fmt.Fprintln(out, "nothing was sent")
		return nil
	}
	return sendAll(out, to, client.SubmitToAgent)
}

// sendAll submits each recipient's message, and fails when any was not sent.
func sendAll(out io.Writer, to []state.Recipient, submit func(pane, text string) error) error {
	var failed []string
	for _, r := range to {
		err := submit(r.Pane, r.Message)
		switch {
		case err == nil:
			fmt.Fprintf(out, "sent to %s\n", r.Key)
		case herdr.IsBlocked(err):
			failed = append(failed, r.Key)
			fmt.Fprintf(out, "not sent to %s: waiting at a prompt\n", r.Key)
		default:
			failed = append(failed, r.Key)
			fmt.Fprintf(out, "could not reach %s: %v\n", r.Key, err)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("not sent to %s", strings.Join(failed, ", "))
	}
	return nil
}

// confirm asks question and reads a yes; anything else, or no answer at all,
// is a no.
func confirm(in io.Reader, out io.Writer, question string) bool {
	fmt.Fprint(out, question)
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

// updateBranchesCmd runs `ekanban update-branches`.
func updateBranchesCmd(args []string, out io.Writer) error {
	var rest []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(out, updateBranchesUsage)
			return nil
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown option %s\n%s", a, updateBranchesUsage)
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 1 {
		return errors.New(updateBranchesUsage)
	}
	repo, err := repoArg(rest)
	if err != nil {
		return err
	}
	cfg := config.LoadFor(repo)
	defaultTarget(&cfg, repo)
	set, _ := pipelineSettings(cfg)
	src := pipeline.New(repo, set, gh.HideWindow)
	targets := src.Targets()
	if len(targets) == 0 {
		return errors.New("no [[pipeline.target]] branches to bring up to date")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r := src.Sync(ctx)
	for _, m := range r.Moved {
		fmt.Fprintln(out, "moved "+m)
	}
	for _, l := range r.Left {
		fmt.Fprintln(out, "left "+l)
	}
	if len(r.Moved)+len(r.Left) == 0 {
		fmt.Fprintln(out, strings.Join(targets, " and ")+" already match origin")
	}
	if len(r.Left) > 0 {
		return fmt.Errorf("%d branch(es) left alone", len(r.Left))
	}
	return nil
}

// repoArg is the repository a command was given, the current folder when none.
func repoArg(rest []string) (string, error) {
	repo := "."
	if len(rest) == 1 {
		repo = rest[0]
	}
	return filepath.Abs(repo)
}
