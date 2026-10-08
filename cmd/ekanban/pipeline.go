package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/config"
	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/team"
	"github.com/ezemacchi/ekanban/internal/ticket"
	"github.com/ezemacchi/ekanban/internal/ticketui"
)

func dumpPipeline(client *herdr.Client, repo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	set, problems := pipelineSettings(config.Load())
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p)
	}
	src := pipeline.New(repo, set, gh.HideWindow)
	src.Refresh(ctx)
	if why := src.Offline(); why != "" {
		fmt.Fprintln(os.Stderr, why)
	}
	agents, _ := client.Agents()

	cmd := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "list", "--porcelain")
	gh.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	for _, line := range strings.Split(string(out), "\n") {
		path, ok := strings.CutPrefix(strings.TrimSpace(line), "worktree ")
		if !ok {
			continue
		}
		info := src.Classify(ctx, path, agents, 0)
		if err := enc.Encode(map[string]any{"worktree": path, "info": info}); err != nil {
			return err
		}
	}
	return nil
}

// pipelineSettings turns config.toml's [pipeline] into the classifier's
// settings, and the links into the boards' URL templates.
func pipelineSettings(cfg config.Settings) (pipeline.Settings, []string) {
	links.Configure(cfg.IssueURL, cfg.Pipeline.PRURL)
	rules, problems := pipeline.Chain(cfg.Pipeline.Rules)
	teams, teamProblems := loadTeams()
	cols, colProblems := columns.Merge(pipeline.DefaultColumns, cfg.Pipeline.Columns, true)
	set := pipeline.Settings{PRJobs: cfg.Pipeline.JenkinsPRJobs, Rules: rules, Teams: teams, Columns: cols}
	for _, t := range cfg.Pipeline.Targets {
		set.Targets = append(set.Targets, pipeline.Target{Branch: t.Branch, Env: t.Env, Publish: t.Publish})
	}
	problems = append(problems, teamProblems...)
	return set, append(problems, colProblems...)
}

// ticketSettings is the ticket board's share of config.toml.
func ticketSettings(cfg config.Settings) ticketui.Settings {
	teams, problems := loadTeams()
	cols, colProblems := columns.Merge(ticket.DefaultColumns, cfg.Ticket.Columns, false)
	return ticketui.Settings{
		Options:  ticket.Options{SpecRoot: cfg.SpecClone, Teams: teams},
		Columns:  cols,
		Icons:    cfg.Icons,
		Problems: append(problems, colProblems...),
	}
}

// loadTeams reads the built-in team definitions plus the user's, from the
// teams folder beside config.toml.
func loadTeams() ([]team.Team, []string) {
	dir, err := config.Dir()
	if err != nil {
		return team.Load("")
	}
	return team.Load(filepath.Join(dir, "teams"))
}
