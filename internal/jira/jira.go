// Package jira reads a Jira Cloud site over its REST API: the tickets the
// global board shows, and one ticket's linked tests and comments. It never
// writes. The site, the account and the token's variable come from
// config.toml's [jira]; nothing about a particular site lives here.
package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Config is the [jira] table.
type Config struct {
	// URL is the site, https://your-site.atlassian.net. Empty turns Jira off.
	URL string `toml:"url"`
	// EmailEnv and TokenEnv name the environment variables holding the
	// account's e-mail and API token. The token itself is never in a file.
	EmailEnv string `toml:"email_env"`
	TokenEnv string `toml:"token_env"`
	// JQL picks the tickets shown without a worktree. The board adds the
	// tickets its worktrees name to it.
	JQL string `toml:"jql"`
	// TestType is the issue type of linked test cases.
	TestType string `toml:"test_type"`
	// Columns place a ticket that has no worktree, by status name or status
	// category (new, indeterminate, done): "In Quality Review" = "ready_qa".
	// An empty column id leaves the ticket off the board.
	Columns map[string]string `toml:"columns"`
	// Handoff starts work on a ticket from the board.
	Handoff Handoff `toml:"handoff"`
}

// Handoff is the [jira.handoff] table.
type Handoff struct {
	// Command is the program and its arguments, run without a shell.
	// Placeholders: {key} {summary} {target} {team} {type} {repo}.
	Command []string `toml:"command"`
	// Teams are the choices offered for {team}.
	Teams []string `toml:"teams"`
	// FixTypes are the issue types {type} calls "fix"; any other is "feat".
	FixTypes []string `toml:"fix_types"`
}

// Defaults for the fields left out.
const (
	DefaultEmailEnv = "JIRA_EMAIL"
	DefaultTokenEnv = "JIRA_API_TOKEN"
	DefaultJQL      = "assignee = currentUser() AND statusCategory != Done ORDER BY priority DESC, updated DESC"
	DefaultTestType = "Test"
)

// DefaultColumns places a ticket by its status category.
var DefaultColumns = map[string]string{"new": "todo", "indeterminate": "in_progress", "done": ""}

// DefaultFixTypes are the issue types handed off as fixes.
var DefaultFixTypes = []string{"Bug", "Defect", "Defect Candidate"}

// WithDefaults fills the fields left out.
func (c Config) WithDefaults() Config {
	if c.EmailEnv == "" {
		c.EmailEnv = DefaultEmailEnv
	}
	if c.TokenEnv == "" {
		c.TokenEnv = DefaultTokenEnv
	}
	if strings.TrimSpace(c.JQL) == "" {
		c.JQL = DefaultJQL
	}
	if c.TestType == "" {
		c.TestType = DefaultTestType
	}
	if c.Handoff.FixTypes == nil {
		c.Handoff.FixTypes = DefaultFixTypes
	}
	return c
}

// Column is where a ticket without a worktree goes, and whether it shows.
func (c Config) Column(i Issue) (string, bool) {
	for _, k := range []string{i.Status, i.Category} {
		for name, col := range c.Columns {
			if strings.EqualFold(name, k) {
				return col, col != ""
			}
		}
	}
	col := DefaultColumns[i.Category]
	return col, col != ""
}

// Type is "fix" or "feat" for {type}.
func (h Handoff) Type(issueType string) string {
	for _, t := range h.FixTypes {
		if strings.EqualFold(t, issueType) {
			return "fix"
		}
	}
	return "feat"
}

// Args is the command with the placeholders filled. Each value stays one
// argument, whatever it contains, since no shell reads it.
func (h Handoff) Args(values map[string]string) []string {
	out := make([]string, len(h.Command))
	for i, a := range h.Command {
		for k, v := range values {
			a = strings.ReplaceAll(a, "{"+k+"}", v)
		}
		out[i] = a
	}
	return out
}

// Issue is a ticket as the board lists it.
type Issue struct {
	Key      string
	Summary  string
	Status   string // "In Implementation"
	Category string // new, indeterminate or done
	Type     string // "Story"
	Priority string
}

// Link is a linked ticket.
type Link struct {
	Key      string
	Summary  string
	Status   string
	Type     string
	Relation string // how the ticket reads it: "Story related to test"
}

// Comment is one comment, its body as Jira's wiki text.
type Comment struct {
	Author string
	At     time.Time
	Body   string
}

// Detail is one ticket with what the card's detail shows.
type Detail struct {
	Issue
	Tests    []Link
	Links    []Link
	Comments []Comment // oldest first
}

// Latest is the newest comment.
func (d Detail) Latest() (Comment, bool) {
	if len(d.Comments) == 0 {
		return Comment{}, false
	}
	return d.Comments[len(d.Comments)-1], true
}

// Client talks to one site.
type Client struct {
	base        string
	email       string
	token       string
	http        *http.Client
	cfg         Config
	maxResults  int
	issueFields []string
}

// New is a client for cfg, or nil and why when Jira is off or the account's
// variables are not set.
func New(cfg Config) (*Client, string) {
	cfg = cfg.WithDefaults()
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, ""
	}
	u, err := url.Parse(strings.TrimRight(cfg.URL, "/"))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Sprintf("jira.url %q is not an https address — Jira is off", cfg.URL)
	}
	email, token := os.Getenv(cfg.EmailEnv), os.Getenv(cfg.TokenEnv)
	if email == "" || token == "" {
		return nil, fmt.Sprintf("Jira is off: set %s and %s (an Atlassian API token)", cfg.EmailEnv, cfg.TokenEnv)
	}
	return &Client{
		base:        u.String(),
		email:       email,
		token:       token,
		http:        &http.Client{Timeout: 20 * time.Second},
		cfg:         cfg,
		maxResults:  100,
		issueFields: []string{"summary", "status", "issuetype", "priority"},
	}, ""
}

// Config is the client's settings, defaults filled.
func (c *Client) Config() Config { return c.cfg }

// IssueKey is the shape of a ticket key; anything else is never put in a query.
var IssueKey = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

var orderBy = regexp.MustCompile(`(?is)\s+order\s+by\s+.*$`)

// Query is base, widened by the given keys: the tickets the worktrees name
// show their status even when base would leave them out.
func Query(base string, keys []string) string {
	var valid []string
	for _, k := range keys {
		if IssueKey.MatchString(k) {
			valid = append(valid, k)
		}
	}
	if len(valid) == 0 {
		return base
	}
	order := orderBy.FindString(base)
	where := strings.TrimSpace(strings.TrimSuffix(base, order))
	in := "key in (" + strings.Join(valid, ", ") + ")"
	if where == "" {
		return in + order
	}
	return "(" + where + ") OR " + in + order
}

// Search lists the tickets of base widened by keys.
func (c *Client) Search(ctx context.Context, base string, keys []string) ([]Issue, error) {
	body, _ := json.Marshal(map[string]any{
		"jql":        Query(base, keys),
		"fields":     c.issueFields,
		"maxResults": c.maxResults,
	})
	var out struct {
		Issues []rawIssue `json:"issues"`
	}
	if err := c.do(ctx, http.MethodPost, "/rest/api/3/search/jql", body, &out); err != nil {
		return nil, err
	}
	issues := make([]Issue, 0, len(out.Issues))
	for _, r := range out.Issues {
		issues = append(issues, r.issue())
	}
	return issues, nil
}

// Detail reads one ticket with its links and comments. Version 2 of the API
// gives comment bodies as text rather than a document tree.
func (c *Client) Detail(ctx context.Context, key string) (Detail, error) {
	if !IssueKey.MatchString(key) {
		return Detail{}, fmt.Errorf("%q is not a ticket key", key)
	}
	var r struct {
		rawIssue
		Fields struct {
			rawFields
			IssueLinks []struct {
				Type struct {
					Inward  string `json:"inward"`
					Outward string `json:"outward"`
				} `json:"type"`
				Inward  *rawIssue `json:"inwardIssue"`
				Outward *rawIssue `json:"outwardIssue"`
			} `json:"issuelinks"`
			Comment struct {
				Comments []struct {
					Author struct {
						DisplayName string `json:"displayName"`
					} `json:"author"`
					Created string `json:"created"`
					Body    string `json:"body"`
				} `json:"comments"`
			} `json:"comment"`
		} `json:"fields"`
	}
	path := "/rest/api/2/issue/" + url.PathEscape(key) + "?fields=" + strings.Join(append(c.issueFields, "issuelinks", "comment"), ",")
	if err := c.do(ctx, http.MethodGet, path, nil, &r); err != nil {
		return Detail{}, err
	}
	r.rawIssue.Fields = r.Fields.rawFields
	d := Detail{Issue: r.rawIssue.issue()}
	seen := map[string]bool{}
	for _, l := range r.Fields.IssueLinks {
		other, rel := l.Outward, l.Type.Outward
		if l.Inward != nil {
			other, rel = l.Inward, l.Type.Inward
		}
		if other == nil {
			continue
		}
		i := other.issue()
		link := Link{Key: i.Key, Summary: i.Summary, Status: i.Status, Type: i.Type, Relation: rel}
		if strings.EqualFold(i.Type, c.cfg.TestType) {
			if !seen[i.Key] {
				d.Tests = append(d.Tests, link)
				seen[i.Key] = true
			}
			continue
		}
		d.Links = append(d.Links, link)
	}
	for _, cm := range r.Fields.Comment.Comments {
		at, _ := time.Parse("2006-01-02T15:04:05.000-0700", cm.Created)
		d.Comments = append(d.Comments, Comment{Author: cm.Author.DisplayName, At: at, Body: strings.TrimSpace(cm.Body)})
	}
	return d, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, into any) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.email+":"+c.token)))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Jira (VPN?): %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("the account in %s / %s was refused by Jira (401)", c.cfg.EmailEnv, c.cfg.TokenEnv)
	case resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("unexpected answer from Jira (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

type rawFields struct {
	Summary string `json:"summary"`
	Status  struct {
		Name     string `json:"name"`
		Category struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"status"`
	IssueType struct {
		Name string `json:"name"`
	} `json:"issuetype"`
	Priority *struct {
		Name string `json:"name"`
	} `json:"priority"`
}

type rawIssue struct {
	Key    string    `json:"key"`
	Fields rawFields `json:"fields"`
}

func (r rawIssue) issue() Issue {
	i := Issue{
		Key:      r.Key,
		Summary:  r.Fields.Summary,
		Status:   r.Fields.Status.Name,
		Category: r.Fields.Status.Category.Key,
		Type:     r.Fields.IssueType.Name,
	}
	if r.Fields.Priority != nil {
		i.Priority = r.Fields.Priority.Name
	}
	return i
}
