package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/jira"
	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Jira on the pipeline board: every ticket of [jira] jql that has no worktree
// is a card of its own, placed by its Jira status; a worktree's card shows
// its ticket's Jira status. The detail adds the linked tests and the latest
// comment, which is where QA writes what failed. H hands a ticket without a
// worktree off to the configured command; y copies a card's context.

// jiraPrefix keys a card that is a Jira ticket with no worktree. It cannot be
// a directory, so nothing mistakes it for one.
const jiraPrefix = "jira:"

// detailEvery is how long a ticket's detail is trusted before it is reread.
const detailEvery = 2 * time.Minute

type jiraState struct {
	client  *jira.Client
	cfg     jira.Config
	targets []string
	repo    string // the board's checkout, for {repo} and the command's directory
	hide    func(*exec.Cmd)

	issues  map[string]jira.Issue
	at      time.Time
	loading bool
	err     string

	detail   map[string]jira.Detail
	detailAt map[string]time.Time
	fetching map[string]bool

	handoff handoffState
}

// SetJira turns the Jira cards on. client is nil when Jira is off; why says
// so on the status line when that is a misconfiguration rather than a choice.
func (m *Model) SetJira(client *jira.Client, cfg jira.Config, targets []string, repo, why string, hide func(*exec.Cmd)) {
	if hide == nil {
		hide = func(*exec.Cmd) {}
	}
	// A handoff makes its worktree from the main checkout, wherever the
	// board was opened: the folder holding the shared .git.
	if filepath.Base(m.scope) == ".git" {
		repo = filepath.Dir(m.scope)
	}
	m.jr = jiraState{client: client, cfg: cfg, targets: targets, repo: repo, hide: hide,
		issues: map[string]jira.Issue{}, detail: map[string]jira.Detail{}, detailAt: map[string]time.Time{}, fetching: map[string]bool{}}
	if why != "" {
		m.SetProblems([]string{why})
	}
}

func (m *Model) jiraOn() bool { return m.pipelineOn() && m.jr.client != nil }

// jiraCardKey is the ticket a Jira-only card stands for.
func jiraCardKey(spaceKey string) (string, bool) {
	if k, ok := strings.CutPrefix(spaceKey, jiraPrefix); ok {
		return k, true
	}
	return "", false
}

// requireWorktree is false, with the reason on the status line, when the
// selected card is a Jira ticket that has no worktree: there is no folder,
// workspace or agent for what the caller would do.
func (m *Model) requireWorktree() bool {
	sp := m.selected()
	if sp == nil {
		return true // the caller says what is missing
	}
	if k, ok := jiraCardKey(sp.Key); ok {
		m.status = k + " has no worktree yet" + m.press("handoff", "to start it")
		return false
	}
	return true
}

// summaryOf is the Jira summary of a card's ticket, "" when there is none.
func (m *Model) summaryOf(sp *space) string {
	return m.jr.issues[m.ticketOf(sp)].Summary
}

// ticketOf is the ticket a card is about: its own for a Jira card, its run's
// or branch's for a worktree.
func (m *Model) ticketOf(sp *space) string {
	if sp == nil {
		return ""
	}
	if k, ok := jiraCardKey(sp.Key); ok {
		return k
	}
	return m.pipeInfo[sp.Key].Key
}

type jiraMsg struct {
	issues []jira.Issue
	err    error
}

// loadJira rereads the ticket list, at most once per pipelineEvery unless
// forced. The worktrees' tickets are asked for by key as well, so their cards
// show a status even when the query would leave them out.
func (m *Model) loadJira(force bool) tea.Cmd {
	if !m.jiraOn() || m.jr.loading || !force && time.Since(m.jr.at) < pipelineEvery {
		return nil
	}
	var keys []string
	for _, info := range m.pipeInfo {
		if info.Key != "" {
			keys = append(keys, info.Key)
		}
	}
	sort.Strings(keys)
	m.jr.loading = true
	c, jql := m.jr.client, m.jr.cfg.JQL
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		issues, err := c.Search(ctx, jql, keys)
		return jiraMsg{issues: issues, err: err}
	}
}

func (m *Model) applyJira(msg jiraMsg) {
	m.jr.loading = false
	m.jr.at = time.Now()
	if msg.err != nil {
		m.jr.err = msg.err.Error()
		m.rebuild()
		return
	}
	m.jr.err = ""
	m.jr.issues = map[string]jira.Issue{}
	for _, i := range msg.issues {
		m.jr.issues[i.Key] = i
	}
	m.rebuild()
}

// addJiraCards puts a card on the board for every listed ticket no worktree
// holds yet.
func (m *Model) addJiraCards(spaces map[string]*space) {
	if !m.jiraOn() {
		return
	}
	held := map[string]bool{}
	for key := range spaces {
		if info, ok := m.pipeInfo[key]; ok && info.Key != "" {
			held[info.Key] = true
			continue
		}
		// Not classified yet: the folder of a ticket worktree names its key.
		folder := strings.ToUpper(baseName(key))
		for k := range m.jr.issues {
			if strings.Contains(folder, k) {
				held[k] = true
			}
		}
	}
	for k := range m.jr.issues {
		if !held[k] {
			spaces[jiraPrefix+k] = &space{Key: jiraPrefix + k, Label: k}
		}
	}
}

// jiraStage is a Jira card's column, and whether it shows at all.
func (m *Model) jiraStage(key string) (string, bool) {
	i, ok := m.jr.issues[key]
	if !ok {
		return "", false
	}
	col, show := m.jr.cfg.Column(i)
	if _, known := m.columns.Find(col); !known {
		col = pipeline.ToDo
	}
	return col, show
}

// jiraLines are a Jira card's facts: its status, and that it has no worktree.
func (m *Model) jiraLines(key string, width int) []string {
	lines := []string{}
	if i, ok := m.jr.issues[key]; ok {
		if i.Summary != "" {
			lines = append(lines, labelStyle.Render(truncate(i.Summary, width)))
		}
		lines = append(lines, m.jiraStatusLine(key, width))
	}
	// Short enough for a card: "no worktree — press H".
	text := "no worktree" + strings.TrimRight(pressWith(m.hintKey("handoff"), ""), " ")
	lines = append(lines, screen.Say(m.glyph(look.Pause, text), dimStyle, width))
	return lines
}

// jiraStatusLine is "Jira: In Implementation" for a ticket on the list, or ""
// when Jira is off or the ticket is not on it.
func (m *Model) jiraStatusLine(key string, width int) string {
	i, ok := m.jr.issues[key]
	if !m.jiraOn() || !ok {
		return ""
	}
	text := "Jira: " + i.Status
	if i.Type != "" {
		text += " · " + i.Type
	}
	return jiraStatusStyle(i.Category).Render(truncate(m.glyph(look.Jira, text), width))
}

func jiraStatusStyle(category string) lipgloss.Style {
	switch category {
	case "indeterminate":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	case "done":
		return prPassStyle
	}
	return dimStyle
}

type jiraDetailMsg struct {
	key    string
	detail jira.Detail
	err    error
}

// ensureDetail reads the selected card's ticket when the detail is on screen
// and what is held is missing or old.
func (m *Model) ensureDetail() tea.Cmd {
	if !m.jiraOn() {
		return nil
	}
	showing := m.mode == modeDetail || m.layout == layoutList && m.detailPaneWidth() > 0
	key := m.ticketOf(m.selected())
	if !showing || key == "" || m.jr.fetching[key] || time.Since(m.jr.detailAt[key]) < detailEvery {
		return nil
	}
	return m.fetchDetail(key)
}

func (m *Model) fetchDetail(key string) tea.Cmd {
	m.jr.fetching[key] = true
	c := m.jr.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		d, err := c.Detail(ctx, key)
		return jiraDetailMsg{key: key, detail: d, err: err}
	}
}

func (m *Model) applyDetail(msg jiraDetailMsg) {
	delete(m.jr.fetching, msg.key)
	m.jr.detailAt[msg.key] = time.Now()
	if msg.err != nil {
		m.status = msg.key + ": " + msg.err.Error()
		return
	}
	m.jr.detail[msg.key] = msg.detail
}

// detailLimit is how many lines of the latest comment the detail shows.
const detailLimit = 12

// jiraDetailLines are the detail's Jira section: the linked tests and the
// latest comment.
func (m *Model) jiraDetailLines(key string, width int) []detailLine {
	if !m.jiraOn() || key == "" {
		return nil
	}
	var lines []detailLine
	d, ok := m.jr.detail[key]
	if !ok {
		if m.jr.fetching[key] {
			lines = append(lines, plain(dimStyle.Render(truncate(m.spinner.Frame()+" reading "+key+" in Jira", width)))...)
		}
		return lines
	}
	if len(d.Tests) > 0 {
		lines = append(lines, plain(dimStyle.Render(truncate(m.glyph(look.Flask, fmt.Sprintf("%d linked tests", len(d.Tests))), width)))...)
		for _, t := range d.Tests {
			lines = append(lines, detailLine{text: dimStyle.Render(truncate("  "+t.Key+" "+t.Summary+" · "+t.Status, width)), url: links.Issue(t.Key)})
		}
	}
	if c, ok := d.Latest(); ok {
		head := fmt.Sprintf("latest comment · %s · %s", c.Author, humanAge(c.At))
		lines = append(lines, plain("", noteStyle.Render(truncate(m.glyph(look.Comment, head), width)))...)
		body := screen.Wrap(c.Body, width-2)
		for i, l := range body {
			if i == detailLimit {
				more := fmt.Sprintf("  … %d more lines", len(body)-detailLimit) + pressWith(m.hintKey("yank"), "to copy all of it")
				lines = append(lines, plain(screen.Say(more, dimStyle, width))...)
				break
			}
			lines = append(lines, plain("  "+l)...)
		}
	}
	return lines
}

// yank copies what another session needs to pick the card up: the ticket,
// its spec, where the work is, and what QA said.

type yankMsg struct {
	text string
	err  error
}

// clipboardWrite is replaced in tests.
var clipboardWrite = clipboard.WriteAll

// yankFacts is everything the copied text is built from.
type yankFacts struct {
	Key      string
	Issue    jira.Issue
	HasIssue bool
	Detail   *jira.Detail
	IssueURL string
	Column   string
	Info     pipeline.Info
	Run      *ticket.Run
	PRURL    string
	Worktree string
	Branch   string
	CI       string
	Merge    []string // Bitbucket's verdict, then each report; see bitbucketYank
	Note     string
}

// yankSelected gathers the card's facts, reading the run and the ticket's
// detail when they are not held, and copies the text.
func (m *Model) yankSelected() tea.Cmd {
	sp := m.selected()
	if sp == nil {
		m.status = "nothing selected to copy"
		return nil
	}
	key := m.ticketOf(sp)
	f := yankFacts{Key: key, Note: sp.Note, IssueURL: links.Issue(key)}
	if st, ok := m.board.StatusByID(sp.StatusID); ok {
		f.Column = st.Label
	}
	f.Issue, f.HasIssue = m.jr.issues[key]
	if d, ok := m.jr.detail[key]; ok {
		f.Detail = &d
	}
	_, jiraOnly := jiraCardKey(sp.Key)
	if !jiraOnly {
		f.Worktree, f.Branch = sp.Key, m.branchFor(sp.Key)
		f.Info = m.pipeInfo[sp.Key]
		if f.Info.PR > 0 {
			f.PRURL = links.PullRequest(f.Info.PR)
			f.Merge = m.bitbucketYank(f.Info)
		}
	}
	if m.pipelineOn() {
		f.CI = m.pipe.CIName()
	}
	if key == "" && f.Worktree == "" {
		m.status = "this card has nothing to copy"
		return nil
	}
	src, client, jc := m.pipe, m.client, m.jr.client
	if !m.jiraOn() {
		jc = nil
	}
	start := m.working("copying " + orKey(key, sp.Key) + "…")
	return tea.Batch(start, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if f.Worktree != "" && src != nil {
			if live, err := ticket.ReadLive(client); err == nil {
				if run, err := src.Run(f.Worktree, live); err == nil {
					f.Run = run
				}
			}
		}
		var why string
		if jc != nil && key != "" && f.Detail == nil {
			if d, err := jc.Detail(ctx, key); err == nil {
				f.Detail = &d
				if !f.HasIssue {
					f.Issue, f.HasIssue = d.Issue, true
				}
			} else {
				why = " (without Jira: " + err.Error() + ")"
			}
		}
		text := yankText(f)
		if err := clipboardWrite(text); err != nil {
			return yankMsg{err: fmt.Errorf("could not copy: %w", err)}
		}
		return yankMsg{text: fmt.Sprintf("copied %s: %d lines%s", orKey(key, f.Worktree), strings.Count(text, "\n")+1, why)}
	})
}

// yankText is the copied context, as plain lines another agent can read.
func yankText(f yankFacts) string {
	var b strings.Builder
	line := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(&b, "%s: %s\n", label, value)
		}
	}
	title := f.Key
	switch {
	case f.HasIssue && f.Issue.Summary != "":
		title += " · " + f.Issue.Summary
	case f.Info.Title != "":
		title += " · " + f.Info.Title
	}
	b.WriteString(strings.TrimPrefix(title, " · ") + "\n")
	if f.HasIssue {
		line("Jira", strings.Join(nonEmpty(f.IssueURL, f.Issue.Status, f.Issue.Type), " · "))
	} else {
		line("Jira", f.IssueURL)
	}
	line("Board column", strings.Join(nonEmpty(f.Column, f.Info.Phase), " · "))
	if r := f.Run; r != nil {
		line("Spec", r.Spec)
		line("Prototype", r.Prototype)
		line("Team", r.Team)
		line("Target", r.Target)
	}
	line("Worktree", f.Worktree)
	line("Branch", f.Branch)
	if r := f.Run; r != nil {
		line("Run state", r.StatePath())
	}
	if f.Info.PR > 0 {
		pr := fmt.Sprintf("#%d", f.Info.PR)
		if f.PRURL != "" {
			pr += " " + f.PRURL
		}
		if f.Info.Build != "" {
			pr += " · " + f.CI + " " + strings.ToLower(f.Info.Build)
		}
		if f.Info.MergedTo != "" {
			pr += " · merged into " + f.Info.MergedTo
		}
		line("Pull request", pr)
		if len(f.Merge) > 0 {
			line("Merge check", f.Merge[0])
			for _, extra := range f.Merge[1:] {
				line("Report", extra)
			}
		}
	}
	line("Shipped to", f.Info.Deployed)
	line("Note", f.Note)
	if r := f.Run; r != nil {
		section(&b, "Current step", r.CurrentStep)
		section(&b, "Open questions", r.Questions)
	}
	if d := f.Detail; d != nil {
		if len(d.Tests) > 0 {
			var tests []string
			for _, t := range d.Tests {
				tests = append(tests, "- "+t.Key+" "+t.Summary+" ["+t.Status+"]")
			}
			section(&b, fmt.Sprintf("Linked tests (%d)", len(d.Tests)), tests)
		}
		if c, ok := d.Latest(); ok {
			head := "Latest comment"
			if c.Author != "" {
				head += ", " + c.Author
			}
			if !c.At.IsZero() {
				head += ", " + c.At.Local().Format("2006-01-02 15:04")
			}
			section(&b, head, strings.Split(c.Body, "\n"))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func section(b *strings.Builder, head string, lines []string) {
	var kept []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, strings.TrimRight(l, " \r"))
		}
	}
	if len(kept) == 0 {
		return
	}
	b.WriteString("\n" + head + ":\n" + strings.Join(kept, "\n") + "\n")
}

// Handoff: H on a Jira card asks for the target branch and the team, shows
// what will run, and runs [jira.handoff] command on enter.

type handoffState struct {
	issue jira.Issue
	step  int // 0 target, 1 team, 2 confirm
	idx   int
	// target and team are the choices made so far.
	target, team string
	prev         mode
}

func (m *Model) startHandoff() tea.Cmd {
	sp := m.selected()
	if sp == nil {
		return nil
	}
	key := m.ticketOf(sp)
	if !m.jiraOn() {
		m.status = "set [jira] in config.toml to hand tickets off from the board"
		return nil
	}
	if _, jiraOnly := jiraCardKey(sp.Key); !jiraOnly {
		if key != "" {
			m.status = key + " already has a worktree" + m.press("jump", "to go there")
		}
		return nil
	}
	if len(m.jr.cfg.Handoff.Command) == 0 {
		m.status = "set [jira.handoff] command in config.toml to hand tickets off"
		return nil
	}
	issue, ok := m.jr.issues[key]
	if !ok {
		issue = jira.Issue{Key: key}
	}
	m.jr.handoff = handoffState{issue: issue, prev: m.mode}
	m.mode = modeHandoff
	m.handoffSkip()
	return nil
}

// handoffChoices are the current step's options.
func (m *Model) handoffChoices() []string {
	switch m.jr.handoff.step {
	case 0:
		return m.jr.targets
	case 1:
		return m.jr.cfg.Handoff.Teams
	}
	return nil
}

// handoffSkip passes over a step with nothing to choose, and chooses alone
// when there is a single option.
func (m *Model) handoffSkip() {
	h := &m.jr.handoff
	for h.step < 2 {
		choices := m.handoffChoices()
		if len(choices) > 1 {
			return
		}
		if len(choices) == 1 {
			m.handoffPick(choices[0])
			continue
		}
		h.step++
	}
}

func (m *Model) handoffPick(choice string) {
	h := &m.jr.handoff
	switch h.step {
	case 0:
		h.target = choice
	case 1:
		h.team = choice
	}
	h.step++
	h.idx = 0
}

func (m *Model) handleHandoffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	h := &m.jr.handoff
	choices := m.handoffChoices()
	switch key := msg.String(); key {
	case "esc", "q":
		m.mode = h.prev
		m.status = "handoff cancelled"
	case "j", "down", "tab":
		if len(choices) > 0 {
			h.idx = (h.idx + 1) % len(choices)
		}
	case "k", "up", "shift+tab":
		if len(choices) > 0 {
			h.idx = (h.idx - 1 + len(choices)) % len(choices)
		}
	case "enter", " ":
		if h.step >= 2 {
			m.mode = h.prev
			return m, m.runHandoff()
		}
		if h.idx < len(choices) {
			m.handoffPick(choices[h.idx])
			m.handoffSkip()
		}
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			if i := int(key[0] - '1'); i < len(choices) {
				m.handoffPick(choices[i])
				m.handoffSkip()
			}
		}
	}
	return m, nil
}

// handoffValues fill the command's placeholders.
func (m *Model) handoffValues() map[string]string {
	h := m.jr.handoff
	return map[string]string{
		"key": h.issue.Key, "summary": firstNonEmpty(h.issue.Summary, h.issue.Key),
		"target": h.target, "team": h.team, "type": m.jr.cfg.Handoff.Type(h.issue.Type), "repo": m.jr.repo,
	}
}

type handoffMsg struct {
	key  string
	text string
	ok   bool
}

// runHandoff runs the command without a shell, in the board's checkout. It
// is told it runs inside Herdr, which it does, when the plugin's environment
// does not say so.
func (m *Model) runHandoff() tea.Cmd {
	h := m.jr.handoff
	args := m.jr.cfg.Handoff.Args(m.handoffValues())
	repo, hide := m.jr.repo, m.jr.hide
	start := m.working("handing " + h.issue.Key + " off: " + strings.Join(nonEmpty(h.target, h.team), ", ") + "…")
	return tea.Batch(start, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = repo
		cmd.Env = handoffEnv(os.Environ())
		hide(cmd)
		out, err := cmd.CombinedOutput()
		text, ok := handoffResult(out, err)
		return handoffMsg{key: h.issue.Key, text: text, ok: ok}
	})
}

func handoffEnv(env []string) []string {
	has := func(name string) bool {
		for _, e := range env {
			if v, ok := strings.CutPrefix(e, name+"="); ok && v != "" {
				return true
			}
		}
		return false
	}
	if !has("HERDR_ENV") {
		env = append(env, "HERDR_ENV=1")
	}
	if !has("HERDR_BIN_PATH") {
		if p, err := exec.LookPath("herdr"); err == nil {
			env = append(env, "HERDR_BIN_PATH="+p)
		}
	}
	return env
}

// handoffResult reads the command's answer: the last line that is a JSON
// object with a message (and a code, 0 for done), else its last output line.
func handoffResult(out []byte, err error) (string, bool) {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var r struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
		}
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "{") && json.Unmarshal([]byte(l), &r) == nil && r.Message != "" {
			ok := err == nil && (r.Code == nil || *r.Code == 0)
			return r.Message, ok
		}
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if err != nil {
		if last == "" {
			return err.Error(), false
		}
		return last + " (" + err.Error() + ")", false
	}
	if last == "" {
		last = "done"
	}
	return last, true
}

func (m *Model) applyHandoff(msg handoffMsg) tea.Cmd {
	if msg.ok {
		m.status = msg.key + " handed off: " + msg.text
	} else {
		m.status = msg.key + " was not handed off: " + msg.text
	}
	m.branchesAt = time.Time{}
	return tea.Batch(m.refresh(), m.loadBranches(), m.loadPipeline(true), m.loadJira(true), m.loadBitbucket(true))
}

// viewHandoff is the handoff box over the board.
func (m *Model) viewHandoff(base string) string {
	h := m.jr.handoff
	inner := max(min(m.width-detailModalMargin, 80)-4, 30)
	title := titleStyle.Render(truncate("Hand off "+h.issue.Key, inner))
	var lines []string
	if h.issue.Summary != "" {
		lines = append(lines, truncate(h.issue.Summary, inner))
	}
	if h.issue.Status != "" {
		lines = append(lines, dimStyle.Render(truncate("Jira: "+h.issue.Status+" · "+m.jr.cfg.Handoff.Type(h.issue.Type), inner)))
	}
	lines = append(lines, "")
	for _, done := range [][2]string{{"target", h.target}, {"team", h.team}} {
		if done[1] != "" {
			lines = append(lines, dimStyle.Render(done[0]+": ")+done[1])
		}
	}
	var help string
	switch h.step {
	case 0, 1:
		ask := "Target branch"
		if h.step == 1 {
			ask = "Team"
		}
		lines = append(lines, labelStyle.Bold(true).Render(ask))
		for i, c := range m.handoffChoices() {
			prefix := "   "
			if i == h.idx {
				prefix = cursorStyle.Render(" ❯ ")
			}
			lines = append(lines, prefix+fmt.Sprintf("%d %s", i+1, c))
		}
		help = screen.Key("j") + "/" + screen.Key("k") + " move · " + screen.Key("enter") + " choose · " + screen.Key("esc") + " cancel"
	default:
		lines = append(lines, labelStyle.Bold(true).Render("Runs"))
		for _, l := range screen.Wrap(strings.Join(m.jr.cfg.Handoff.Args(m.handoffValues()), " "), inner-2) {
			lines = append(lines, dimStyle.Render("  "+l))
		}
		lines = append(lines, "", "It creates the branch and worktree and starts the work.")
		help = screen.Key("enter") + " hand it off · " + screen.Key("esc") + " cancel"
	}
	body := title + "\n\n" + strings.Join(lines, "\n") + "\n\n" + screen.Say(help, dimStyle, inner)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).Width(inner + 2).Render(body)
	x := max0((m.width - lipgloss.Width(box)) / 2)
	y := max0((m.height - lipgloss.Height(box)) / 2)
	return overlay(base, box, x, y, m.width, m.height)
}
