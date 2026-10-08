// Package bitbucket reads what a Bitbucket Server pull request says about
// whether it can go in: Bitbucket's own merge checks, the code reviewers'
// verdicts, and the reports other tools post on the commit (SonarQube's quality
// gate is one). It is read only and holds no state.
//
// Jenkins turning green is not enough to merge: Bitbucket can still refuse a
// pull request because a quality gate failed, a required approval is missing
// or the branch conflicts. Judge turns those facts into one answer.
package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config is [bitbucket] in config.toml. It is read from that file only: the
// token is sent to url, so a repository must not be able to choose it.
type Config struct {
	// URL is the server's root, for example https://bitbucket.example.com.
	// Empty turns Bitbucket off.
	URL string `toml:"url"`
	// TokenEnv names the environment variable holding a personal access token
	// that can read the repository. The token is never kept in a file.
	TokenEnv string `toml:"token_env"`
}

// DefaultTokenEnv is where the token is read from when token_env is not set.
const DefaultTokenEnv = "BITBUCKET_TOKEN"

// WithDefaults fills what was left out.
func (c Config) WithDefaults() Config {
	if c.TokenEnv == "" {
		c.TokenEnv = DefaultTokenEnv
	}
	return c
}

// Client reads one Bitbucket Server.
type Client struct {
	base  string
	token string
	env   string
	http  *http.Client
}

// New builds the client. A nil client with an empty reason means Bitbucket is
// off by choice (no url); with a reason, it is off because something it needs
// is missing, and the reason is for the status line.
func New(cfg Config) (*Client, string) {
	cfg = cfg.WithDefaults()
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, ""
	}
	u, err := url.Parse(strings.TrimRight(cfg.URL, "/"))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Sprintf("bitbucket.url %q is not an https address — Bitbucket is off", cfg.URL)
	}
	token := os.Getenv(cfg.TokenEnv)
	if token == "" {
		return nil, fmt.Sprintf("Bitbucket is off: set %s (a personal access token that can read the repository)", cfg.TokenEnv)
	}
	return &Client{base: u.String(), token: token, env: cfg.TokenEnv, http: &http.Client{Timeout: 20 * time.Second}}, ""
}

// Metric is one figure a report carries, such as "Code Coverage 61%".
type Metric struct{ Title, Value string }

// Check is a report posted on the commit by another tool.
type Check struct {
	Key     string
	Title   string
	Result  string // PASS, FAIL, or anything else while it is not finished
	Details string
	Link    string
	Metrics []Metric
}

// Veto is one reason Bitbucket gives for refusing the merge.
type Veto struct{ Summary, Detail string }

// Reviewer is one person asked to review, and what they said.
type Reviewer struct{ Name, Status string }

// Review is everything Bitbucket says about one pull request.
type Review struct {
	Number     int
	State      string // OPEN, MERGED, DECLINED
	Target     string // the branch it goes into
	Head       string // the commit the reports belong to
	CanMerge   bool
	Conflicted bool
	Vetoes     []Veto
	Reviewers  []Reviewer
	Checks     []Check
}

var (
	pathPart = regexp.MustCompile(`^[A-Za-z0-9_.~-]+$`)
	commitID = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
)

// Review reads pull request number n of project/repo. Reports are optional:
// a server without Code Insights answers 404 for them and the review simply
// has none.
func (c *Client) Review(ctx context.Context, project, repo string, n int) (Review, error) {
	if !pathPart.MatchString(project) || !pathPart.MatchString(repo) || n <= 0 {
		return Review{}, fmt.Errorf("not a Bitbucket repository: %q/%q", project, repo)
	}
	repoPath := "/projects/" + url.PathEscape(project) + "/repos/" + url.PathEscape(repo)
	pr := fmt.Sprintf("/rest/api/latest%s/pull-requests/%d", repoPath, n)

	var raw struct {
		State   string `json:"state"`
		FromRef struct {
			Latest string `json:"latestCommit"`
		} `json:"fromRef"`
		ToRef struct {
			Name string `json:"displayId"`
		} `json:"toRef"`
		Reviewers []struct {
			Status string `json:"status"`
			User   struct {
				Name    string `json:"displayName"`
				Account string `json:"name"`
			} `json:"user"`
		} `json:"reviewers"`
	}
	if err := c.get(ctx, pr, &raw); err != nil {
		if isNotFound(err) {
			return Review{}, fmt.Errorf("pull request #%d was not found in %s/%s", n, project, repo)
		}
		return Review{}, err
	}
	r := Review{Number: n, State: raw.State, Target: raw.ToRef.Name, Head: raw.FromRef.Latest}
	for _, rv := range raw.Reviewers {
		name := rv.User.Name
		if name == "" {
			name = rv.User.Account
		}
		r.Reviewers = append(r.Reviewers, Reviewer{Name: name, Status: rv.Status})
	}
	if r.State != "OPEN" {
		return r, nil // merged or declined: nothing left to judge
	}

	var merge struct {
		CanMerge   bool `json:"canMerge"`
		Conflicted bool `json:"conflicted"`
		Vetoes     []struct {
			Summary string `json:"summaryMessage"`
			Detail  string `json:"detailedMessage"`
		} `json:"vetoes"`
	}
	if err := c.get(ctx, pr+"/merge", &merge); err != nil {
		return Review{}, err
	}
	r.CanMerge, r.Conflicted = merge.CanMerge, merge.Conflicted
	for _, v := range merge.Vetoes {
		r.Vetoes = append(r.Vetoes, Veto{Summary: v.Summary, Detail: v.Detail})
	}

	if commitID.MatchString(r.Head) {
		var reports struct {
			Values []struct {
				Key     string `json:"key"`
				Title   string `json:"title"`
				Result  string `json:"result"`
				Details string `json:"details"`
				Link    string `json:"link"`
				Data    []struct {
					Title string          `json:"title"`
					Value json.RawMessage `json:"value"`
					Type  string          `json:"type"`
				} `json:"data"`
			} `json:"values"`
		}
		err := c.get(ctx, "/rest/insights/latest"+repoPath+"/commits/"+r.Head+"/reports?limit=50", &reports)
		if err != nil && !isNotFound(err) {
			return Review{}, err
		}
		for _, v := range reports.Values {
			ck := Check{Key: v.Key, Title: v.Title, Result: strings.ToUpper(v.Result), Details: v.Details, Link: v.Link}
			if ck.Title == "" {
				ck.Title = v.Key
			}
			for _, d := range v.Data {
				ck.Metrics = append(ck.Metrics, Metric{Title: d.Title, Value: metricValue(d.Value, d.Type)})
			}
			r.Checks = append(r.Checks, ck)
		}
	}
	return r, nil
}

// metricValue is a report figure as text: a percentage keeps its sign.
func metricValue(raw json.RawMessage, kind string) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		out := strconv.FormatFloat(f, 'f', -1, 64)
		if kind == "PERCENTAGE" {
			out += "%"
		}
		return out
	}
	return strings.Trim(string(raw), `"`)
}

// notFound marks an answer of 404, which is an answer rather than a failure.
type notFound struct{}

func (notFound) Error() string { return "not found" }

func isNotFound(err error) bool { _, ok := err.(notFound); return ok }

func (c *Client) get(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Bitbucket (VPN?): %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("the token in %s was refused by Bitbucket (%d)", c.env, resp.StatusCode)
	case resp.StatusCode == http.StatusNotFound:
		return notFound{}
	case resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("unexpected answer from Bitbucket (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// Verdict is whether a pull request may go in.
type Verdict int

const (
	// NoVerdict: not open, or nothing to say.
	NoVerdict Verdict = iota
	// Fit: nothing stands in the way.
	Fit
	// Waiting: something is still running.
	Waiting
	// Blocked: something has to change first.
	Blocked
)

// Judgement is the verdict and the reasons behind it, most important first.
type Judgement struct {
	Verdict Verdict
	Why     []string
}

// Judge says whether the pull request is fit to merge. buildRunning is the
// build server's word that a build is still going: Bitbucket's own refusal
// while required builds are unfinished is then a wait, not a block.
func (r Review) Judge(buildRunning bool) Judgement {
	if r.State != "OPEN" {
		return Judgement{}
	}
	var blocked, waiting []string
	if r.Conflicted {
		blocked = append(blocked, "has merge conflicts")
	}
	for _, c := range r.Checks {
		switch c.Result {
		case "PASS":
		case "FAIL":
			// Short on purpose: it has to fit on a card. The report's own
			// words are in Check.Details for the detail view.
			blocked = append(blocked, c.Title+" failed")
		default:
			waiting = append(waiting, c.Title+" not finished")
		}
	}
	for _, rv := range r.Reviewers {
		if rv.Status == "NEEDS_WORK" {
			blocked = append(blocked, "changes requested by "+rv.Name)
		}
	}
	for _, v := range r.Vetoes {
		why := strings.TrimSpace(v.Summary)
		if why == "" {
			why = "refused by Bitbucket"
		}
		if buildRunning {
			waiting = append(waiting, why)
		} else {
			blocked = append(blocked, why)
		}
	}
	if buildRunning {
		waiting = append(waiting, "build running")
	}
	switch {
	case len(blocked) > 0:
		return Judgement{Verdict: Blocked, Why: blocked}
	case len(waiting) > 0:
		return Judgement{Verdict: Waiting, Why: waiting}
	case !r.CanMerge:
		return Judgement{Verdict: Blocked, Why: []string{"Bitbucket will not merge it"}}
	}
	return Judgement{Verdict: Fit}
}

// Lines is the non-empty lines of a report's text, with control characters
// and list marks removed.
func Lines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, l)
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "?•-*"))
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

// FirstLines is the first n lines of Lines joined by " · ".
func FirstLines(text string, n int) string {
	lines := Lines(text)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " · ")
}

// Location is a repository found in a pull request address.
type Location struct{ Project, Repo string }

var prAddress = regexp.MustCompile(`/projects/([^/]+)/repos/([^/]+)/pull-requests/`)

// LocationOf reads the project and repository out of a pull request address
// template such as https://host/projects/P/repos/R/pull-requests/{pr}. Only
// the path is used: the server to ask is never taken from a repository's file.
func LocationOf(template string) (Location, bool) {
	m := prAddress.FindStringSubmatch(template)
	if m == nil {
		return Location{}, false
	}
	return Location{Project: m[1], Repo: m[2]}, true
}
