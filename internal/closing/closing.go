// Package closing closes a finished ticket: it makes sure nothing would be
// lost, then accepts and archives it, closes its Herdr workspaces and removes
// its worktree. Every check comes first; the first one that fails stops the
// close with its reason and nothing has been touched.
//
// The order is the point. A worktree removed before its commits reached a
// remote loses them; one removed while a workspace is open on it fails, or
// takes an agent's folder from under it; one removed without archiving takes
// the run's notes along.
package closing

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/state"
)

// Plan is one ticket to close.
type Plan struct {
	Ticket state.Ticket
	Repo   string // the repository's main checkout, where git worktree remove runs
	// Workspaces are the Herdr workspaces open on the worktree; Self is the
	// one this process runs in, "" outside Herdr.
	Workspaces []string
	Self       string
	// Check is [close] check, its placeholders not yet filled.
	Check []string
}

// Env is what a close does, passed in so the order can be tested.
type Env struct {
	Git            state.Git
	RunCheck       func(dir string, args []string) (output string, err error)
	Archive        func(t state.Ticket) (path string, err error)
	Accept         func(t state.Ticket, archive string) error
	CloseWorkspace func(id string) error
	RemoveWorktree func(repo, worktree string) error
}

// Step is one step done, for the caller to say.
type Step struct {
	Name, Note string
}

// Refused is a check that failed: nothing was changed.
type Refused struct {
	Check, Why string
}

func (r *Refused) Error() string { return r.Check + ": " + r.Why }

// Checks runs the checks alone, in order, and returns what each found.
func Checks(env Env, p Plan) ([]Step, error) {
	var done []Step
	t := p.Ticket
	refuse := func(check, why string, args ...any) ([]Step, error) {
		return done, &Refused{check, fmt.Sprintf(why, args...)}
	}

	if p.Self != "" && contains(p.Workspaces, p.Self) {
		return refuse("where it runs", "this runs in workspace %s, which is open on the ticket's worktree; closing it would stop the close halfway. Run it from another workspace", p.Self)
	}

	out, err := env.Git(t.Worktree, "status", "--porcelain")
	if err != nil {
		return refuse("uncommitted changes", "git could not read the worktree: %v", err)
	}
	if paths := nonEmptyLines(out); len(paths) > 0 {
		return refuse("uncommitted changes", "%d path(s) changed or not tracked (%s): commit or discard them first", len(paths), strings.Join(first(paths, 5), "; "))
	}
	done = append(done, Step{"uncommitted changes", "none"})

	out, err = env.Git(t.Worktree, "rev-list", "--count", "HEAD", "--not", "--remotes")
	if err != nil {
		return refuse("pushed", "git could not count the commits on no remote: %v", err)
	}
	if n, _ := strconv.Atoi(strings.TrimSpace(out)); n > 0 {
		return refuse("pushed", "%d commit(s) are on no remote: push them first", n)
	}
	done = append(done, Step{"pushed", "every commit is on a remote"})

	switch target := t.Git.Target; {
	case t.PR != nil && t.PR.MergedTo != "":
		done = append(done, Step{"merged", fmt.Sprintf("PR #%d merged to %s", t.PR.Number, t.PR.MergedTo)})
	case target == "":
		return refuse("merged", "no pull request merge seen and no target branch to compare with")
	default:
		if _, err := env.Git(t.Worktree, "merge-base", "--is-ancestor", "HEAD", "origin/"+target); err != nil {
			why := "the branch is not in origin/" + target
			if t.PR != nil {
				why = fmt.Sprintf("PR #%d is not merged and %s", t.PR.Number, why)
			}
			return refuse("merged", "%s", why)
		}
		done = append(done, Step{"merged", "the branch is in origin/" + target})
	}

	if len(p.Check) > 0 {
		args := Fill(p.Check, p)
		out, err := env.RunCheck(t.Worktree, args)
		if err != nil {
			why := strings.TrimSpace(out)
			if why == "" {
				why = err.Error()
			}
			return refuse("close check", "%s said no:\n%s", args[0], why)
		}
		done = append(done, Step{"close check", firstLine(out, args[0]+" passed")})
	}
	return done, nil
}

// Close runs the checks, then accepts and archives the ticket, closes its
// workspaces and removes its worktree, saying each step as it is done.
func Close(env Env, p Plan, say func(Step)) error {
	steps, err := Checks(env, p)
	for _, s := range steps {
		say(s)
	}
	if err != nil {
		return err
	}

	t := p.Ticket
	path, err := env.Archive(t)
	if err != nil {
		return fmt.Errorf("could not archive the run's notes, so nothing was closed: %w", err)
	}
	if err := env.Accept(t, path); err != nil {
		return fmt.Errorf("could not accept %s, so nothing was closed: %w", t.Key, err)
	}
	say(Step{"accepted", "in the Archive, notes kept in " + orNone(path)})

	for _, ws := range p.Workspaces {
		if err := env.CloseWorkspace(ws); err != nil {
			return fmt.Errorf("%s is accepted, but workspace %s did not close (%v), so the worktree stays", t.Key, ws, err)
		}
		say(Step{"workspace closed", ws})
	}

	if err := env.RemoveWorktree(p.Repo, t.Worktree); err != nil {
		return fmt.Errorf("%s is accepted and its workspaces closed, but the worktree was not removed: %w", t.Key, err)
	}
	say(Step{"worktree removed", t.Worktree})
	return nil
}

// Fill is args with the ticket's placeholders filled in.
func Fill(args []string, p Plan) []string {
	t := p.Ticket
	pr := ""
	if t.PR != nil {
		pr = strconv.Itoa(t.PR.Number)
	}
	r := strings.NewReplacer("{key}", t.Key, "{worktree}", t.Worktree, "{branch}", t.Branch,
		"{target}", t.Git.Target, "{pr}", pr, "{repo}", p.Repo)
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = r.Replace(a)
	}
	return out
}

// Retry calls f until it succeeds or tries run out, waiting between: a
// workspace just closed may hold its folder for a moment longer.
func Retry(tries int, wait time.Duration, f func() error) error {
	var err error
	for i := 0; i < tries; i++ {
		if err = f(); err == nil {
			return nil
		}
		if i < tries-1 {
			time.Sleep(wait)
		}
	}
	return err
}

// IsRefused says whether err is a check that failed.
func IsRefused(err error) bool {
	var r *Refused
	return errors.As(err, &r)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func first(list []string, n int) []string {
	if len(list) > n {
		return append(append([]string(nil), list[:n]...), "…")
	}
	return list
}

func firstLine(s, otherwise string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return otherwise
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
