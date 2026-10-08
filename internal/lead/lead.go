// Package lead takes the user to the agent running a ticket (the team's lead),
// opening one when none is open: a
// tab in the ticket's workspace, the agent the configuration names, and its
// prompt written to the run folder.
package lead

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Herdr is what Go needs from the Herdr client.
type Herdr interface {
	Agents() ([]herdr.Agent, error)
	FocusAgent(paneID string) error
	CreateTab(workspaceID, cwd, label string) (tabID, paneID string, err error)
	StartAgent(name, kind, paneID string, args []string) error
	PromptAgent(target, text string) error
}

// Go focuses run's lead, or opens a new one. The string says what happened.
func Go(c Herdr, run *ticket.Run) (string, error) {
	label := strings.ToLower(run.Lead.Label)
	if run.LeadPane != "" {
		if err := c.FocusAgent(run.LeadPane); err != nil {
			return "", err
		}
		return "going to the " + label, nil
	}
	if run.Workspace == "" {
		return "", fmt.Errorf("no Herdr workspace is open on %s, so there is nowhere to open the %s", run.Worktree, label)
	}
	if run.Lead.Kind == "" {
		return "", fmt.Errorf("no %s is open; set kind in config.toml's [lead] (e.g. kind = \"cursor\") to open one", label)
	}

	agents, err := c.Agents()
	if err != nil {
		return "", err
	}
	name := freeName(run, agents)
	_, pane, err := c.CreateTab(run.Workspace, run.Worktree, run.Lead.Tab)
	if err != nil {
		return "", fmt.Errorf("open the %s tab: %w", run.Lead.Tab, err)
	}
	if err := c.StartAgent(name, run.Lead.Kind, pane, run.Lead.Args); err != nil {
		return "", fmt.Errorf("start the %s in tab %s: %w", label, run.Lead.Tab, err)
	}
	file := filepath.Join(run.Dir, run.Lead.Tab+"-resume.md")
	if err := os.WriteFile(file, []byte(Expand(run.Lead.Prompt, run)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write the %s's prompt: %w", label, err)
	}
	// One line pointing at the file: long text typed into a terminal agent is
	// easily cut or split.
	if err := c.PromptAgent(name, "Read "+slash(file)+" in full and follow it. It is your complete instructions."); err != nil {
		return "", fmt.Errorf("send the %s its prompt: %w", label, err)
	}
	_ = c.FocusAgent(pane)
	return fmt.Sprintf("opened a new %s (%s) in tab %s", label, name, run.Lead.Tab), nil
}

// freeName is the ticket key in lower case, or that plus the tab, or a
// numbered one: agent names are unique in Herdr.
func freeName(run *ticket.Run, agents []herdr.Agent) string {
	taken := map[string]bool{}
	for _, a := range agents {
		taken[strings.ToLower(a.Name)] = true
	}
	base := strings.ToLower(run.Key)
	for i, name := 0, base; ; i++ {
		if !taken[name] {
			return name
		}
		name = base + "-" + run.Lead.Tab
		if i > 0 {
			name = fmt.Sprintf("%s-%s-%d", base, run.Lead.Tab, i+1)
		}
	}
}

// Expand fills a prompt's placeholders from run.
func Expand(prompt string, run *ticket.Run) string {
	return strings.NewReplacer(
		"{label}", run.Lead.Label,
		"{key}", run.Key,
		"{team}", run.TeamName,
		"{target}", run.Target,
		"{branch}", run.Branch,
		"{worktree}", slash(run.Worktree),
		"{run}", slash(run.Dir),
		"{state}", slash(run.StatePath()),
		"{workspace}", run.Workspace,
		"{issue}", run.JiraURL,
	).Replace(prompt)
}

func slash(p string) string { return filepath.ToSlash(p) }
