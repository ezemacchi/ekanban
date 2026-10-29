package closing

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/archive"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/state"
	"github.com/ezemacchi/ekanban/internal/store"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// checkTimeout bounds [close] check: a check that hangs must not hold the
// close open for ever.
const checkTimeout = 3 * time.Minute

// Real is the Env that does it: git, the check program, the archive beside
// the board in stateDir, board.json and Herdr. accepted receives the record
// written, for a board that keeps its own copy of the file.
func Real(client *herdr.Client, stateDir string, hide func(*exec.Cmd), accepted func(key string, a store.Accepted)) Env {
	if hide == nil {
		hide = func(*exec.Cmd) {}
	}
	git := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		hide(cmd)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	return Env{
		Git: func(dir string, args ...string) (string, error) {
			cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
			hide(cmd)
			out, err := cmd.Output()
			return strings.TrimSpace(string(out)), err
		},
		RunCheck: func(dir string, args []string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Dir = dir
			hide(cmd)
			out, err := cmd.CombinedOutput()
			return string(out), err
		},
		Archive: func(t state.Ticket) (string, error) {
			if t.RunDir == "" {
				return "", nil // no run: no notes to keep
			}
			dest := archive.Path(stateDir, t.Key)
			_, err := archive.Snapshot(t.Worktree, t.RunDir, dest)
			return dest, err
		},
		Accept: func(t state.Ticket, path string) error {
			board, err := store.Load()
			if err != nil {
				return err
			}
			key := store.Key(t.Worktree)
			rec := Record(t, path)
			board.Accept(key, rec)
			if err := board.Save(); err != nil {
				return err
			}
			if accepted != nil {
				accepted(key, rec)
			}
			return nil
		},
		CloseWorkspace: func(id string) error {
			if client == nil {
				return fmt.Errorf("not connected to Herdr")
			}
			return client.CloseWorkspace(id)
		},
		RemoveWorktree: func(repo, worktree string) error {
			return Retry(5, time.Second, func() error {
				out, err := git(repo, "worktree", "remove", worktree)
				if err != nil && out != "" {
					return fmt.Errorf("%s", firstLine(out, err.Error()))
				}
				return err
			})
		},
	}
}

// Record is the archive record of a closed ticket.
func Record(t state.Ticket, archivePath string) store.Accepted {
	a := store.Accepted{
		At:       time.Now().UTC(),
		Ticket:   t.Key,
		Title:    t.Title,
		Branch:   t.Branch,
		Worktree: store.Key(t.Worktree),
		Repo:     store.CommonDir(t.Worktree),
		Archive:  archivePath,
	}
	if t.PR != nil {
		a.PR, a.MergedTo, a.Deployed = t.PR.Number, t.PR.MergedTo, t.PR.Deployed
	}
	return a
}

// WorkspacesOn are the live workspaces opened on worktree, sorted.
func WorkspacesOn(live ticket.Live, worktree string) []string {
	var out []string
	for id, checkout := range live.Workspaces {
		if store.SameDir(checkout, worktree) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
