package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/config"
	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/state"
	"github.com/ezemacchi/ekanban/internal/store"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// `ekanban state` is the repository board as data: what an agent or a script
// reads to decide what to do next, instead of scraping a board or every
// run's files.

const stateUsage = `usage:
  ekanban state [--json] [repo]

One row per ticket worktree of the repository: the branch against its
target, the pull request, the column, the orchestrator and the open
questions. The repository defaults to the current folder. Without --json
it prints a short table.`

// stateCmd runs `ekanban state`, writing to out.
func stateCmd(args []string, out io.Writer) error {
	asJSON := false
	var rest []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(out, stateUsage)
			return nil
		case a == "--json":
			asJSON = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown option %s\n%s", a, stateUsage)
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 1 {
		return fmt.Errorf("%s", stateUsage)
	}
	repo, err := repoArg(rest)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	report, err := readState(ctx, repo)
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	writeState(out, report)
	return nil
}

// readState classifies every ticket worktree of repo the way the board does.
// Herdr is optional: outside a session the rows have no agents, and the
// report says why.
func readState(ctx context.Context, repo string) (state.Report, error) {
	src := repoSource(repo)
	src.Refresh(ctx)

	report := state.Report{Repo: repo, Targets: src.Targets(), Offline: src.Offline(), Tickets: []state.Ticket{}}
	live := ticket.Live{}
	if client, err := herdr.New(); err != nil {
		report.Herdr = "no agents: " + err.Error()
	} else if live, err = ticket.ReadLive(client); err != nil {
		report.Herdr = "no agents: " + err.Error()
	}

	porcelain, err := runGit(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return report, fmt.Errorf("%s is not a git repository: %w", repo, err)
	}
	var paths []string
	for _, path := range state.Worktrees(porcelain) {
		paths = append(paths, filepath.Clean(path))
	}
	report.Tickets = ticketRows(ctx, src, live, paths, knownPRs())
	return report, nil
}

// ticketRows classifies each of paths and keeps the ticket worktrees.
func ticketRows(ctx context.Context, src *pipeline.Source, live ticket.Live, paths []string, known map[string]int) []state.Ticket {
	out := []state.Ticket{}
	for _, path := range paths {
		info := src.Classify(ctx, path, live, known[store.Key(path)])
		if info.Key == "" {
			continue
		}
		target, from := state.TargetOf(info.Target, src.Targets())
		facts := state.Facts(runGit, path, target, from)
		out = append(out, state.Row(path, ticket.ReadBranch(path), live.WorkspaceOf(path), info, facts, src.Columns()))
	}
	return out
}

// knownPRs are the pull requests the board remembers by worktree, for when
// the build server has dropped a closed one's job.
func knownPRs() map[string]int {
	known := map[string]int{}
	if board, err := store.Load(); err == nil {
		for key, e := range board.Entries {
			known[key] = e.PR
		}
	}
	return known
}

// repoSource is the pipeline of repo with its config.toml and .ekanban.toml.
func repoSource(repo string) *pipeline.Source {
	cfg := config.LoadFor(repo)
	defaultTarget(&cfg, repo)
	set, _ := pipelineSettings(cfg)
	return pipeline.New(repo, set, gh.HideWindow)
}

// runGit is state.Git over the git on PATH.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	gh.HideWindow(cmd)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// writeState prints the report as a short table, one ticket a line.
func writeState(out io.Writer, r state.Report) {
	for _, why := range []string{r.Offline, r.Herdr} {
		if why != "" {
			fmt.Fprintln(out, "note: "+why)
		}
	}
	if len(r.Tickets) == 0 {
		fmt.Fprintf(out, "no ticket worktrees in %s\n", r.Repo)
		return
	}
	for _, t := range r.Tickets {
		fmt.Fprintln(out, stateLine(t))
	}
}

// stateLine is one ticket in a line: key, column, pull request, branch,
// orchestrator and questions.
func stateLine(t state.Ticket) string {
	col := t.ColumnLabel
	if col == "" {
		col = t.Column
	}
	parts := []string{fmt.Sprintf("%-10s %-10s", t.Key, col)}
	if t.PR != nil {
		pr := fmt.Sprintf("PR %d", t.PR.Number)
		switch {
		case t.PR.MergedTo != "":
			pr += " merged to " + t.PR.MergedTo
		case t.PR.Open:
			pr += " open"
		}
		if t.PR.Build != "" {
			pr += " " + t.PR.Build
		}
		parts = append(parts, pr)
	}
	g := t.Git
	if g.Compared != "" {
		parts = append(parts, fmt.Sprintf("%d behind / %d ahead of %s", g.Behind, g.Ahead, g.Compared))
	}
	if g.Dirty > 0 {
		parts = append(parts, fmt.Sprintf("%d uncommitted", g.Dirty))
	}
	if g.Unpushed > 0 {
		parts = append(parts, fmt.Sprintf("%d unpushed", g.Unpushed))
	}
	if t.Orchestrator != nil {
		parts = append(parts, "orchestrator "+t.Orchestrator.Status)
	}
	if n := len(t.Questions.ForYou); n > 0 {
		parts = append(parts, fmt.Sprintf("%d for you", n))
	}
	if n := len(t.Questions.Others); n > 0 {
		parts = append(parts, fmt.Sprintf("%d for others", n))
	}
	return strings.Join(parts, "  ·  ")
}
