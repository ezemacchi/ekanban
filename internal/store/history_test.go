package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// worktree makes a folder that looks like a linked worktree: it has a .git file.
func worktree(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRecordVanishedWritesWhatTheBoardKnew(t *testing.T) {
	b := loadTemp(t)
	root := t.TempDir()
	gone := filepath.Join(root, "wt-ABC-1") // never created: the folder is gone
	b.Entries[gone] = Entry{Label: "ABC-1 landing", Note: "waiting on review", PR: 42}

	got, err := b.RecordVanished(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != gone || got[0].PR != 42 || got[0].Note != "waiting on review" {
		t.Fatalf("want one event carrying the entry, got %+v", got)
	}
	if !strings.Contains(got[0].Detail, "the folder is gone") {
		t.Fatalf("detail should say the folder is gone: %q", got[0].Detail)
	}

	read, err := ReadHistory()
	if err != nil || len(read) != 1 || read[0].Kind != KindVanished {
		t.Fatalf("history on disk: %+v, %v", read, err)
	}
}

func TestRecordVanishedCatchesAFolderThatLostItsCheckout(t *testing.T) {
	b := loadTemp(t)
	root := t.TempDir()
	half := filepath.Join(root, "wt-ABC-2")
	if err := os.MkdirAll(filepath.Join(half, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	b.Entries[half] = Entry{PR: 7}

	got, err := b.RecordVanished(time.Now())
	if err != nil || len(got) != 1 {
		t.Fatalf("a folder with no .git is a vanished worktree: %+v, %v", got, err)
	}
	if !strings.Contains(got[0].Detail, "node_modules") {
		t.Fatalf("detail should name what was left: %q", got[0].Detail)
	}
}

func TestRecordVanishedLeavesLiveAndAcceptedAlone(t *testing.T) {
	b := loadTemp(t)
	root := t.TempDir()
	live := worktree(t, root, "wt-ABC-3")
	b.Entries[live] = Entry{PR: 1}
	plain := filepath.Join(root, "notes") // a space in a folder that is not a repository
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	b.Entries[plain] = Entry{Note: "reading list"}
	b.Entries[filepath.Join(root, "wt-ABC-4")] = Entry{Accepted: &Accepted{Ticket: "ABC-4"}}

	got, err := b.RecordVanished(time.Now())
	if err != nil || len(got) != 0 {
		t.Fatalf("nothing vanished, got %+v, %v", got, err)
	}
	if _, err := os.Stat(mustHistory(t)); !os.IsNotExist(err) {
		t.Fatalf("no history file should be written when nothing vanished: %v", err)
	}
}

func TestRecordVanishedWritesEachOnce(t *testing.T) {
	b := loadTemp(t)
	b.Entries[filepath.Join(t.TempDir(), "wt-ABC-5")] = Entry{PR: 5}

	for i := 0; i < 3; i++ {
		if _, err := b.RecordVanished(time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	read, _ := ReadHistory()
	if len(read) != 1 {
		t.Fatalf("a watcher polling again must not repeat the record; got %d lines", len(read))
	}
}

func TestRecordVanishedDoesNotChangeTheBoard(t *testing.T) {
	b := loadTemp(t)
	key := filepath.Join(t.TempDir(), "wt-ABC-6")
	b.Entries[key] = Entry{PR: 6}
	if _, err := b.RecordVanished(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Entries[key]; !ok {
		t.Fatal("recording must not drop the entry; Prune does that")
	}
}

func TestHistorySkipsAJunkLine(t *testing.T) {
	loadTemp(t)
	if err := AppendHistory(Event{Kind: KindNote, Key: "k", Detail: "first"}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(mustHistory(t), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("not json\n")
	f.Close()
	if err := AppendHistory(Event{Kind: KindNote, Key: "k", Detail: "second"}); err != nil {
		t.Fatal(err)
	}
	read, err := ReadHistory()
	if err != nil || len(read) != 2 || read[1].Detail != "second" {
		t.Fatalf("a bad line must not hide the rest: %+v, %v", read, err)
	}
}

func mustHistory(t *testing.T) string {
	t.Helper()
	p, err := HistoryPath()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
