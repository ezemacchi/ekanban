package jira

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{base: srv.URL, email: "me@example.com", token: "secret", http: srv.Client(),
		cfg: Config{}.WithDefaults(), maxResults: 100, issueFields: []string{"summary", "status", "issuetype", "priority"}}
}

func TestQueryAddsTheWorktreeKeysBeforeTheOrder(t *testing.T) {
	got := Query("assignee = currentUser() ORDER BY updated DESC", []string{"ABC-1", "bad key", "XY-22"})
	want := "(assignee = currentUser()) OR key in (ABC-1, XY-22) ORDER BY updated DESC"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if got := Query("project = X", nil); got != "project = X" {
		t.Fatalf("no keys changed the query: %q", got)
	}
}

func TestSearchReadsStatusAndCategory(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" || r.Method != http.MethodPost {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "me@example.com" || pass != "secret" {
			t.Error("no basic auth")
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req["jql"].(string), "key in (ABC-7)") {
			t.Errorf("jql %v", req["jql"])
		}
		_, _ = w.Write([]byte(`{"issues":[{"key":"ABC-7","fields":{"summary":"Do it","status":{"name":"In Implementation","statusCategory":{"key":"indeterminate"}},"issuetype":{"name":"Story"},"priority":{"name":"High"}}}]}`))
	})
	got, err := c.Search(context.Background(), DefaultJQL, []string{"ABC-7"})
	if err != nil {
		t.Fatal(err)
	}
	want := Issue{Key: "ABC-7", Summary: "Do it", Status: "In Implementation", Category: "indeterminate", Type: "Story", Priority: "High"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("%+v", got)
	}
}

func TestDetailSplitsTestsFromOtherLinks(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/issue/ABC-7" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"key":"ABC-7","fields":{"summary":"Do it","status":{"name":"In Implementation","statusCategory":{"key":"indeterminate"}},"issuetype":{"name":"Story"},
			"issuelinks":[
				{"type":{"inward":"Story related to test","outward":"Test related to story"},"inwardIssue":{"key":"ABC-8","fields":{"summary":"Shows the dialog","status":{"name":"Defined"},"issuetype":{"name":"Test"}}}},
				{"type":{"inward":"is part of","outward":"is part of"},"inwardIssue":{"key":"ABC-8","fields":{"summary":"Shows the dialog","status":{"name":"Defined"},"issuetype":{"name":"Test"}}}},
				{"type":{"inward":"relates to","outward":"relates to"},"outwardIssue":{"key":"ABC-9","fields":{"summary":"Other","status":{"name":"New"},"issuetype":{"name":"Story"}}}}],
			"comment":{"comments":[
				{"author":{"displayName":"Ann"},"created":"2026-10-07T10:00:00.000+0200","body":"first"},
				{"author":{"displayName":"Bo"},"created":"2026-10-08T14:58:18.034+0200","body":"Failed: ABC-8\n"}]}}}`))
	})
	d, err := c.Detail(context.Background(), "ABC-7")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != "In Implementation" || len(d.Tests) != 1 || d.Tests[0].Relation != "Story related to test" {
		t.Fatalf("tests %+v", d.Tests)
	}
	if len(d.Links) != 1 || d.Links[0].Key != "ABC-9" || d.Links[0].Relation != "relates to" {
		t.Fatalf("links %+v", d.Links)
	}
	latest, ok := d.Latest()
	if !ok || latest.Author != "Bo" || latest.Body != "Failed: ABC-8" || latest.At.IsZero() {
		t.Fatalf("latest %+v", latest)
	}
}

func TestUnauthorizedNamesTheVariables(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	_, err := c.Search(context.Background(), DefaultJQL, nil)
	if err == nil || !strings.Contains(err.Error(), DefaultTokenEnv) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("%v", err)
	}
}

func TestNewNeedsHTTPSAndTheVariables(t *testing.T) {
	if c, why := New(Config{}); c != nil || why != "" {
		t.Fatal("no url should be off without a complaint")
	}
	if c, why := New(Config{URL: "http://site.example"}); c != nil || why == "" {
		t.Fatal("http accepted")
	}
	t.Setenv("EK_MAIL", "")
	if c, why := New(Config{URL: "https://site.example", EmailEnv: "EK_MAIL", TokenEnv: "EK_TOKEN"}); c != nil || !strings.Contains(why, "EK_TOKEN") {
		t.Fatalf("missing variables: %q", why)
	}
	t.Setenv("EK_MAIL", "a@b")
	t.Setenv("EK_TOKEN", "t")
	if c, _ := New(Config{URL: "https://site.example/", EmailEnv: "EK_MAIL", TokenEnv: "EK_TOKEN"}); c == nil || c.base != "https://site.example" {
		t.Fatal("not built")
	}
}

func TestColumnByStatusThenCategory(t *testing.T) {
	c := Config{Columns: map[string]string{"In Quality Review": "ready_qa", "new": "todo", "Blocked": ""}}
	for _, tc := range []struct {
		issue Issue
		col   string
		show  bool
	}{
		{Issue{Status: "In Quality Review", Category: "indeterminate"}, "ready_qa", true},
		{Issue{Status: "In Implementation", Category: "indeterminate"}, "in_progress", true},
		{Issue{Status: "Ready", Category: "new"}, "todo", true},
		{Issue{Status: "Blocked", Category: "indeterminate"}, "", false},
		{Issue{Status: "Closed", Category: "done"}, "", false},
	} {
		col, show := c.Column(tc.issue)
		if col != tc.col || show != tc.show {
			t.Errorf("%s: %q %v", tc.issue.Status, col, show)
		}
	}
}

func TestHandoffArgsKeepEachValueOneArgument(t *testing.T) {
	h := Handoff{Command: []string{"run", "-Key", "{key}", "-Summary", "{summary}", "-Type", "{type}"}, FixTypes: DefaultFixTypes}
	got := h.Args(map[string]string{"key": "ABC-1", "summary": "a; rm -rf / & b", "type": h.Type("Bug")})
	want := []string{"run", "-Key", "ABC-1", "-Summary", "a; rm -rf / & b", "-Type", "fix"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", got)
	}
	if h.Type("Story") != "feat" {
		t.Fatal("a story is a fix")
	}
}
