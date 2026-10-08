package ui

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/bitbucket"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
)

// Bitbucket on the pipeline board: a green build is not a pull request that
// may merge. Bitbucket also holds the reports other tools post on the commit
// (SonarQube's quality gate), the reviewers' verdicts and its own merge
// checks, so each card's pull request says whether it is fit to merge, still
// waiting, or blocked and by what. Read only.

type bitbucketState struct {
	client *bitbucket.Client
	loc    bitbucket.Location

	reviews map[int]bitbucket.Review
	errs    map[int]string
	at      map[int]time.Time // when each pull request was last asked about
	loading bool
}

// SetBitbucket turns the merge verdicts on. client is nil when Bitbucket is
// off; why says so on the status line when that is a misconfiguration rather
// than a choice.
func (m *Model) SetBitbucket(client *bitbucket.Client, loc bitbucket.Location, why string) {
	m.bb = bitbucketState{client: client, loc: loc,
		reviews: map[int]bitbucket.Review{}, errs: map[int]string{}, at: map[int]time.Time{}}
	if why != "" {
		m.SetProblems([]string{why})
	}
}

func (m *Model) bbOn() bool { return m.pipelineOn() && m.bb.client != nil }

type bitbucketMsg struct {
	reviews map[int]bitbucket.Review
	errs    map[int]error
}

// bbWanted are the pull requests worth asking about: those the board knows of
// that have not been merged.
func (m *Model) bbWanted() []int {
	seen := map[int]bool{}
	var out []int
	for _, info := range m.pipeInfo {
		if info.PR > 0 && info.MergedTo == "" && !seen[info.PR] {
			seen[info.PR] = true
			out = append(out, info.PR)
		}
	}
	sort.Ints(out)
	return out
}

// loadBitbucket rereads the pull requests that are due: each at most once per
// pipelineEvery unless forced.
func (m *Model) loadBitbucket(force bool) tea.Cmd {
	if !m.bbOn() || m.bb.loading {
		return nil
	}
	var due []int
	for _, n := range m.bbWanted() {
		if at, ok := m.bb.at[n]; force || !ok || time.Since(at) >= pipelineEvery {
			due = append(due, n)
		}
	}
	if len(due) == 0 {
		return nil
	}
	m.bb.loading = true
	c, loc := m.bb.client, m.bb.loc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		out := bitbucketMsg{reviews: map[int]bitbucket.Review{}, errs: map[int]error{}}
		var (
			mu  sync.Mutex
			wg  sync.WaitGroup
			sem = make(chan struct{}, 4)
		)
		for _, n := range due {
			wg.Add(1)
			sem <- struct{}{}
			go func(n int) {
				defer wg.Done()
				defer func() { <-sem }()
				r, err := c.Review(ctx, loc.Project, loc.Repo, n)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					out.errs[n] = err
					return
				}
				out.reviews[n] = r
			}(n)
		}
		wg.Wait()
		return out
	}
}

func (m *Model) applyBitbucket(msg bitbucketMsg) {
	m.bb.loading = false
	now := time.Now()
	for n, r := range msg.reviews {
		m.bb.reviews[n] = r
		delete(m.bb.errs, n)
		m.bb.at[n] = now
	}
	// A failed read is remembered too, so a server that is down is asked once
	// a minute rather than on every tick. The last good answer is kept.
	for n, err := range msg.errs {
		m.bb.errs[n] = err.Error()
		m.bb.at[n] = now
	}
}

// judge is Bitbucket's verdict on a pull request, and false when there is
// none to give. The build server's word that a build is running turns
// Bitbucket's refusal for unfinished builds into a wait.
func (m *Model) judge(info pipeline.Info) (bitbucket.Review, bitbucket.Judgement, bool) {
	if !m.bbOn() || info.PR <= 0 || info.MergedTo != "" {
		return bitbucket.Review{}, bitbucket.Judgement{}, false
	}
	r, ok := m.bb.reviews[info.PR]
	if !ok {
		return bitbucket.Review{}, bitbucket.Judgement{}, false
	}
	j := r.Judge(info.Build == "RUNNING")
	return r, j, j.Verdict != bitbucket.NoVerdict
}

// bitbucketDetailLines is the detail's section: every reason, each report
// with its own words and figures, and who said what.
func (m *Model) bitbucketDetailLines(info pipeline.Info, width int) []detailLine {
	r, j, ok := m.judge(info)
	if !ok {
		return nil
	}
	var lines []detailLine
	// Under a line, what explains it: dim and indented, cut to the width.
	explain := func(texts ...string) {
		for _, t := range texts {
			for _, l := range screen.Wrap(t, width-2) {
				lines = append(lines, plain(dimStyle.Render("  "+l))...)
			}
		}
	}
	for _, v := range r.Vetoes {
		lines = append(lines, plain(prFailStyle.Render(truncate("✗ "+nonBlank(v.Summary, "refused by Bitbucket"), width)))...)
		explain(bitbucket.Lines(v.Detail)...)
	}
	if r.Conflicted {
		lines = append(lines, plain(prFailStyle.Render(truncate("✗ has merge conflicts", width)))...)
	}
	// A report that passed is one line; one that failed, or has not
	// finished, says why and with what figures, since that is what you will
	// be asked about.
	for _, c := range r.Checks {
		style, mark, word := prPendingStyle, "◌", "not finished"
		switch c.Result {
		case "PASS":
			style, mark, word = prPassStyle, "✓", "passed"
		case "FAIL":
			style, mark, word = prFailStyle, "✗", "failed"
		}
		line := detailLine{text: style.Render(truncate(mark+" "+c.Title+" "+word, width))}
		if strings.HasPrefix(c.Link, "https://") {
			line.url = c.Link // the report's own page, SonarQube's dashboard for one
		}
		lines = append(lines, line)
		if c.Result == "PASS" {
			continue
		}
		explain(bitbucket.Lines(c.Details)...)
		if len(c.Metrics) > 0 {
			var parts []string
			for _, mt := range c.Metrics {
				parts = append(parts, mt.Title+" "+mt.Value)
			}
			explain(strings.Join(parts, " · "))
		}
	}
	if len(r.Reviewers) > 0 {
		var who []string
		for _, rv := range r.Reviewers {
			who = append(who, rv.Name+" "+reviewerWord(rv.Status))
		}
		for _, l := range screen.Wrap("reviewers: "+strings.Join(who, " · "), width) {
			lines = append(lines, plain(dimStyle.Render(l))...)
		}
	}
	if j.Verdict == bitbucket.Fit && len(r.Checks) == 0 && len(r.Vetoes) == 0 {
		lines = append(lines, plain(dimStyle.Render("no checks stand in the way"))...)
	}
	return lines
}

func reviewerWord(status string) string {
	switch status {
	case "APPROVED":
		return "approved"
	case "NEEDS_WORK":
		return "asked for changes"
	}
	return "has not answered"
}

func nonBlank(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// bitbucketYank is the verdict as plain lines for the copied context: the
// verdict with its reasons, then each report with its page.
func (m *Model) bitbucketYank(info pipeline.Info) []string {
	r, j, ok := m.judge(info)
	if !ok {
		return nil
	}
	word := map[bitbucket.Verdict]string{bitbucket.Fit: "ready to merge", bitbucket.Waiting: "waiting", bitbucket.Blocked: "blocked"}[j.Verdict]
	out := []string{word}
	if len(j.Why) > 0 {
		out[0] += " — " + strings.Join(j.Why, "; ")
	}
	for _, c := range r.Checks {
		line := c.Title + ": " + strings.ToLower(c.Result)
		if d := bitbucket.FirstLines(c.Details, 3); d != "" {
			line += " — " + d
		}
		if strings.HasPrefix(c.Link, "https://") {
			line += " " + c.Link
		}
		out = append(out, line)
	}
	return out
}
