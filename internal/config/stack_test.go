package config

import (
	"path/filepath"
	"slices"
	"testing"
)

// [stack] is off until it names a source, so a board with no such table is
// the board as it was.
func TestStackIsOffByDefault(t *testing.T) {
	write(t, "icons = true\n")
	s := LoadFor("")
	if len(s.Stack.Sources) != 0 || s.Stack.MaxItems != 0 || s.Stack.Collapse != nil || s.Stack.Column != "" {
		t.Fatalf("a stack setting appeared from nothing: %+v", s.Stack)
	}
}

func TestStackReadsItsTableAndEachSourcesOwn(t *testing.T) {
	write(t, "[stack]\nsources = [\"failed-tests\"]\nmax_items = 3\ncollapse = false\ncolumn = \"todo\"\n"+
		"[stack.source.failed-tests]\nfrom = \"linked\"\n")
	s := LoadFor("")
	if !slices.Equal(s.Stack.Sources, []string{"failed-tests"}) || s.Stack.MaxItems != 3 || s.Stack.Collapse == nil || *s.Stack.Collapse || s.Stack.Column != "todo" {
		t.Fatalf("stack = %+v", s.Stack)
	}
	if got := s.Stack.Source["failed-tests"]["from"]; got != "linked" {
		t.Fatalf("source table: %q", got)
	}
}

// A repository decides how its team records a failed test, so its file may
// set [stack]; a list replaces the whole list.
func TestStackFromTheRepositoryReplacesTheList(t *testing.T) {
	write(t, "[stack]\nsources = [\"failed-tests\", \"other\"]\n")
	main, wt := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), "[stack]\nsources = [\"failed-tests\"]\n[stack.source.failed-tests]\nfrom = \"comment\"\n")
	s := LoadFor(wt)
	if !slices.Equal(s.Stack.Sources, []string{"failed-tests"}) {
		t.Fatalf("sources = %v", s.Stack.Sources)
	}
	if s.Stack.Source["failed-tests"]["from"] != "comment" {
		t.Fatalf("source table = %v", s.Stack.Source)
	}
	if len(s.Problems) != 0 {
		t.Fatalf("a repository may set [stack]: %v", s.Problems)
	}
}
