package store

import (
	"testing"
	"time"
)

// A ticket accepted by another process -- `ekanban close` -- while a board is
// open survives that board saving its own copy.
func TestSaveKeepsAnAcceptMadeElsewhere(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	open, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	open.SetPR("/w/other", 3)
	if err := open.Save(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)
	closer, _ := Load()
	closer.SetPR("/w/abc-1", 7)
	closer.Accept("/w/abc-1", Accepted{At: time.Now().UTC(), Ticket: "ABC-1", Archive: "/state/archive/ABC-1"})
	if err := closer.Save(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)
	open.SetPR("/w/other", 4)
	if err := open.Save(); err != nil {
		t.Fatal(err)
	}
	after, _ := Load()
	a := after.Entries["/w/abc-1"].Accepted
	if a == nil || a.Ticket != "ABC-1" || a.Archive != "/state/archive/ABC-1" || after.Entries["/w/other"].PR != 4 {
		t.Fatalf("entries: %+v", after.Entries)
	}
}

// Unaccepting here, after the accept elsewhere, is the newer word.
func TestSaveKeepsAnUnacceptMadeHere(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	open, _ := Load()
	open.SetPR("/w/abc-1", 7)
	_ = open.Save()

	time.Sleep(5 * time.Millisecond)
	closer, _ := Load()
	closer.Accept("/w/abc-1", Accepted{At: time.Now().UTC(), Ticket: "ABC-1"})
	_ = closer.Save()

	time.Sleep(5 * time.Millisecond)
	open.Unaccept("/w/abc-1")
	_ = open.Save()
	after, _ := Load()
	if after.Entries["/w/abc-1"].Accepted != nil {
		t.Fatal("an unaccept made after the accept was undone")
	}
}

// An accept the board already knew of when it was read is not brought back
// once the board forgets the entry.
func TestSaveDoesNotResurrectAForgottenAccept(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	b, _ := Load()
	b.Accept("/w/abc-1", Accepted{At: time.Now().UTC(), Ticket: "ABC-1"})
	_ = b.Save()

	time.Sleep(5 * time.Millisecond)
	open, _ := Load()
	delete(open.Entries, "/w/abc-1")
	_ = open.Save()
	after, _ := Load()
	if _, ok := after.Entries["/w/abc-1"]; ok {
		t.Fatal("a forgotten entry came back")
	}
}
