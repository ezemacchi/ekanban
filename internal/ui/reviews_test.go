package ui

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/reviews"
)

// fakeReviews is a Source that is never asked: the tests send its answer.
type fakeReviews struct{}

func (fakeReviews) Name() string                                        { return "fake" }
func (fakeReviews) Awaiting(context.Context) ([]reviews.Request, error) { return nil, nil }

var ticketKey = regexp.MustCompile(pipeline.DefaultTicketKey)

// reviewBoard is a pipeline board whose api worktree holds ABC-1, with two
// pull requests waiting on you: one for ABC-1 and one for a ticket the board
// has no card for.
func reviewBoard(t *testing.T, column string) *Model {
	t.Helper()
	m := pipelineBoard(t)
	m.SetReviews(fakeReviews{}, column, ticketKey, "")
	send(t, m, reviewsMsg{list: []reviews.Request{
		{ID: "212", Title: "ABC-1 Fix the grid", Author: "Ana", Branch: "feature/ABC-1", Target: "main", URL: "https://git.example.com/pr/212", Status: "not reviewed yet"},
		{ID: "300", Title: "Tidy the menu", Author: "Bo", Branch: "feature/XYZ-9-menu", Target: "main", URL: "https://git.example.com/pr/300"},
	}})
	m.layout = layoutKanban
	m.width, m.height = 200, 50
	return m
}

func TestMatchReviewsMarksHeldTicketsAndKeepsTheRest(t *testing.T) {
	list := []reviews.Request{
		{ID: "1", Title: "ABC-1 one"},
		{ID: "2", Title: "ABC-1 again"},
		{ID: "3", Branch: "feature/XYZ-2"},
		{ID: "4", Title: "no key"},
	}
	marks, rest := matchReviews(list, func(k string) bool { return k == "ABC-1" }, ticketKey)
	if len(marks) != 1 || marks["ABC-1"].ID != "1" {
		t.Fatalf("marks %v", marks)
	}
	var ids []string
	for _, r := range rest {
		ids = append(ids, r.ID)
	}
	if strings.Join(ids, " ") != "3 4" {
		t.Fatalf("unmatched %v", ids)
	}
}

func TestARequestWithNoCardGetsOneInTheReviewColumn(t *testing.T) {
	m := reviewBoard(t, pipeline.OnReview)
	var keys []string
	for _, sp := range m.groups[pipeline.OnReview] {
		keys = append(keys, sp.Key)
	}
	if strings.Join(keys, " ") != reviewPrefix+"300" {
		t.Fatalf("Reviewing holds %v", keys)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "XYZ-9 #300") || !strings.Contains(out, "review #300 by Bo") {
		t.Fatalf("the review card does not say what it is:\n%s", out)
	}
}

func TestARequestForATicketOnTheBoardMarksThatCardInstead(t *testing.T) {
	m := reviewBoard(t, pipeline.OnReview)
	for _, st := range m.board.Statuses {
		for _, sp := range m.groups[st.ID] {
			if sp.Key == reviewPrefix+"212" {
				t.Fatalf("a second card for ABC-1, in %s", st.ID)
			}
		}
	}
	selectSpace(t, m, tmp+"api")
	if a := m.attention(m.selected()); a != "review #212 by Ana" {
		t.Fatalf("the worktree's card does not ask for the review: %q", a)
	}
}

func TestReviewCardsAreAmongTheCardsThatNeedYou(t *testing.T) {
	m := reviewBoard(t, pipeline.OnReview)
	var keys []string
	for _, sp := range m.needingYou() {
		keys = append(keys, m.spaceName(sp))
	}
	if !strings.Contains(strings.Join(keys, ","), "XYZ-9 #300") {
		t.Fatalf("needing you: %v", keys)
	}
}

func TestAnEmptyColumnMarksCardsButAddsNone(t *testing.T) {
	m := reviewBoard(t, "")
	for _, st := range m.board.Statuses {
		for _, sp := range m.groups[st.ID] {
			if _, ok := reviewCardKey(sp.Key); ok {
				t.Fatalf("a review card in %s", st.ID)
			}
		}
	}
	selectSpace(t, m, tmp+"api")
	if m.attention(m.selected()) == "" {
		t.Fatal("the mark is gone too")
	}
}

func TestReviewsOffMeansNoCardsAndNoReads(t *testing.T) {
	m := pipelineBoard(t)
	m.rv.list = []reviews.Request{{ID: "300", Title: "XYZ-9"}}
	m.rebuild()
	for _, st := range m.board.Statuses {
		for _, sp := range m.groups[st.ID] {
			if _, ok := reviewCardKey(sp.Key); ok {
				t.Fatal("a review card with reviews off")
			}
		}
	}
	if m.loadReviews(true) != nil {
		t.Fatal("asked a source that is not configured")
	}
}

func TestAReviewCardHasNoWorktreeToGoTo(t *testing.T) {
	m := reviewBoard(t, pipeline.OnReview)
	sp := m.groups[pipeline.OnReview][0]
	if m.whyNotPlace(sp) == "" {
		t.Fatal("a review card can be moved by hand")
	}
	m.col = 0
	for i, st := range m.board.Statuses {
		if st.ID == pipeline.OnReview {
			m.col, m.rowInCol = i, 0
		}
	}
	if m.requireWorktree() || !strings.Contains(m.status, "someone else's pull request") {
		t.Fatalf("status %q", m.status)
	}
	if cmd := m.openPipelinePR(sp); cmd == nil || m.status != "opening #300" {
		t.Fatalf("the review button does not open the pull request: %q", m.status)
	}
}

func TestAFailedReadKeepsTheLastReviewList(t *testing.T) {
	m := reviewBoard(t, pipeline.OnReview)
	m.applyReviews(reviewsMsg{err: errString("not logged in")})
	if len(m.rv.list) != 2 || !strings.Contains(m.status, "fake: not logged in") {
		t.Fatalf("list %d, status %q", len(m.rv.list), m.status)
	}
	if m.loadReviews(false) != nil {
		t.Fatal("asked again straight after a failure")
	}
}

func TestTheCopiedContextNamesTheReview(t *testing.T) {
	m := reviewBoard(t, pipeline.OnReview)
	got := m.reviewYank(m.groups[pipeline.OnReview][0])
	if got != "#300 by Bo · feature/XYZ-9-menu → main · https://git.example.com/pr/300" {
		t.Fatalf("yank %q", got)
	}
	if !strings.Contains(yankText(yankFacts{Key: "XYZ-9", Review: got}), "Review asked of you: #300 by Bo") {
		t.Fatal("the copied text has no review line")
	}
}
