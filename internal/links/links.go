// Package links builds the URLs the boards open, from templates in
// config.toml. Nothing about a particular Jira, Bitbucket or Jenkins lives in
// the code: with no template set, the link is simply not offered.
package links

import (
	"strconv"
	"strings"
)

var issue, pullRequest string

// Configure sets the templates. issueTemplate holds {key} (a Jira key);
// prTemplate holds {pr} (a pull request number).
func Configure(issueTemplate, prTemplate string) {
	issue, pullRequest = strings.TrimSpace(issueTemplate), strings.TrimSpace(prTemplate)
}

// Issue is the tracker URL of a ticket key, or "".
func Issue(key string) string {
	if issue == "" || key == "" {
		return ""
	}
	return strings.ReplaceAll(issue, "{key}", key)
}

// PullRequest is the URL of a pull request number, or "".
func PullRequest(n int) string {
	if pullRequest == "" || n <= 0 {
		return ""
	}
	return strings.ReplaceAll(pullRequest, "{pr}", strconv.Itoa(n))
}
