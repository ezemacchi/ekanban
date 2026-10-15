package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// The board forgets a ticket when its worktree goes: Prune drops the entry,
// and only an accepted ticket keeps a record. A worktree closed without being
// accepted therefore left nothing to say it had existed, who it belonged to or
// what was in it. The history is that record: one JSON object per line, only
// ever appended to, in a file beside the board's. Nothing here edits the
// board, so the watcher can write it while a board is open.

// Event kinds.
const (
	// KindVanished is a board entry whose worktree is gone, or no longer holds
	// a checkout, and that was never accepted.
	KindVanished = "vanished"
	// KindNote is a line added by hand to explain something that happened.
	KindNote = "note"
)

// Event is one line of the history.
type Event struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	// Key is the worktree's directory, as the board keys it.
	Key string `json:"key"`
	// What the board knew of the ticket when it vanished.
	Label  string     `json:"label,omitempty"`
	Status string     `json:"status,omitempty"`
	Note   string     `json:"note,omitempty"`
	PR     int        `json:"pr,omitempty"`
	Seen   *time.Time `json:"last_updated,omitempty"`
	// Detail says what was found on disk, or, for a note, what the user wrote.
	Detail string `json:"detail,omitempty"`
}

// HistoryPath is the history file, beside the board file.
func HistoryPath() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "history.jsonl"), nil
}

// AppendHistory adds events to the end of the history, creating it if needed.
// A single write per event keeps lines whole when two writers meet.
func AppendHistory(events ...Event) error {
	if len(events) == 0 {
		return nil
	}
	path, err := HistoryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, e := range events {
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// ReadHistory returns the history, oldest first. A missing file is an empty
// history; a line that is not an event is skipped rather than hiding the rest.
func ReadHistory() ([]Event, error) {
	path, err := HistoryPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Kind != "" {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// Checkout says whether dir still holds a git checkout: a worktree has a .git
// file, a clone a .git directory. A folder that git removed only part of, or
// that something else kept alive, has neither.
func Checkout(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// gone says whether an entry's worktree is no longer there: the folder does not
// exist, or it exists without a checkout and the entry has had a pull request,
// which only a git worktree has. A plain folder the board lists as a space has
// no .git and no pull request, and is not a worktree that vanished.
func gone(key string, e Entry) bool {
	if _, err := os.Stat(key); errors.Is(err, os.ErrNotExist) {
		return true
	}
	return e.PR != 0 && !Checkout(key)
}

// leftovers names what is still inside dir after its checkout went, so the
// record says whether the folder vanished or was only half removed.
func leftovers(dir string) string {
	names, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "the folder is gone"
	case err != nil:
		return "the folder cannot be read: " + err.Error()
	case len(names) == 0:
		return "the folder is empty"
	}
	var list []string
	for _, n := range names {
		list = append(list, n.Name())
	}
	sort.Strings(list)
	if len(list) > 8 {
		list = append(list[:8], "...")
	}
	return fmt.Sprintf("the folder holds no checkout, only: %v", list)
}

// RecordVanished writes one KindVanished event for every entry that was never
// accepted and whose worktree is gone, and returns them. An entry already
// recorded as vanished, with nothing since, is not written again. It reads the
// board and never changes it.
func (b *Board) RecordVanished(now time.Time) ([]Event, error) {
	past, err := ReadHistory()
	if err != nil {
		return nil, err
	}
	last := map[string]string{}
	for _, e := range past {
		last[e.Key] = e.Kind
	}

	var missing []string
	for key, e := range b.Entries {
		if e.Accepted != nil || last[key] == KindVanished || !gone(key, e) {
			continue
		}
		missing = append(missing, key)
	}
	sort.Strings(missing)

	events := make([]Event, 0, len(missing))
	for _, key := range missing {
		e := b.Entries[key]
		ev := Event{
			At: now.UTC(), Kind: KindVanished, Key: key,
			Label: e.Label, Status: e.Status, Note: e.Note, PR: e.PR,
			Detail: "not accepted before it went; " + leftovers(key),
		}
		if !e.UpdatedAt.IsZero() {
			seen := e.UpdatedAt.UTC()
			ev.Seen = &seen
		}
		events = append(events, ev)
	}
	return events, AppendHistory(events...)
}
