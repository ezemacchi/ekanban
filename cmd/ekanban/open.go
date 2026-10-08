package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ezemacchi/ekanban/internal/config"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// boardTabLabel is the label of a tab holding a board, global or per ticket.
const boardTabLabel = "board"

// ticketPaneLabel is the title of the ticket board's pane, the one the
// manifest gives the `ticket` entrypoint; the space kanban's is "Board".
const ticketPaneLabel = "Ticket"

// holdsPane reports whether tab has a pane titled label.
func holdsPane(panes []herdr.Pane, tab, label string) bool {
	for _, p := range panes {
		if p.TabID == tab && strings.EqualFold(p.Label, label) {
			return true
		}
	}
	return false
}

// opening is where `open` puts the board.
type opening int

const (
	toBoardTab opening = iota // the workspace has a board tab: go there
	inNewTab                  // a new board tab, first in the workspace
	overPane                  // over the current pane, until q or a jump
)

// chooseOpening decides where the board opens from the workspace's tabs and
// the pane the key was pressed in. An agent's pane is worth keeping in view,
// so the board gets a tab of its own beside it; anywhere else it covers the
// pane for a moment and gives it back.
//
// Both kinds of board are labelled "board", so in a ticket's workspace the
// label alone cannot say which one a tab holds: the space kanban there is the
// wrong answer. panes say what each tab holds, by the pane's title.
func chooseOpening(tabs []herdr.Tab, panes []herdr.Pane, current herdr.Pane, ticketWorkspace bool) (opening, string) {
	for _, t := range tabs {
		if !strings.EqualFold(t.Label, boardTabLabel) {
			continue
		}
		if ticketWorkspace && !holdsPane(panes, t.ID, ticketPaneLabel) {
			continue
		}
		return toBoardTab, t.ID
	}
	if current.Agent != nil && *current.Agent != "" {
		return inNewTab, ""
	}
	return overPane, ""
}

// openBoard is the `open` action, which Herdr runs for a keybinding such as
// prefix+shift+k: it shows the kanban for wherever you are.
func openBoard(client *herdr.Client) error {
	err := showBoard(client)
	if err != nil {
		_ = client.Notify("eKanban", err.Error(), "")
	}
	return err
}

func showBoard(client *herdr.Client) error {
	current, err := currentPane(client)
	if err != nil {
		return err
	}
	tabs, err := client.Tabs(current.WorkspaceID)
	if err != nil {
		return err
	}
	plugin := envOr("HERDR_PLUGIN_ID", "ekanban")

	panes, err := client.Panes()
	if err != nil {
		return err
	}
	// In a ticket's workspace the board that belongs there is the ticket's,
	// not the space kanban, and it reads the worktree's root.
	dir, isTicket := ticketWorkspace(client, current)
	where, tab := chooseOpening(tabs, panes, current, isTicket)
	tabEntry, overEntry, cwd := "tab", "board", current.Cwd
	if isTicket {
		tabEntry, overEntry, cwd = "ticket", "ticket", dir
	}
	switch where {
	case toBoardTab:
		return client.FocusTab(tab)
	case inNewTab:
		params := map[string]any{"placement": "tab", "workspace_id": current.WorkspaceID, "focus": true}
		if cwd != "" {
			params["cwd"] = cwd
		}
		if err := client.OpenPluginPane(plugin, tabEntry, params); err != nil {
			return err
		}
		after, err := client.Tabs(current.WorkspaceID)
		if err != nil {
			return err
		}
		added := newTabs(tabs, after)
		if len(added) != 1 {
			return nil // opened; a tab we cannot tell apart keeps its own label
		}
		if err := client.RenameTab(added[0], boardTabLabel); err != nil {
			return err
		}
		if err := client.MoveTab(added[0], 0); err != nil {
			return err
		}
		return client.FocusTab(added[0])
	default:
		params := map[string]any{"placement": "overlay", "target_pane_id": current.PaneID, "focus": true}
		if cwd != "" {
			params["cwd"] = cwd
		}
		return client.OpenPluginPane(plugin, overEntry, params)
	}
}

// ticketWorkspace reports whether the pane's workspace is a ticket's: its
// worktree was made for a run. dir is the worktree's root, which is where the
// ticket board reads the run; a pane's own folder may be a subfolder.
func ticketWorkspace(client *herdr.Client, current herdr.Pane) (dir string, ok bool) {
	dir = current.Cwd
	if all, err := client.Workspaces(); err == nil {
		for _, ws := range all {
			if ws.ID == current.WorkspaceID && ws.Checkout() != "" {
				dir = ws.Checkout()
			}
		}
	}
	if dir == "" {
		return "", false
	}
	return dir, ticket.Owns(dir, config.LoadFor(dir).Layout)
}

// currentPane is the pane the action was invoked from: Herdr names it for a
// plugin action, and for a custom command as the active pane. Without either,
// the focused pane.
func currentPane(client *herdr.Client) (herdr.Pane, error) {
	id := envOr("HERDR_PANE_ID", os.Getenv("HERDR_ACTIVE_PANE_ID"))
	panes, err := client.Panes()
	if err != nil {
		return herdr.Pane{}, err
	}
	var focused *herdr.Pane
	for i, p := range panes {
		if id != "" && p.PaneID == id {
			return p, nil
		}
		if p.Focused && focused == nil {
			focused = &panes[i]
		}
	}
	if focused == nil {
		return herdr.Pane{}, fmt.Errorf("no focused pane to open the board from")
	}
	return *focused, nil
}

// newTabs are the ids in after that were not in before.
func newTabs(before, after []herdr.Tab) []string {
	had := map[string]bool{}
	for _, t := range before {
		had[t.ID] = true
	}
	var out []string
	for _, t := range after {
		if !had[t.ID] {
			out = append(out, t.ID)
		}
	}
	return out
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
