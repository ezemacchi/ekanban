package bitbucket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const secret = "s3cr3t-token-value"

// server answers the three calls a review needs, for pull request 203.
func server(t *testing.T, reports string, status map[string]int) *Client {
	t.Helper()
	mux := http.NewServeMux()
	reply := func(path, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+secret {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			if code := status[path]; code != 0 {
				http.Error(w, "x", code)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	reply("/rest/api/latest/projects/PROJ/repos/app/pull-requests/203",
		`{"state":"OPEN","fromRef":{"latestCommit":"4693eb54aa"},"toRef":{"displayId":"master"},
		  "reviewers":[{"status":"UNAPPROVED","user":{"displayName":"Ana","name":"ana"}}]}`)
	reply("/rest/api/latest/projects/PROJ/repos/app/pull-requests/203/merge",
		`{"canMerge":true,"conflicted":false,"vetoes":[]}`)
	reply("/rest/insights/latest/projects/PROJ/repos/app/commits/4693eb54aa/reports", reports)
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("BB_TEST_TOKEN", secret)
	c, why := New(Config{URL: srv.URL, TokenEnv: "BB_TEST_TOKEN"})
	if c == nil {
		t.Fatalf("no client: %s", why)
	}
	c.http = srv.Client()
	return c
}

const sonarFail = `{"values":[{"key":"com.sonarsource.sonarqube","title":"SonarQube","result":"FAIL",
	"details":"Quality Gate failed\n\n- 2 New Critical Issues (required \u2264 0)\n",
	"link":"https://sonar.example/dashboard?id=x&pullRequest=203",
	"data":[{"title":"New issues","value":"5","type":"TEXT"},{"title":"Code Coverage","value":0.0,"type":"PERCENTAGE"}]}]}`

// The case that started this: the build is green and the quality gate failed.
func TestAFailedQualityGateBlocksAPullRequestBitbucketWouldMerge(t *testing.T) {
	c := server(t, sonarFail, nil)
	r, err := c.Review(context.Background(), "PROJ", "app", 203)
	if err != nil {
		t.Fatal(err)
	}
	if !r.CanMerge || len(r.Vetoes) != 0 {
		t.Fatalf("setup: Bitbucket itself sees no problem: %+v", r)
	}
	if r.Target != "master" || r.Head != "4693eb54aa" || len(r.Checks) != 1 {
		t.Fatalf("review: %+v", r)
	}
	ck := r.Checks[0]
	if ck.Title != "SonarQube" || ck.Result != "FAIL" || !strings.Contains(ck.Link, "pullRequest=203") {
		t.Fatalf("check: %+v", ck)
	}
	if len(ck.Metrics) != 2 || ck.Metrics[1].Title != "Code Coverage" || ck.Metrics[1].Value != "0%" {
		t.Fatalf("metrics: %+v", ck.Metrics)
	}

	j := r.Judge(false)
	if j.Verdict != Blocked || len(j.Why) != 1 {
		t.Fatalf("judgement: %+v", j)
	}
	if j.Why[0] != "SonarQube failed" {
		t.Fatalf("reason: %q", j.Why[0])
	}
	// The report's own words are kept for the detail view.
	if got := FirstLines(ck.Details, 2); !strings.HasPrefix(got, "Quality Gate failed") || !strings.Contains(got, "2 New Critical Issues") {
		t.Fatalf("details: %q", got)
	}
}

func TestAPassingReviewIsFit(t *testing.T) {
	c := server(t, `{"values":[{"key":"k","title":"SonarQube","result":"PASS"}]}`, nil)
	r, err := c.Review(context.Background(), "PROJ", "app", 203)
	if err != nil {
		t.Fatal(err)
	}
	if j := r.Judge(false); j.Verdict != Fit || len(j.Why) != 0 {
		t.Fatalf("%+v", j)
	}
}

func TestAServerWithoutReportsStillAnswers(t *testing.T) {
	c := server(t, `{}`, map[string]int{"/rest/insights/latest/projects/PROJ/repos/app/commits/4693eb54aa/reports": http.StatusNotFound})
	r, err := c.Review(context.Background(), "PROJ", "app", 203)
	if err != nil {
		t.Fatalf("no Code Insights is not a failure: %v", err)
	}
	if len(r.Checks) != 0 || r.Judge(false).Verdict != Fit {
		t.Fatalf("%+v", r)
	}
}

func TestAMergedPullRequestIsNotJudged(t *testing.T) {
	r := Review{State: "MERGED", CanMerge: false, Conflicted: true}
	if j := r.Judge(false); j.Verdict != NoVerdict {
		t.Fatalf("%+v", j)
	}
}

func TestJudge(t *testing.T) {
	pass := Check{Title: "SonarQube", Result: "PASS"}
	cases := []struct {
		name    string
		r       Review
		running bool
		want    Verdict
		why     string
	}{
		{"clean", Review{State: "OPEN", CanMerge: true, Checks: []Check{pass}}, false, Fit, ""},
		{"conflicts", Review{State: "OPEN", Conflicted: true}, false, Blocked, "merge conflicts"},
		{"report not finished", Review{State: "OPEN", CanMerge: true, Checks: []Check{{Title: "SonarQube", Result: "PENDING"}}}, false, Waiting, "SonarQube not finished"},
		{"changes requested", Review{State: "OPEN", CanMerge: true, Reviewers: []Reviewer{{Name: "Ana", Status: "NEEDS_WORK"}}}, false, Blocked, "changes requested by Ana"},
		{"unapproved reviewer alone does not block", Review{State: "OPEN", CanMerge: true, Reviewers: []Reviewer{{Name: "Ana", Status: "UNAPPROVED"}}}, false, Fit, ""},
		{"veto", Review{State: "OPEN", Vetoes: []Veto{{Summary: "Needs 1 approval"}}}, false, Blocked, "Needs 1 approval"},
		{"veto while the build runs is a wait", Review{State: "OPEN", Vetoes: []Veto{{Summary: "Required builds not successful yet"}}}, true, Waiting, "Required builds"},
		{"build running", Review{State: "OPEN", CanMerge: true}, true, Waiting, "build running"},
		{"a failure outranks a running build", Review{State: "OPEN", CanMerge: true, Checks: []Check{{Title: "Sonar", Result: "FAIL"}}}, true, Blocked, "Sonar failed"},
		{"cannot merge for no stated reason", Review{State: "OPEN"}, false, Blocked, "will not merge"},
	}
	for _, tc := range cases {
		j := tc.r.Judge(tc.running)
		if j.Verdict != tc.want || (tc.why != "" && !strings.Contains(strings.Join(j.Why, "|"), tc.why)) {
			t.Errorf("%s: %+v, want verdict %v with %q", tc.name, j, tc.want, tc.why)
		}
	}
}

// A refused token is reported by name, never by value.
func TestARefusedTokenIsNamedNotShown(t *testing.T) {
	c := server(t, `{}`, nil)
	c.token = "wrong-" + secret
	_, err := c.Review(context.Background(), "PROJ", "app", 203)
	if err == nil {
		t.Fatal("a wrong token must fail")
	}
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "BB_TEST_TOKEN") {
		t.Fatalf("error: %v", err)
	}
}

func TestAMissingPullRequestIsNamed(t *testing.T) {
	c := server(t, `{}`, map[string]int{"/rest/api/latest/projects/PROJ/repos/app/pull-requests/203": http.StatusNotFound})
	_, err := c.Review(context.Background(), "PROJ", "app", 203)
	if err == nil || !strings.Contains(err.Error(), "#203 was not found in PROJ/app") {
		t.Fatalf("%v", err)
	}
}

func TestNeverAsksForAnUnsafePath(t *testing.T) {
	c := server(t, `{}`, nil)
	for _, bad := range [][2]string{{"PROJ/../x", "app"}, {"PROJ", "a b"}, {"", "app"}} {
		if _, err := c.Review(context.Background(), bad[0], bad[1], 1); err == nil {
			t.Errorf("%q/%q was accepted", bad[0], bad[1])
		}
	}
}

func TestNewNeedsHTTPSAndAToken(t *testing.T) {
	t.Setenv("BB_TEST_TOKEN", "")
	if c, why := New(Config{}); c != nil || why != "" {
		t.Fatalf("no url is off, quietly: %v %q", c, why)
	}
	if c, why := New(Config{URL: "http://bb.example", TokenEnv: "BB_TEST_TOKEN"}); c != nil || !strings.Contains(why, "https") {
		t.Fatalf("plain http must be refused: %q", why)
	}
	if c, why := New(Config{URL: "https://bb.example", TokenEnv: "BB_TEST_TOKEN"}); c != nil || !strings.Contains(why, "BB_TEST_TOKEN") {
		t.Fatalf("a missing token is named: %q", why)
	}
	if got := (Config{}).WithDefaults().TokenEnv; got != DefaultTokenEnv {
		t.Fatalf("default token variable %q", got)
	}
}

func TestLocationOf(t *testing.T) {
	loc, ok := LocationOf("https://bitbucket.example.com/projects/PROJ/repos/app/pull-requests/{pr}")
	if !ok || loc.Project != "PROJ" || loc.Repo != "app" {
		t.Fatalf("%+v %v", loc, ok)
	}
	if _, ok := LocationOf("https://github.com/o/r/pull/{pr}"); ok {
		t.Fatal("a GitHub address is not a Bitbucket one")
	}
}

func TestFirstLines(t *testing.T) {
	got := FirstLines("Quality Gate failed\n\n? 2 New Critical Issues (required \u2264 0)\nthird\n", 2)
	if got != "Quality Gate failed · 2 New Critical Issues (required \u2264 0)" {
		t.Fatalf("%q", got)
	}
}
