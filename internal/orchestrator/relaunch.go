package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Relauncher is what Relaunch needs of Herdr: Go's calls, the tabs and panes
// of the workspace (to find the orchestrator's tab even when no agent is in
// it), and keys, which is how a terminal agent is told to quit.
type Relauncher interface {
	Herdr
	Tabs(workspaceID string) ([]herdr.Tab, error)
	Panes() ([]herdr.Pane, error)
	SendKeys(paneID string, keys ...string) error
}

// How long Relaunch waits. Variables so a test does not.
var (
	keyGap      = 800 * time.Millisecond // between the two ctrl+c a terminal agent needs to quit
	stopPoll    = 700 * time.Millisecond
	stopFor     = 8 * time.Second
	submitFor   = 8 * time.Second // per try: how long a sent prompt may take to start the agent working
	submitTries = 5
)

// Relaunch starts a new orchestrator for run in the tab the old one used,
// whatever state that one is in: running, idle, hung at an empty prompt, or
// gone and leaving an empty shell. Go cannot do this: with an orchestrator
// open it only goes there, so a dead or useless one could not be replaced.
//
// The old agent, if there is one, is quit with two ctrl+c (the way a Cursor or
// Claude terminal agent exits) and its tab is kept, never closed and never
// killed: an agent that does not quit is reported, and the person decides.
// The new agent is a new conversation, not a resumed one; it is given the
// same prompt as Go's new orchestrator, which reads the run's files to see
// where the run stopped.
func Relaunch(c Relauncher, run *ticket.Run) (string, error) {
	label := strings.ToLower(run.Orchestrator.Label)
	if run.Workspace == "" {
		return "", fmt.Errorf("no Herdr workspace is open on %s, so there is nowhere to relaunch the %s", run.Worktree, label)
	}
	if text, ran, err := viaCommand(run, run.Orchestrator.RelaunchCommand); ran {
		return text, err
	}
	if run.Orchestrator.Kind == "" {
		return "", fmt.Errorf("set kind in config.toml's [orchestrator] (e.g. kind = \"cursor\") to relaunch the %s", label)
	}

	pane, err := relaunchPane(c, run, label)
	if err != nil {
		return "", err
	}

	// The prompt file first: nothing is started if it cannot be written.
	file := filepath.Join(run.Dir, run.Orchestrator.Tab+"-resume.md")
	if err := os.WriteFile(file, []byte(Expand(run.Orchestrator.Prompt, run)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write the %s's prompt: %w", label, err)
	}

	agents, err := c.Agents()
	if err != nil {
		return "", err
	}
	name := freeName(run, agents)
	if err := c.StartAgent(name, run.Orchestrator.Kind, pane, run.Orchestrator.Args); err != nil {
		return "", fmt.Errorf("start the %s in %s: %w", label, pane, err)
	}
	if err := c.PromptAgent(name, "Read "+slash(file)+" in full and follow it. It is your complete instructions."); err != nil {
		return "", fmt.Errorf("send the %s its prompt: %w", label, err)
	}
	if err := submitted(c, pane); err != nil {
		return "", fmt.Errorf("%w; press enter in its tab", err)
	}
	_ = c.FocusAgent(pane)
	return fmt.Sprintf("relaunched the %s (%s) in %s", label, name, pane), nil
}

// relaunchPane is the pane the new agent starts in, with whatever agent was
// in it quit: the orchestrator's own pane, else the pane of the tab named
// for the orchestrator, else a new tab.
func relaunchPane(c Relauncher, run *ticket.Run, label string) (string, error) {
	pane := run.OrchestratorPane
	if pane == "" {
		tabs, err := c.Tabs(run.Workspace)
		if err != nil {
			return "", err
		}
		for _, t := range tabs {
			if t.Label == run.Orchestrator.Tab {
				if pane, err = firstPane(c, t.ID); err != nil {
					return "", err
				}
				break
			}
		}
	}
	if pane == "" {
		_, p, err := c.CreateTab(run.Workspace, run.Worktree, run.Orchestrator.Tab)
		if err != nil {
			return "", fmt.Errorf("open the %s tab: %w", run.Orchestrator.Tab, err)
		}
		return p, nil
	}
	live, err := isAgent(c, pane)
	if err != nil {
		return "", err
	}
	if live {
		if err := quit(c, pane); err != nil {
			return "", fmt.Errorf("the %s did not exit after two ctrl+c: press ctrl+c in its tab, or close the tab, then relaunch (nothing was killed)", label)
		}
	}
	return pane, nil
}

// firstPane is the first pane of a tab, "" when it has none.
func firstPane(c Relauncher, tab string) (string, error) {
	panes, err := c.Panes()
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if p.TabID == tab {
			return p.PaneID, nil
		}
	}
	return "", nil
}

// isAgent is whether Herdr sees an agent in pane.
func isAgent(c Relauncher, pane string) (bool, error) {
	agents, err := c.Agents()
	if err != nil {
		return false, err
	}
	for _, a := range agents {
		if a.PaneID == pane && a.Agent != nil {
			return true, nil
		}
	}
	return false, nil
}

// quit presses ctrl+c twice, then waits for Herdr to stop seeing an agent.
func quit(c Relauncher, pane string) error {
	for i := 0; i < 2; i++ {
		if err := c.SendKeys(pane, "ctrl+c"); err != nil {
			return err
		}
		time.Sleep(keyGap)
	}
	for end := time.Now().Add(stopFor); time.Now().Before(end); time.Sleep(stopPoll) {
		if live, err := isAgent(c, pane); err == nil && !live {
			return nil
		}
	}
	return fmt.Errorf("still running")
}

// submitted waits for the agent in pane to start working on the prompt just
// sent, pressing enter when it does not. A terminal agent is deaf to enter
// for a few seconds after it starts, while it draws its banner, though Herdr
// already calls it idle and ready: the text lands in the input box and its
// enter is lost. Seen on a Cursor agent twice in a row; a later enter works.
func submitted(c Herdr, pane string) error {
	for try := 1; try <= submitTries; try++ {
		for end := time.Now().Add(submitFor); time.Now().Before(end); time.Sleep(stopPoll) {
			agents, err := c.Agents()
			if err != nil {
				return err
			}
			for _, a := range agents {
				if a.PaneID == pane && a.AgentStatus != "idle" {
					return nil
				}
			}
		}
		if try < submitTries {
			if err := c.SendKeys(pane, "enter"); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("the prompt was typed but the agent did not start on it")
}
