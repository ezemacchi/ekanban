// Package orchestrator takes the user to the agent running a ticket (the team's orchestrator),
// opening one when none is open: a
// tab in the ticket's workspace, the agent the configuration names, and its
// prompt written to the run folder.
package orchestrator

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
	SendKeys(paneID string, keys ...string) error
}

// Go focuses run's orchestrator, or opens a new one. The string says what happened.
func Go(c Herdr, run *ticket.Run) (string, error) {
	label := strings.ToLower(run.Orchestrator.Label)
	if run.OrchestratorPane != "" {
		if err := c.FocusAgent(run.OrchestratorPane); err != nil {
			return "", err
		}
		return "going to the " + label, nil
	}
	if run.Workspace == "" {
		return "", fmt.Errorf("no Herdr workspace is open on %s, so there is nowhere to open the %s", run.Worktree, label)
	}
	if text, ran, err := viaCommand(run, run.Orchestrator.Command); ran {
		return text, err
	}
	if run.Orchestrator.Kind == "" {
		return "", fmt.Errorf("no %s is open; set kind in config.toml's [orchestrator] (e.g. kind = \"cursor\") to open one", label)
	}

	agents, err := c.Agents()
	if err != nil {
		return "", err
	}
	name := freeName(run, agents)
	_, pane, err := c.CreateTab(run.Workspace, run.Worktree, run.Orchestrator.Tab)
	if err != nil {
		return "", fmt.Errorf("open the %s tab: %w", run.Orchestrator.Tab, err)
	}
	args, resumed := resumeArgs(run)
	err = c.StartAgent(name, run.Orchestrator.Kind, pane, args)
	if err != nil && resumed {
		// The old conversation is gone or cannot be reopened: start a new one.
		resumed = false
		err = c.StartAgent(name, run.Orchestrator.Kind, pane, run.Orchestrator.Args)
	}
	if err != nil {
		return "", fmt.Errorf("start the %s in tab %s: %w", label, run.Orchestrator.Tab, err)
	}
	prompt := run.Orchestrator.Prompt
	if resumed {
		prompt = run.Orchestrator.ResumePrompt
	}
	file := filepath.Join(run.Dir, run.Orchestrator.Tab+"-resume.md")
	if err := os.WriteFile(file, []byte(Expand(prompt, run)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write the %s's prompt: %w", label, err)
	}
	// One line pointing at the file: long text typed into a terminal agent is
	// easily cut or split.
	if err := c.PromptAgent(name, "Read "+slash(file)+" in full and follow it. It is your complete instructions."); err != nil {
		return "", fmt.Errorf("send the %s its prompt: %w", label, err)
	}
	if err := submitted(c, pane); err != nil {
		return "", fmt.Errorf("%w; press enter in its tab", err)
	}
	_ = c.FocusAgent(pane)
	return fmt.Sprintf("opened a new %s (%s) in tab %s", label, name, run.Orchestrator.Tab), nil
}

// resumeArgs are the orchestrator's arguments with --resume <id> added, when
// the run recorded the conversation of an agent of this kind: Cursor and
// Claude both take it. Otherwise the arguments as they are.
func resumeArgs(run *ticket.Run) ([]string, bool) {
	agent, id := ticket.SavedSession(run.Dir)
	if id == "" || agent != run.Orchestrator.Kind || (agent != "cursor" && agent != "claude") {
		return run.Orchestrator.Args, false
	}
	return append(append([]string(nil), run.Orchestrator.Args...), "--resume", id), true
}

// freeName is the ticket key in lower case, or that plus the tab, or a
// numbered one: agent names are unique in Herdr.
func freeName(run *ticket.Run, agents []herdr.Agent) string {
	taken := map[string]bool{}
	for _, a := range agents {
		taken[strings.ToLower(a.Name)] = true
	}
	base := strings.ToLower(run.Key)
	for i, name := 0, herdrName(base, maxAgentName); ; i++ {
		if !taken[name] {
			return name
		}
		name = base + "-" + run.Orchestrator.Tab
		if i > 0 {
			name = fmt.Sprintf("%s-%s-%d", base, run.Orchestrator.Tab, i+1)
		}
		name = herdrName(name, maxAgentName)
	}
}

// maxAgentName is the longest agent name Herdr accepts.
const maxAgentName = 32

// herdrName makes s a name Herdr accepts: lower-case letters, digits, '-' and
// '_', starting with a letter, at most max characters.
func herdrName(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		}
		return '-'
	}, s)
	s = strings.TrimLeft(s, "-_0123456789")
	if s == "" {
		s = "agent"
	}
	if len(s) > max {
		s = s[:max]
	}
	return strings.TrimRight(s, "-_")
}

// freeAgentName is base, or base numbered: agent names are unique in Herdr,
// and at most maxAgentName characters.
func freeAgentName(base string, agents []herdr.Agent) string {
	taken := map[string]bool{}
	for _, a := range agents {
		taken[strings.ToLower(a.Name)] = true
	}
	name := herdrName(base, maxAgentName)
	for i := 2; taken[name]; i++ {
		suffix := fmt.Sprintf("-%d", i)
		name = herdrName(base, maxAgentName-len(suffix)) + suffix
	}
	return name
}

// Expand fills a prompt's placeholders from run.
func Expand(prompt string, run *ticket.Run) string {
	return strings.NewReplacer(
		"{label}", run.Orchestrator.Label,
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
