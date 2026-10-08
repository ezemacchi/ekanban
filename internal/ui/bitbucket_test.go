package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/bitbucket"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/store"
)

// bitbucketBoard is a pipeline board whose api card has pull request 203 open
// with a green build, and Bitbucket turned on without a server to ask.
func bitbucketBoard(t *testing.T) *Model {
	t.Helper()
	t.Setenv("BB_UI_TEST_TOKEN", "x")
	c, why := bitbucket.New(bitbucket.Config{URL: "https://bb.example", TokenEnv: "BB_UI_TEST_TOKEN"})
	if c == nil {
		t.Fatal(why)
	}
	m := pipelineBoard(t)
	m.SetBitbucket(c, bitbucket.Location{Project: "PROJ", Repo: "app"}, "")
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key(tmp + "api"): {Stage: pipeline.ReadyQA, Key: "ABC-1", Title: "Arreglar la grilla", PR: 203, PROpen: true, Build: "SUCCESS"},
		store.Key(tmp + "web"): {},
	}})
	m.layout = layoutKanban
	m.width, m.height = 200, 50
	return m
}

func review(checks ...bitbucket.Check) bitbucket.Review {
	return bitbucket.Review{Number: 203, State: "OPEN", Target: "master", CanMerge: true, Checks: checks}
}

const sonarPage = "https://sonar.example/dashboard?id=x&pullRequest=203"

var sonarFailed = bitbucket.Check{Key: "sonar", Title: "SonarQube", Result: "FAIL", Link: sonarPage,
	Details: "Quality Gate failed\n\n- 2 New Critical Issues (required \u2264 0)",
	Metrics: []bitbucket.Metric{{Title: "New issues", Value: "5"}, {Title: "Code Coverage", Value: "0%"}}}

// The case that started this: Jenkins is green, Bitbucket would merge it, and
// SonarQube's quality gate failed. The card must not look ready.
func TestACardWithAGreenBuildAndAFailedQualityGateIsBlocked(t *testing.T) {
	m := bitbucketBoard(t)
	m.bb.reviews[203] = review(sonarFailed)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "PR #203") || !strings.Contains(out, "green") {
		t.Fatalf("setup: the build is green:\n%s", out)
	}
	if !strings.Contains(out, "blocked: SonarQube failed") {
		t.Fatalf("the card does not say what blocks it:\n%s", out)
	}
	if strings.Contains(out, "ready to merge") {
		t.Fatalf("a blocked pull request reads as ready:\n%s", out)
	}
}

func TestAPassingReviewReadsAsReadyToMerge(t *testing.T) {
	m := bitbucketBoard(t)
	m.bb.reviews[203] = review(bitbucket.Check{Title: "SonarQube", Result: "PASS"})
	if out := ansi.Strip(m.View()); !strings.Contains(out, "ready to merge") {
		t.Fatalf("no verdict:\n%s", out)
	}
}

// While Jenkins is still running, Bitbucket refusing for unfinished builds is
// a wait, not a block.
func TestAnUnfinishedBuildIsAWaitNotABlock(t *testing.T) {
	m := bitbucketBoard(t)
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key(tmp + "api"): {Stage: pipeline.ReadyQA, Key: "ABC-1", PR: 203, PROpen: true, Build: "RUNNING"},
		store.Key(tmp + "web"): {},
	}})
	r := review()
	r.CanMerge = false
	r.Vetoes = []bitbucket.Veto{{Summary: "Required builds not successful yet"}}
	m.bb.reviews[203] = r
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "waiting:") || strings.Contains(out, "blocked:") {
		t.Fatalf("a running build should read as waiting:\n%s", out)
	}
}

func TestAMergedPullRequestGetsNoVerdictAndIsNotAskedAbout(t *testing.T) {
	m := bitbucketBoard(t)
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key(tmp + "api"): {Stage: pipeline.ReadyQA, Key: "ABC-1", PR: 203, MergedTo: "master", Build: "SUCCESS"},
		store.Key(tmp + "web"): {},
	}})
	m.bb.reviews[203] = review(sonarFailed)
	if out := ansi.Strip(m.View()); strings.Contains(out, "blocked:") || strings.Contains(out, "ready to merge") {
		t.Fatalf("a merged pull request was judged:\n%s", out)
	}
	if got := m.bbWanted(); len(got) != 0 {
		t.Fatalf("asking Bitbucket about merged pull requests: %v", got)
	}
}

func TestBitbucketOffMeansNoLine(t *testing.T) {
	m := pipelineBoard(t)
	send(t, m, pipelineMsg{infos: map[string]pipeline.Info{
		store.Key(tmp + "api"): {Stage: pipeline.ReadyQA, Key: "ABC-1", PR: 203, Build: "SUCCESS"},
		store.Key(tmp + "web"): {},
	}})
	m.width, m.height = 200, 50
	m.bb.reviews = map[int]bitbucket.Review{203: review(sonarFailed)}
	if out := ansi.Strip(m.View()); strings.Contains(out, "blocked:") || strings.Contains(out, "Bitbucket") {
		t.Fatalf("Bitbucket is off but the board mentions it:\n%s", out)
	}
	if cmd := m.loadBitbucket(true); cmd != nil {
		t.Fatal("asked a server that is not configured")
	}
}

// A server that cannot be read says so on the card rather than showing
// nothing, which would read as "no problem".
func TestAnUnreadableServerIsSaidOnTheCard(t *testing.T) {
	m := bitbucketBoard(t)
	m.bb.errs[203] = "cannot reach Bitbucket (VPN?)"
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Bitbucket: cannot reach") {
		t.Fatalf("no word about the failure:\n%s", out)
	}
}

func TestEachPullRequestIsReadOncePerInterval(t *testing.T) {
	m := bitbucketBoard(t)
	// Learning of the pull request (the pipeline message) starts its first read.
	if !m.bb.loading {
		t.Fatal("a pull request never read was not read")
	}
	m.bb.loading = false // the command was not run; let it be asked for again
	if cmd := m.loadBitbucket(false); cmd == nil || !m.bb.loading {
		t.Fatal("a pull request never read must be read")
	}
	m.applyBitbucket(bitbucketMsg{reviews: map[int]bitbucket.Review{203: review()}})
	if m.bb.loading {
		t.Fatal("still loading after the answer")
	}
	if cmd := m.loadBitbucket(false); cmd != nil {
		t.Fatal("read again before the interval passed")
	}
	if cmd := m.loadBitbucket(true); cmd == nil {
		t.Fatal("r must read again")
	}
	m.applyBitbucket(bitbucketMsg{reviews: map[int]bitbucket.Review{203: review()}})
	m.bb.at[203] = time.Now().Add(-2 * pipelineEvery)
	if cmd := m.loadBitbucket(false); cmd == nil {
		t.Fatal("an old answer must be read again")
	}
}

// A failed read keeps the last good answer and does not hammer the server.
func TestAFailedReadKeepsTheLastAnswerAndWaits(t *testing.T) {
	m := bitbucketBoard(t)
	m.applyBitbucket(bitbucketMsg{reviews: map[int]bitbucket.Review{203: review(sonarFailed)}})
	m.applyBitbucket(bitbucketMsg{errs: map[int]error{203: errString("cannot reach Bitbucket (VPN?)")}})
	if _, ok := m.bb.reviews[203]; !ok {
		t.Fatal("the last good answer was dropped")
	}
	if cmd := m.loadBitbucket(false); cmd != nil {
		t.Fatal("asked again straight after a failure")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestTheDetailExplainsTheBlockAndLinksToSonar(t *testing.T) {
	m := bitbucketBoard(t)
	m.bb.reviews[203] = review(sonarFailed)
	selectSpace(t, m, tmp+"api")
	var text []string
	var urls []string
	for _, l := range m.detailBody(m.selected(), 80) {
		text = append(text, ansi.Strip(l.text))
		if l.url != "" {
			urls = append(urls, l.url)
		}
	}
	body := strings.Join(text, "\n")
	for _, want := range []string{"SonarQube failed", "Quality Gate failed", "2 New Critical Issues", "Code Coverage 0%"} {
		if !strings.Contains(body, want) {
			t.Fatalf("detail lacks %q:\n%s", want, body)
		}
	}
	found := false
	for _, u := range urls {
		found = found || u == sonarPage
	}
	if !found {
		t.Fatalf("the SonarQube line does not open its page: %v", urls)
	}
}

// A link from the server is only offered when it is https.
func TestAReportLinkThatIsNotHTTPSIsNotOffered(t *testing.T) {
	m := bitbucketBoard(t)
	bad := sonarFailed
	bad.Link = "file:///C:/Windows/System32/calc.exe"
	m.bb.reviews[203] = review(bad)
	selectSpace(t, m, tmp+"api")
	for _, l := range m.detailBody(m.selected(), 80) {
		if strings.HasPrefix(l.url, "file:") {
			t.Fatalf("offered %q", l.url)
		}
	}
}

func TestTheCopiedContextCarriesTheVerdict(t *testing.T) {
	m := bitbucketBoard(t)
	m.bb.reviews[203] = review(sonarFailed)
	info := m.pipeInfo[store.Key(tmp+"api")]
	text := yankText(yankFacts{Key: "ABC-1", Info: info, PRURL: "https://bb.example/pr/203", CI: "Jenkins", Merge: m.bitbucketYank(info)})
	for _, want := range []string{"Merge check: blocked — SonarQube failed", "Report: SonarQube: fail — Quality Gate failed", sonarPage} {
		if !strings.Contains(text, want) {
			t.Fatalf("copied text lacks %q:\n%s", want, text)
		}
	}
}
