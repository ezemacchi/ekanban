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
	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/team"
	"github.com/ezemacchi/ekanban/internal/ticket"
	"github.com/ezemacchi/ekanban/internal/ticketui"
	"github.com/ezemacchi/ekanban/internal/ui"
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
	live, _ := ticket.ReadLive(client)

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
		info := src.Classify(ctx, path, live, 0)
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
	ci, ciProblems := pipeline.NewCI(pipeline.CIConfig{Kind: cfg.Pipeline.CI.Kind, URL: cfg.Pipeline.CI.URL})
	host, hostProblems := pipeline.Host(cfg.Pipeline.CodeHost)
	key, keyProblems := pipeline.TicketKey(cfg.Pipeline.TicketKey)
	set := pipeline.Settings{CI: ci, Host: host, TicketKey: key, Rules: rules, Teams: teams, Lead: cfg.Lead, Columns: cols}
	for _, t := range cfg.Pipeline.Targets {
		set.Targets = append(set.Targets, pipeline.Target{Branch: t.Branch, Env: t.Env, Publish: t.Publish})
	}
	for _, more := range [][]string{teamProblems, colProblems, ciProblems, hostProblems, keyProblems} {
		problems = append(problems, more...)
	}
	return set, problems
}

// ticketSettings is the ticket board's share of config.toml.
func ticketSettings(cfg config.Settings) ticketui.Settings {
	teams, problems := loadTeams()
	cols, colProblems := columns.Merge(ticket.DefaultColumns, cfg.Ticket.Columns, false)
	problems = append(problems, colProblems...)
	return ticketui.Settings{
		Options:  ticket.Options{SpecRoot: cfg.SpecClone, Teams: teams, Lead: cfg.Lead},
		Columns:  cols,
		Keys:     cfg.KeysFor("ticket"),
		Icons:    cfg.Icons,
		Problems: append(problems, unknownKeys(cfg)...),
	}
}

// unknownKeys reports [keys] names that no screen has, and [keys.board] or
// [keys.ticket] names that board does not have.
func unknownKeys(cfg config.Settings) []string {
	out := keys.Unknown(cfg.Keys, ui.BoardActions, ui.PipelineActions, ui.ArchiveActions, ticketui.Actions)
	for _, p := range keys.Unknown(cfg.ScreenKeys["board"], ui.BoardActions, ui.PipelineActions, ui.ArchiveActions) {
		out = append(out, strings.Replace(p, "[keys]", "[keys.board]", 1))
	}
	for _, p := range keys.Unknown(cfg.ScreenKeys["ticket"], ticketui.Actions) {
		out = append(out, strings.Replace(p, "[keys]", "[keys.ticket]", 1))
	}
	return out
}

// showKeys lists every action by screen: what [keys] in config.toml can bind.
func showKeys() {
	cfg := config.Load()
	screens := []struct {
		name    string
		screen  string // the [keys.<screen>] table that applies
		actions []keys.Action
	}{
		{"board (outside a repository)", "board", ui.BoardActions},
		{"board (in a repository: computed columns)", "board", ui.PipelineActions},
		{"archive of accepted tickets", "board", ui.ArchiveActions},
		{"ticket board", "ticket", ticketui.Actions},
	}
	for _, s := range screens {
		km, _ := keys.New(s.actions, cfg.KeysFor(s.screen))
		fmt.Println(s.name)
		for _, a := range km.Actions() {
			fixed := ""
			if a.Fixed {
				fixed = "  (fixed)"
			}
			fmt.Printf("  %-18s %-16s %s%s\n", a.Name, keys.Display(a.Keys), a.Help, fixed)
		}
		fmt.Println()
	}
	for _, p := range unknownKeys(cfg) {
		fmt.Println(p)
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
