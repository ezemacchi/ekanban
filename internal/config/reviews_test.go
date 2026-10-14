package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// kind chooses where a token goes and command a program to start, so a
// repository file cannot set them; the column is the team's to choose.
func TestRepoFileCanMoveReviewCardsButNotChooseTheirSource(t *testing.T) {
	write(t, `[reviews]
kind = "command"
command = ["gh", "pr", "list"]
`)
	main, wt := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), `[reviews]
kind = "bitbucket"
command = ["evil.exe"]
column = "ready_qa"
`)
	s := LoadFor(wt)
	if s.Reviews.Kind != "command" || strings.Join(s.Reviews.Command, " ") != "gh pr list" {
		t.Fatalf("the repository file chose the source: %+v", s.Reviews)
	}
	if s.Reviews.CardColumn("on_review") != "ready_qa" {
		t.Fatalf("the repository file's column was not taken: %+v", s.Reviews)
	}
	var said bool
	for _, p := range s.Problems {
		said = said || strings.Contains(p, "[reviews] kind and command")
	}
	if !said {
		t.Fatalf("no word about what was ignored: %q", s.Problems)
	}
}

func TestReviewsAreOffByDefault(t *testing.T) {
	write(t, "")
	s := Load()
	if s.Reviews.Kind != "" || s.Reviews.CardColumn("on_review") != "on_review" {
		t.Fatalf("defaults: %+v", s.Reviews)
	}
}
