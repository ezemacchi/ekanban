package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/orchestrator"
)

// [questions]: the messages default to what the skills teach the
// orchestrator, and a repository may word them for its team, but the agents
// Discuss starts come from config.toml only.
func TestQuestionsComeFromConfigAndTheRepository(t *testing.T) {
	write(t, "[questions]\nsmall = { kind = \"cursor\", args = [\"--model\", \"light\"] }\n")
	main, wt := fakeRepo(t)
	s := LoadFor(wt)
	if s.Questions.Reply != orchestrator.DefaultQuestions.Reply || s.Questions.Tab != "question" {
		t.Fatalf("defaults not filled: %+v", s.Questions)
	}
	if s.Questions.Small.Kind != "cursor" || !slices.Equal(s.Questions.Small.Args, []string{"--model", "light"}) {
		t.Fatalf("small from config.toml: %+v", s.Questions.Small)
	}

	put(t, filepath.Join(main, RepoFileName), "[questions]\nreply = \"{to} answered {brief}: \"\nsmall = { kind = \"claude\", args = [\"--model\", \"other\"] }\nlarge = { kind = \"claude\" }\n")
	s = LoadFor(wt)
	if s.Questions.Reply != "{to} answered {brief}: " {
		t.Fatalf("the repository's reply was not used: %q", s.Questions.Reply)
	}
	if s.Questions.Small.Kind != "cursor" || s.Questions.Small.Args[1] != "light" || s.Questions.Large.Kind != "" {
		t.Fatalf("the repository chose the agents: %+v %+v", s.Questions.Small, s.Questions.Large)
	}
	if !slices.ContainsFunc(s.Problems, func(p string) bool {
		return strings.Contains(p, "[questions] small and large are only read from config.toml")
	}) {
		t.Fatalf("no word about what was ignored: %q", s.Problems)
	}
}
