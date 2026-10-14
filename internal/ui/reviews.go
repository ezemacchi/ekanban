package ui

import (
	"context"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/reviews"
)

// Review requests on the pipeline board: the pull requests waiting on your
// review, from whichever reviews.Source the board was given. A card whose
// ticket has one says so and asks for you; a request whose ticket has no card
// gets one of its own, in the configured column. The board knows nothing of
// the code host behind the Source.

// reviewPrefix keys a card that is a pull request to review. It cannot be a
// directory, so nothing mistakes it for one.
const reviewPrefix = "review:"

type reviewState struct {
	src    reviews.Source
	column string         // where review cards go; "" adds none
	keyRe  *regexp.Regexp // a ticket key in a title or branch

	list    []reviews.Request
	at      time.Time
	loading bool
	err     string
}

// SetReviews turns review requests on. src is nil when they are off; why
// says so on the status line when that is a misconfiguration rather than a
// choice. column is where a request with no card of its own goes, "" for
// nowhere; keyRe finds a request's ticket.
func (m *Model) SetReviews(src reviews.Source, column string, keyRe *regexp.Regexp, why string) {
	m.rv = reviewState{src: src, column: column, keyRe: keyRe}
	if why != "" {
		m.SetProblems([]string{why})
	}
}

func (m *Model) reviewsOn() bool { return m.pipelineOn() && m.rv.src != nil }

// reviewCardKey is the request a review card stands for.
func reviewCardKey(spaceKey string) (string, bool) {
	return strings.CutPrefix(spaceKey, reviewPrefix)
}

// noWorktree is true for a card with no folder behind it: a Jira ticket
// not started, or someone else's pull request to review.
func noWorktree(spaceKey string) bool {
	_, jiraOnly := jiraCardKey(spaceKey)
	_, review := reviewCardKey(spaceKey)
	return jiraOnly || review
}

type reviewsMsg struct {
	list []reviews.Request
	err  error
}

// loadReviews rereads the list, at most once per pipelineEvery unless
// forced.
func (m *Model) loadReviews(force bool) tea.Cmd {
	if !m.reviewsOn() || m.rv.loading || !force && time.Since(m.rv.at) < pipelineEvery {
		return nil
	}
	m.rv.loading = true
	src := m.rv.src
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		list, err := src.Awaiting(ctx)
		return reviewsMsg{list: list, err: err}
	}
}

// applyReviews takes the answer. A failed read keeps the last good list and
// says why on the status line.
func (m *Model) applyReviews(msg reviewsMsg) {
	m.rv.loading = false
	m.rv.at = time.Now()
	if msg.err != nil {
		m.rv.err = msg.err.Error()
		m.status = m.rv.src.Name() + ": " + m.rv.err
		m.rebuild()
		return
	}
	m.rv.err = ""
	m.rv.list = msg.list
	m.rebuild()
}

// reviewTicket is the ticket a request is about: the first key in its
// title, else in its branch; "" when neither names one.
func reviewTicket(r reviews.Request, keyRe *regexp.Regexp) string {
	if keyRe == nil {
		return ""
	}
	if k := keyRe.FindString(r.Title); k != "" {
		return k
	}
	return keyRe.FindString(r.Branch)
}

// matchReviews splits the requests into those that mark a card already on
// the board, by ticket, and those that need a card of their own. held says
// whether a ticket has a card. The first request for a ticket marks it.
func matchReviews(list []reviews.Request, held func(ticket string) bool, keyRe *regexp.Regexp) (marks map[string]reviews.Request, unmatched []reviews.Request) {
	marks = map[string]reviews.Request{}
	for _, r := range list {
		t := reviewTicket(r, keyRe)
		if t != "" && held(t) {
			if _, dup := marks[t]; !dup {
				marks[t] = r
			}
			continue
		}
		unmatched = append(unmatched, r)
	}
	return marks, unmatched
}

// addReviewCards puts a card on the board for every request whose ticket
// no card holds yet. It runs after the worktree and Jira cards are known.
func (m *Model) addReviewCards(spaces map[string]*space) {
	if !m.reviewsOn() || m.rv.column == "" {
		return
	}
	held := map[string]bool{}
	var folders []string
	for key := range spaces {
		if k, ok := jiraCardKey(key); ok {
			held[k] = true
		} else if info, ok := m.pipeInfo[key]; ok && info.Key != "" {
			held[info.Key] = true
		} else {
			folders = append(folders, strings.ToUpper(baseName(key)))
		}
	}
	isHeld := func(t string) bool {
		if held[t] {
			return true
		}
		for _, f := range folders {
			if strings.Contains(f, strings.ToUpper(t)) {
				return true
			}
		}
		return false
	}
	_, unmatched := matchReviews(m.rv.list, isHeld, m.rv.keyRe)
	for _, r := range unmatched {
		key := reviewPrefix + r.ID
		spaces[key] = &space{Key: key, Label: reviewName(r)}
	}
}

// review is the request with this id.
func (m *Model) review(id string) (reviews.Request, bool) {
	for _, r := range m.rv.list {
		if r.ID == id {
			return r, true
		}
	}
	return reviews.Request{}, false
}

// reviewFor is the request a card stands for, or the one that marks it.
func (m *Model) reviewFor(sp *space) (reviews.Request, bool) {
	if !m.reviewsOn() || sp == nil {
		return reviews.Request{}, false
	}
	if id, ok := reviewCardKey(sp.Key); ok {
		return m.review(id)
	}
	t := m.ticketOf(sp)
	if t == "" {
		return reviews.Request{}, false
	}
	for _, r := range m.rv.list {
		if reviewTicket(r, m.rv.keyRe) == t {
			return r, true
		}
	}
	return reviews.Request{}, false
}

// reviewStage is a review card's column, and whether it shows at all.
func (m *Model) reviewStage(id string) (string, bool) {
	if _, ok := m.review(id); !ok || m.rv.column == "" {
		return "", false
	}
	if _, known := m.columns.Find(m.rv.column); !known {
		return pipeline.OnReview, true
	}
	return m.rv.column, true
}

// reviewName is how a request is named: #212 for a number, else the host's
// own identifier (!7 on GitLab).
func reviewName(r reviews.Request) string {
	if r.ID != "" && strings.Trim(r.ID, "0123456789") == "" {
		return "#" + r.ID
	}
	return r.ID
}

// reviewLabel names a review card: its ticket and request, "ABC-9 #212", or
// the request alone.
func (m *Model) reviewLabel(sp *space) string {
	r, _ := m.reviewFor(sp)
	name := reviewName(r)
	if t := m.ticketOf(sp); t != "" {
		return t + " " + name
	}
	return firstNonEmpty(name, sp.Label)
}

// reviewLink is the request's page, "" unless it is an https address.
func reviewLink(r reviews.Request) string {
	if strings.HasPrefix(r.URL, "https://") {
		return r.URL
	}
	return ""
}

// reviewFact is the request on a card: it asks for you.
func (m *Model) reviewFact(r reviews.Request) fact {
	text := "review " + reviewName(r)
	if r.Author != "" {
		text += " by " + r.Author
	}
	return fact{glyph: look.Eye, text: text, style: look.Attention, weight: weightYou, attention: true,
		url: reviewLink(r), kind: kindPR, short: said(look.Eye, "review "+reviewName(r), look.Attention)}
}

// reviewFacts are what a review card says: its ticket's Jira status when it
// has one, the request, and where it goes.
func (m *Model) reviewFacts(sp *space) []fact {
	r, ok := m.reviewFor(sp)
	if !ok {
		return nil
	}
	var out []fact
	if f, ok := m.jiraFact(m.ticketOf(sp)); ok {
		out = append(out, f)
	}
	out = append(out, m.reviewFact(r))
	if where := reviewWhere(r); where != "" {
		out = append(out, fact{glyph: look.Target, text: where, style: dimStyle, weight: weightContext, kind: kindPR})
	}
	return out
}

// reviewWhere is "feature/x → main", as much as is known.
func reviewWhere(r reviews.Request) string {
	switch {
	case r.Branch != "" && r.Target != "":
		return r.Branch + " → " + r.Target
	case r.Target != "":
		return "into " + r.Target
	}
	return r.Branch
}

// reviewDetailLines are the detail's lines about the request: where you
// stand and when it last changed.
func (m *Model) reviewDetailLines(sp *space, width int) []detailLine {
	r, ok := m.reviewFor(sp)
	if !ok {
		return nil
	}
	var parts []string
	if r.Status != "" {
		parts = append(parts, r.Status)
	}
	if !r.Updated.IsZero() {
		parts = append(parts, "changed "+humanAge(r.Updated))
	}
	if len(parts) == 0 {
		return nil
	}
	return plain(dimStyle.Render(truncate(strings.Join(parts, " · "), width)))
}

// openReview opens the request's page.
func (m *Model) openReview(r reviews.Request) tea.Cmd {
	url := reviewLink(r)
	if url == "" {
		m.status = m.rv.src.Name() + " gave no https address for " + reviewName(r)
		return nil
	}
	m.status = "opening " + reviewName(r)
	return openURLCmd(url)
}

// reviewYank is the request as one line for the copied context.
func (m *Model) reviewYank(sp *space) string {
	r, ok := m.reviewFor(sp)
	if !ok {
		return ""
	}
	return strings.Join(nonEmpty(reviewName(r)+" by "+firstNonEmpty(r.Author, "someone"), r.Status, reviewWhere(r), reviewLink(r)), " · ")
}
