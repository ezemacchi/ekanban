package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ezemacchi/ekanban/internal/alert"
	"github.com/ezemacchi/ekanban/internal/archive"
	"github.com/ezemacchi/ekanban/internal/state"
)

// Besides pull requests on GitHub, the watcher tells you what a ticket's run
// did while you were elsewhere: its orchestrator finished, or stopped at a
// prompt; it left a question for you; its pull request merged, whoever merged
// it and wherever it is hosted; or its branch is already in its target. These
// come from the same pipeline the repository board reads, so they need no
// GitHub at all.

// Tickets reads the ticket worktrees among dirs, the way `ekanban state`
// does. refresh asks it to fetch the targets and reread the build server as
// well, which is slower; otherwise it may answer from what it read last.
type Tickets func(ctx context.Context, dirs []string, refresh bool) []Row

// Row is one ticket worktree as read.
type Row struct {
	Dir    string // the board's key for the worktree
	Ticket state.Ticket
}

// seen is what the watcher last knew of a ticket, kept between polls and
// across restarts so that nothing is announced twice.
type seen struct {
	// V is seenVersion when the record keeps the orchestrator's pane and
	// finished turn; a record without them cannot be compared by turn.
	V            int      `json:"v,omitempty"`
	Orchestrator string   `json:"orchestrator,omitempty"`
	Pane         string   `json:"pane,omitempty"`
	Completion   uint64   `json:"completion,omitempty"`
	ForYou       []string `json:"for_you,omitempty"`
	PR           int      `json:"pr,omitempty"`
	MergedTo     string   `json:"merged_to,omitempty"`
	Ahead        int      `json:"ahead,omitempty"`
	Compared     string   `json:"compared,omitempty"`
}

const seenVersion = 1

func seenOf(t state.Ticket) seen {
	s := seen{V: seenVersion, Ahead: t.Git.Ahead, Compared: t.Git.Compared}
	if t.Orchestrator != nil {
		s.Orchestrator, s.Pane, s.Completion = t.Orchestrator.Status, t.Orchestrator.Pane, t.Orchestrator.Completion
	}
	for _, q := range t.Questions.ForYou {
		s.ForYou = append(s.ForYou, q.Text)
	}
	if t.PR != nil {
		s.PR, s.MergedTo = t.PR.Number, t.PR.MergedTo
	}
	return s
}

// ticketNotices is what changed between before and now worth telling the
// user. A ticket first seen is the starting point, not news.
func ticketNotices(t state.Ticket, before *seen, now seen) []alert.Alert {
	if before == nil {
		return nil
	}
	var out []alert.Alert
	say := func(k alert.Kind, format string, args ...any) {
		out = append(out, alert.Alert{Kind: k, Text: t.Key + " · " + fmt.Sprintf(format, args...)})
	}

	if finished(*before, now) {
		say(alert.OrchestratorDone, "orchestrator finished")
	}
	if now.Orchestrator == "blocked" && before.Orchestrator != "blocked" {
		say(alert.OrchestratorBlocked, "orchestrator waiting at a prompt")
	}
	for _, q := range now.ForYou {
		if !slices.Contains(before.ForYou, q) {
			say(alert.QuestionForYou, "%s", q)
		}
	}
	if now.MergedTo != "" && before.MergedTo == "" {
		say(alert.Merged, "#%d merged to %s", now.PR, now.MergedTo)
	}
	// The branch's own commits are now all in its target, with no merge of
	// its pull request seen: a squash, a rebase merge, or a merge of another
	// branch that carried them. Only a branch that had commits of its own
	// counts, or every new worktree would be "already merged".
	if now.MergedTo == "" && now.Compared != "" && now.Compared == before.Compared && before.Ahead > 0 && now.Ahead == 0 {
		say(alert.Contained, "branch is already in %s", now.Compared)
	}
	return out
}

// finished is whether the orchestrator finished a turn since before. Herdr
// numbers each finished turn whether or not anyone has looked at it, so a turn
// you glanced at before the poll, or two turns between polls, still count.
// Herdr's "done" (finished and not yet seen) stands in when there is no number
// to compare: a server older than it, another agent in the pane, or a record
// written before the watcher kept one.
func finished(before, now seen) bool {
	if now.Completion != 0 && before.V >= seenVersion && now.Pane == before.Pane {
		return now.Completion != before.Completion
	}
	return now.Orchestrator == "done" && before.Orchestrator != "done"
}

// keepNotes copies the ticket's run folder beside the board when it changed,
// so removing the worktree does not lose its notes. A ticket with no run has
// nothing to keep; a copy that fails is tried again next poll.
func keepNotes(stateDir string, t state.Ticket) {
	if t.RunDir == "" {
		return
	}
	_, _ = archive.Snapshot(t.Worktree, t.RunDir, archive.Path(stateDir, t.Key))
}

// seenStore is the file the watcher keeps what it last knew in.
type seenStore struct {
	Tickets map[string]seen `json:"tickets"`
	path    string
}

func loadSeen(dir string) *seenStore {
	s := &seenStore{Tickets: map[string]seen{}, path: filepath.Join(dir, "tickets-seen.json")}
	if data, err := os.ReadFile(s.path); err == nil {
		var loaded seenStore
		if json.Unmarshal(data, &loaded) == nil && loaded.Tickets != nil {
			s.Tickets = loaded.Tickets
		}
	}
	return s
}

func (s *seenStore) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// pollTickets reads the tickets, records what changed as alerts and returns
// them. Tickets no longer read are forgotten, so one that comes back starts
// afresh rather than announcing everything that happened meanwhile.
// mergedOnGitHub are the spaces GitHub has just said merged, not said twice.
func pollTickets(ctx context.Context, stateDir string, dirs []string, read Tickets, refresh bool, mergedOnGitHub map[string]bool) []alert.Alert {
	if read == nil || len(dirs) == 0 {
		return nil
	}
	rows := read(ctx, dirs, refresh)
	known := loadSeen(stateDir)
	next := map[string]seen{}
	fresh := alert.Load(stateDir)
	var raised []alert.Alert
	for _, r := range rows {
		keepNotes(stateDir, r.Ticket)
		now := seenOf(r.Ticket)
		next[r.Dir] = now
		var before *seen
		if b, ok := known.Tickets[r.Dir]; ok {
			before = &b
		}
		for _, a := range ticketNotices(r.Ticket, before, now) {
			if a.Kind == alert.Merged && mergedOnGitHub[r.Dir] {
				continue
			}
			a.At = time.Now().UTC()
			fresh.Add(r.Dir, a)
			raised = append(raised, a)
		}
	}
	known.Tickets = next
	_ = known.save()
	if len(raised) > 0 {
		_ = fresh.Save()
	}
	return raised
}
