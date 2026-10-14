package reviews

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ezemacchi/ekanban/internal/bitbucket"
)

type fakeSource struct{}

func (fakeSource) Name() string                                { return "fake" }
func (fakeSource) Awaiting(context.Context) ([]Request, error) { return nil, nil }

func TestNewIsOffWithoutAKindAndSaysWhyForAWrongOne(t *testing.T) {
	kinds := map[string]Builder{
		"fake":   func(Config) (Source, error) { return fakeSource{}, nil },
		"broken": func(Config) (Source, error) { return nil, errors.New("needs a url") },
	}
	if src, why := New(Config{}, kinds); src != nil || why != "" {
		t.Fatalf("no kind: %v %q", src, why)
	}
	if src, why := New(Config{Kind: "fake"}, kinds); src == nil || why != "" {
		t.Fatalf("fake: %v %q", src, why)
	}
	if _, why := New(Config{Kind: "gitea"}, kinds); !strings.Contains(why, `"gitea" is not known (known: broken, fake)`) {
		t.Fatalf("unknown kind: %q", why)
	}
	if _, why := New(Config{Kind: "broken"}, kinds); !strings.Contains(why, "needs a url") {
		t.Fatalf("builder error: %q", why)
	}
}

func TestCardColumnDefaultsAndCanBeTurnedOff(t *testing.T) {
	empty, qa := "", "ready_qa"
	for _, c := range []struct {
		col  *string
		want string
	}{{nil, "on_review"}, {&empty, ""}, {&qa, "ready_qa"}} {
		if got := (Config{Column: c.col}).CardColumn("on_review"); got != c.want {
			t.Fatalf("CardColumn = %q, want %q", got, c.want)
		}
	}
}

func TestParseRequestsTakesNumbersOrStringsAndDropsWhatHasNoID(t *testing.T) {
	got, err := parseRequests([]byte(`[
		{"id": 12, "title": "PROJ-1 Fix", "author": "ana", "url": "https://git.example.com/pr/12", "updated": "2026-01-02T03:04:05Z"},
		{"id": "!7", "title": "MR"},
		{"title": "no id"},
		{"id": 12, "title": "twice"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "12" || got[0].Author != "ana" || got[0].Updated.IsZero() || got[1].ID != "!7" {
		t.Fatalf("got %+v", got)
	}
	if _, err := parseRequests([]byte("not json")); err == nil {
		t.Fatal("bad output was accepted")
	}
}

// The test binary stands in for the review command.
func TestMain(m *testing.M) {
	switch os.Getenv("EK_REVIEWS_HELPER") {
	case "list":
		fmt.Print(`[{"id": 3, "title": "PROJ-3 Thing"}]`)
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stderr, "first line\nnot logged in")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func helper(t *testing.T, mode string) Source {
	t.Helper()
	t.Setenv("EK_REVIEWS_HELPER", mode)
	src, err := Command(t.TempDir(), nil)(Config{Command: []string{os.Args[0], "-test.run=^$"}})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestTheCommandKindRunsTheProgramAndReadsItsList(t *testing.T) {
	got, err := helper(t, "list").Awaiting(context.Background())
	if err != nil || len(got) != 1 || got[0].ID != "3" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestTheCommandKindReportsTheProgramsLastErrorLine(t *testing.T) {
	_, err := helper(t, "fail").Awaiting(context.Background())
	if err == nil || !strings.HasSuffix(err.Error(), ": not logged in") {
		t.Fatalf("err = %v", err)
	}
}

func TestTheCommandKindNeedsACommand(t *testing.T) {
	if _, err := Command("", nil)(Config{}); err == nil {
		t.Fatal("no command was accepted")
	}
}

type fakeReader struct {
	project, repo string
	list          []bitbucket.Awaiting
}

func (f *fakeReader) AwaitingReview(_ context.Context, project, repo string) ([]bitbucket.Awaiting, error) {
	f.project, f.repo = project, repo
	return f.list, nil
}

func TestTheBitbucketKindAsksForTheBoardsRepositoryAndFillsMissingLinks(t *testing.T) {
	when := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	r := &fakeReader{list: []bitbucket.Awaiting{
		{Number: 5, Title: "PROJ-5 A", Author: "Ana", MyStatus: "UNAPPROVED", Updated: when},
		{Number: 6, MyStatus: "NEEDS_WORK", Link: "https://bb.example/own/6"},
	}}
	build := Bitbucket(r, bitbucket.Location{Project: "PROJ", Repo: "app"}, func(n int) string { return fmt.Sprintf("https://bb.example/pr/%d", n) }, "")
	src, err := build(Config{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := src.Awaiting(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.project != "PROJ" || r.repo != "app" {
		t.Fatalf("asked about %s/%s", r.project, r.repo)
	}
	if got[0].ID != "5" || got[0].URL != "https://bb.example/pr/5" || got[0].Status != "not reviewed yet" || !got[0].Updated.Equal(when) {
		t.Fatalf("first: %+v", got[0])
	}
	if got[1].URL != "https://bb.example/own/6" || got[1].Status != "you asked for changes" {
		t.Fatalf("second: %+v", got[1])
	}
}

func TestTheBitbucketKindSaysWhyWhenBitbucketIsOff(t *testing.T) {
	if _, err := Bitbucket(nil, bitbucket.Location{}, nil, "set BITBUCKET_TOKEN")(Config{}); err == nil || err.Error() != "set BITBUCKET_TOKEN" {
		t.Fatalf("err = %v", err)
	}
}
