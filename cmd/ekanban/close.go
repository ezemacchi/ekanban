package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/closing"
	"github.com/ezemacchi/ekanban/internal/config"
	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/store"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

const closeUsage = `usage:
  ekanban close [--yes] <worktree>

Closes a finished ticket, in this order, stopping at the first check that
fails with nothing changed:

  1. no uncommitted changes in the worktree
  2. every commit is on a remote
  3. its pull request merged, or its branch already in its target
  4. [close] check in config.toml, if set, exits 0
  5. accept it into the Archive, with a copy of the run's notes
  6. close its Herdr workspaces
  7. git worktree remove

It lists what it found and asks first unless --yes. Run it from outside the
ticket's workspace: closing that one would stop the close halfway.`

// closeCmd runs `ekanban close`: in answers the confirmation.
func closeCmd(args []string, in io.Reader, out io.Writer) error {
	yes := false
	var rest []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(out, closeUsage)
			return nil
		case a == "--yes" || a == "-y":
			yes = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown option %s\n%s", a, closeUsage)
		default:
			rest = append(rest, a)
		}
	}
	// The worktree is never taken from the current folder: removing the
	// folder you stand in is not something to do by leaving out an argument.
	if len(rest) != 1 {
		return errors.New(closeUsage)
	}
	worktree, err := filepath.Abs(rest[0])
	if err != nil {
		return err
	}
	repo := repoOf(worktree)
	if repo == "" {
		return fmt.Errorf("%s is not a git checkout", worktree)
	}
	if store.SameDir(repo, worktree) {
		return fmt.Errorf("%s is the repository's main checkout, not a ticket worktree", worktree)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	src := repoSource(repo)
	src.Refresh(ctx)
	client, herr := herdr.New()
	live := ticket.Live{}
	if herr == nil {
		live, _ = ticket.ReadLive(client)
	} else {
		client = nil
	}
	rows := ticketRows(ctx, src, live, []string{worktree}, knownPRs())
	if len(rows) == 0 {
		return fmt.Errorf("%s has no ticket run", worktree)
	}
	t := rows[0]
	plan := closing.Plan{
		Ticket:     t,
		Repo:       repo,
		Workspaces: closing.WorkspacesOn(live, worktree),
		Self:       os.Getenv("HERDR_WORKSPACE_ID"),
		Check:      config.LoadFor(repo).CloseCheck,
	}
	if len(plan.Workspaces) > 0 && client == nil {
		return fmt.Errorf("workspaces are open on %s but Herdr is not reachable to close them: %v", worktree, herr)
	}

	dir, err := stateDir()
	if err != nil {
		return err
	}
	env := closing.Real(client, dir, gh.HideWindow, nil)

	title := []rune(firstNonEmptyStr(t.Title, t.Branch))
	if len(title) > 100 {
		title = append(title[:99], '…')
	}
	fmt.Fprintf(out, "%s · %s\n", t.Key, string(title))
	steps, err := closing.Checks(env, plan)
	for _, s := range steps {
		fmt.Fprintf(out, "  ok  %s: %s\n", s.Name, s.Note)
	}
	if err != nil {
		return fmt.Errorf("not closed: %w", err)
	}
	ws := "no Herdr workspace open"
	if len(plan.Workspaces) > 0 {
		ws = "close workspace " + strings.Join(plan.Workspaces, ", ")
	}
	fmt.Fprintf(out, "Then: accept into the Archive, %s, remove %s.\n", ws, worktree)
	if !yes && !confirm(in, out, "Close it? [y/N] ") {
		fmt.Fprintln(out, "nothing was closed")
		return nil
	}
	// Close runs the checks again: a minute may have passed at the question.
	return closing.Close(env, plan, func(s closing.Step) {
		fmt.Fprintf(out, "  ok  %s: %s\n", s.Name, s.Note)
	})
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
