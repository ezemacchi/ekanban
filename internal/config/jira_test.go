package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// The project file says which tickets and which columns; only config.toml
// says which site, which account's variables and which program a handoff runs.
func TestRepoFileCannotChooseTheJiraAccountOrTheHandoffProgram(t *testing.T) {
	write(t, `[jira]
url = "https://mine.example"
token_env = "MY_TOKEN"

[jira.handoff]
command = ["mine", "{key}"]
teams = ["a", "b"]
`)
	main, wt := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), `[jira]
url = "https://evil.example"
token_env = "STEAL_ME"
jql = "project = X ORDER BY rank"

[jira.columns]
"In Quality Review" = "ready_qa"

[jira.handoff]
command = ["evil", "{key}"]
teams = ["x"]
`)
	s := LoadFor(wt)
	if s.Jira.URL != "https://mine.example" || s.Jira.TokenEnv != "MY_TOKEN" || strings.Join(s.Jira.Handoff.Command, " ") != "mine {key}" {
		t.Fatalf("the repository file chose the account or the program: %+v", s.Jira)
	}
	if s.Jira.JQL != "project = X ORDER BY rank" || s.Jira.Columns["In Quality Review"] != "ready_qa" || strings.Join(s.Jira.Handoff.Teams, ",") != "x" {
		t.Fatalf("the project's own settings were lost: %+v", s.Jira)
	}
	var said bool
	for _, p := range s.Problems {
		said = said || strings.Contains(p, "[jira] url, email_env, token_env and handoff.command")
	}
	if !said {
		t.Fatalf("no word about what was ignored: %q", s.Problems)
	}
}

func TestJiraDefaultsAreFilledAndOffWithoutAURL(t *testing.T) {
	write(t, "")
	s := Load()
	if s.Jira.URL != "" || s.Jira.TokenEnv != "JIRA_API_TOKEN" || s.Jira.EmailEnv != "JIRA_EMAIL" || s.Jira.JQL == "" || s.Jira.TestType != "Test" {
		t.Fatalf("%+v", s.Jira)
	}
}

// A list of the same length as the personal one is decoded over it in place:
// the repository file must still not choose the program's arguments.
func TestRepoFileCannotRewriteOrchestratorArgsOfTheSameLength(t *testing.T) {
	write(t, "[orchestrator]\nkind = \"cursor\"\nargs = [\"--model\", \"mine\"]\n")
	main, wt := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), "[orchestrator]\nargs = [\"--model\", \"theirs\"]\n")
	s := LoadFor(wt)
	if got := strings.Join(s.Orchestrator.Args, " "); got != "--model mine" {
		t.Fatalf("the repository file changed the arguments: %q", got)
	}
}
