package links

import "testing"

func TestTemplates(t *testing.T) {
	Configure("", "")
	if Issue("ABC-1") != "" || PullRequest(5) != "" {
		t.Fatal("no template must mean no link")
	}
	Configure("https://jira.example.com/browse/{key}", "https://git.example.com/pr/{pr}")
	defer Configure("", "")
	if got := Issue("ABC-1"); got != "https://jira.example.com/browse/ABC-1" {
		t.Fatalf("issue link %q", got)
	}
	if got := PullRequest(5); got != "https://git.example.com/pr/5" {
		t.Fatalf("pull request link %q", got)
	}
	if PullRequest(0) != "" || Issue("") != "" {
		t.Fatal("an empty key or number must not make a link")
	}
}
