package ui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/archive"
	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/orchestrator"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/store"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Pipeline mode: the board's columns are delivery stages computed from each
// ticket's run, the build server and git (package pipeline), not statuses set by hand.
// It is on when the board is scoped to a repository. Notes, names and manual
// order still belong to the user; the column does not.

const pipelineEvery = 60 * time.Second

// SetProblems shows configuration complaints (an unknown rule name) on the
// status line, so a typo in config.toml is not silently ignored.
func (m *Model) SetProblems(problems []string) {
	if len(problems) == 0 {
		return
	}
	if m.status != "" {
		problems = append([]string{m.status}, problems...)
	}
	m.status = strings.Join(uniq(problems), " · ")
}

// SetPipeline turns pipeline mode on with src as its source.
func (m *Model) SetPipeline(src *pipeline.Source) {
	m.pipe = src
	m.pipeInfo = map[string]pipeline.Info{}
	m.spinner = look.NewSpinner()
	m.manualStatuses = m.board.Statuses
	m.manualDefault = m.board.Default
	m.columns = src.Columns()
	m.board.Statuses = nil
	for _, c := range m.columns {
		m.board.Statuses = append(m.board.Statuses, store.Status{ID: c.ID, Label: c.Label, Color: c.Color})
	}
	m.board.Default = pipeline.ToDo
	// Pull requests come from the build server here; the GitHub client stays off.
	m.gh = nil
	if m.layout == layoutList {
		m.layout = layoutKanban
	}
}

func (m *Model) pipelineOn() bool { return m.pipe != nil }

// closedWorktrees are the repository's checkouts with no workspace open, in
// pipeline mode: a ticket does not leave the board because its workspace was
// closed. Herdr's worktree list (branches) knows every checkout; live is the
// set already on the board.
func (m *Model) closedWorktrees(open map[string]bool) []string {
	if m.pipe == nil {
		return nil
	}
	var out []string
	for key := range m.branches {
		if open[key] || !m.inScope(key) {
			continue
		}
		if _, err := os.Stat(key); err != nil {
			continue // pruned since Herdr listed it
		}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

type pipelineMsg struct {
	infos map[string]pipeline.Info
	at    time.Time
	panes []string // agent panes, for the status subscription
}

// loadPipeline classifies every space in scope in the background. The build
// server and git are refreshed at most once per pipelineEvery; the run files and agents
// are reread every time, so a role finishing shows within one tick.
func (m *Model) loadPipeline(force bool) tea.Cmd {
	if m.pipe == nil {
		return nil
	}
	if m.pipeLoading {
		// Something changed while a load runs: load once more after it,
		// rather than dropping the change.
		m.pipeAgain = true
		m.pipeForce = m.pipeForce || force
		return nil
	}
	refresh := force || time.Since(m.pipeAt) >= pipelineEvery
	keys := map[string]int{}
	open := map[string]bool{}
	for _, ws := range m.live {
		key := store.Key(ws.Cwd)
		if key != "" && m.inScope(key) {
			keys[key] = m.board.Entries[key].PR
			open[key] = true
		}
	}
	// A closed worktree has no agents to react to: it is classified on the
	// periodic refresh, or the first time it is seen.
	for _, key := range m.closedWorktrees(open) {
		if _, seen := m.pipeInfo[key]; refresh || !seen {
			keys[key] = m.board.Entries[key].PR
		}
	}
	m.pipeLoading = true
	src, client := m.pipe, m.client
	return tea.Batch(m.spinner.Start(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if refresh {
			src.Refresh(ctx)
		}
		live, _ := ticket.ReadLive(client)
		infos := map[string]pipeline.Info{}
		for key, pr := range keys {
			infos[key] = src.Classify(ctx, key, live, pr)
		}
		at := time.Time{}
		if refresh {
			at = time.Now()
		}
		return pipelineMsg{infos: infos, at: at, panes: live.AgentPanes()}
	})
}

// pipelineBusy is when the spinner turns: a load is running, or nothing has
// been read yet.
func (m *Model) pipelineBusy() bool {
	return m.pipelineOn() && (m.pipeLoading || m.pipeAt.IsZero())
}

func (m *Model) applyPipeline(msg pipelineMsg) tea.Cmd {
	m.pipeLoading = false
	if !msg.at.IsZero() {
		m.pipeAt = msg.at
	}
	changed := false
	if m.agentSeen == nil {
		m.agentSeen = map[string]map[string]string{}
	}
	var news []string
	for key, info := range msg.infos {
		m.pipeInfo[key] = info
		seen, moved := ticket.Changes(m.agentSeen[key], info.Agents)
		m.agentSeen[key] = seen
		for _, c := range moved {
			news = append(news, orKey(info.Key, key)+" · "+c.Says())
		}
		if info.PR > 0 && m.board.Entries[key].PR != info.PR {
			m.board.SetPR(key, info.PR)
			changed = true
		}
		if m.settlePlacement(key, info) {
			changed = true
			news = append(news, orKey(info.Key, key)+" → "+m.statusLabelOf(info.Stage))
		}
	}
	if changed {
		m.save()
	}
	if len(news) > 0 {
		sort.Strings(news)
		m.status = time.Now().Format("15:04") + "  " + strings.Join(news, "; ")
	}
	m.rebuild()
	cmds := []tea.Cmd{m.subscribe(msg.panes)}
	if m.pipeAgain {
		force := m.pipeForce
		m.pipeAgain, m.pipeForce = false, false
		cmds = append(cmds, m.loadPipeline(force))
	}
	return tea.Batch(cmds...)
}

// orKey is the ticket key, or the worktree's folder when it has none.
func orKey(ticketKey, space string) string {
	if ticketKey != "" {
		return ticketKey
	}
	return baseName(space)
}

// pipelineStage is a space's column, and whether it belongs on the board.
// With [stack] column set, a card whose stack says it is not finished goes
// back to that column (stack.go).
func (m *Model) pipelineStage(sp *space) (string, bool) {
	stage, ok := m.computedStage(sp)
	if ok && m.stk.On() {
		if col := m.stackColumn(sp); col != "" {
			return col, true
		}
	}
	return stage, ok
}

// computedStage is a space's column before any stack moves it.
func (m *Model) computedStage(sp *space) (string, bool) {
	if k, ok := jiraCardKey(sp.Key); ok {
		return m.jiraStage(k)
	}
	if id, ok := reviewCardKey(sp.Key); ok {
		return m.reviewStage(id)
	}
	if e, ok := m.board.Entries[sp.Key]; ok && e.Accepted != nil {
		return "", false
	}
	info, ok := m.pipeInfo[sp.Key]
	if !sp.Live && (!ok || info.Key == "") {
		// A closed worktree shows once it is known to hold a ticket.
		return "", false
	}
	if !ok {
		// Not classified yet: show it rather than flicker it in later.
		return pipeline.ToDo, true
	}
	if info.Key == "" {
		return "", false
	}
	if p, ok := m.placed(sp.Key); ok {
		return p.Stage, true
	}
	return info.Stage, true
}

// settlePlacement drops a card's placement by hand when the board has caught
// up with it: at once when the computed column is the one chosen, or when a
// different column is computed twice in a row. A single different read is
// held back, since a git command that failed or a run file caught mid-write
// also changes the column. It reports whether the placement was dropped.
func (m *Model) settlePlacement(key string, info pipeline.Info) bool {
	p := m.board.Entries[key].Placed
	if p == nil || info.Key == "" {
		delete(m.placeOff, key)
		return false
	}
	if m.placeOff == nil {
		m.placeOff = map[string]string{}
	}
	switch {
	case info.Stage == p.Over:
		delete(m.placeOff, key)
		return false
	case info.Stage != p.Stage && m.placeOff[key] != info.Stage:
		m.placeOff[key] = info.Stage
		return false
	}
	delete(m.placeOff, key)
	m.board.Unplace(key)
	return true
}

// placed is the column a card was moved to by hand. Only settlePlacement
// drops it, so it holds through a read that disagrees once.
func (m *Model) placed(key string) (store.Placed, bool) {
	p := m.board.Entries[key].Placed
	info, ok := m.pipeInfo[key]
	if p == nil || !ok || info.Key == "" || !m.pipe.HandMoves() {
		return store.Placed{}, false
	}
	return *p, true
}

// statusLabelOf is a column's label with its icon.
func (m *Model) statusLabelOf(id string) string {
	if st, ok := m.board.StatusByID(id); ok {
		return m.statusLabel(st)
	}
	return m.columns.Label(id)
}

// whyNotPlace is why sp cannot be moved by hand, "" when it can.
func (m *Model) whyNotPlace(sp *space) string {
	if !m.pipe.HandMoves() {
		return m.computedColumns()
	}
	if _, jiraOnly := jiraCardKey(sp.Key); jiraOnly {
		return "a ticket with no worktree follows its Jira status"
	}
	if _, review := reviewCardKey(sp.Key); review {
		return "a pull request to review stays where review requests go"
	}
	if info, ok := m.pipeInfo[sp.Key]; !ok || info.Key == "" {
		return "the board has not worked out this card's column yet"
	}
	return ""
}

// placeByHand puts a card in dest over its computed column. Moving it to the
// computed column drops the placement. It reports whether the card moved.
func (m *Model) placeByHand(sp *space, dest store.Status) bool {
	if why := m.whyNotPlace(sp); why != "" {
		m.status = why
		return false
	}
	info := m.pipeInfo[sp.Key]
	name := orKey(info.Key, sp.Key)
	delete(m.placeOff, sp.Key)
	if dest.ID == info.Stage {
		m.board.Unplace(sp.Key)
		m.status = name + " → " + m.statusLabel(dest)
		return true
	}
	m.board.Place(sp.Key, dest.ID, info.Stage)
	m.status = name + " → " + m.glyph(look.Hand, m.statusLabel(dest)) + m.press("follow-board", "to undo")
	return true
}

// followBoard drops the selected card's placement by hand.
func (m *Model) followBoard() {
	sp := m.selected()
	if sp == nil {
		m.status = "no card selected"
		return
	}
	if _, ok := m.placed(sp.Key); !ok {
		m.status = "not moved by hand"
		return
	}
	stage := m.pipeInfo[sp.Key].Stage
	if st, ok := m.board.StatusByID(stage); ok && m.placeByHand(sp, st) {
		m.save()
		m.rebuild()
		m.restoreColumnCursor(sp.Key)
	}
}

// computedColumns is the answer to a key the computed board does not take.
func (m *Model) computedColumns() string {
	return "columns are computed from the run, " + m.pipe.CIName() + " and git"
}

// handlePipelineKey takes the keys whose meaning changes in pipeline mode:
// editing the statuses is refused, since the columns are the board's; the
// picker, grab and 1-9 move a card by hand (see placeByHand) unless that is
// turned off; a accepts, A opens the archive, r also rereads the build server.
func (m *Model) handlePipelineKey(key string) (bool, tea.Model, tea.Cmd) {
	switch key {
	case "S":
		m.status = m.computedColumns()
		return true, m, nil
	case "s":
		if sp := m.selected(); sp != nil {
			if why := m.whyNotPlace(sp); why != "" {
				m.status = why
				return true, m, nil
			}
		}
		return false, m, nil // the picker, as on the plain board
	case "follow-board":
		m.followBoard()
		return true, m, nil
	case "a":
		model, cmd := m.acceptSelected()
		return true, model, cmd
	case "A":
		m.archiveView = true
		m.archiveIdx = 0
		m.archRuns = nil // a ticket accepted again has a new copy
		return true, m, nil
	case "close-ticket":
		return true, m, m.closeSelected()
	case "orchestrator":
		return true, m, m.goOrchestrator()
	case "handoff":
		return true, m, m.startHandoff()
	case "questions":
		return true, m, m.openQuestions()
	case "B":
		return true, m, m.startBroadcast()
	case "U":
		return true, m, m.syncNow()
	case "open-pull-request":
		if sp := m.selected(); sp != nil {
			return true, m, m.openPipelinePR(sp)
		}
		return true, m, nil
	case "open-issue":
		return true, m, m.openIssue()
	case "prototype":
		return true, m, m.openPrototype()
	case "r":
		m.branchesAt = time.Time{}
		return true, m, tea.Batch(m.refresh(), m.loadBranches(), m.loadPipeline(true), m.loadJira(true), m.loadBitbucket(true), m.loadReviews(true))
	}
	return false, m, nil
}

// openPipelinePR opens the pull request the board found for a card, from
// pipeline.pr_url. The pipeline board has no GitHub cache to read it from.
func (m *Model) openPipelinePR(sp *space) tea.Cmd {
	info := m.pipeInfo[sp.Key]
	if info.PR == 0 {
		if r, ok := m.reviewFor(sp); ok {
			return m.openReview(r)
		}
		m.status = "no pull request for " + m.spaceName(sp) + " yet"
		return nil
	}
	url := links.PullRequest(info.PR)
	if url == "" {
		m.status = "pipeline.pr_url is missing from config.toml"
		return nil
	}
	m.status = fmt.Sprintf("opening PR #%d", info.PR)
	return openURLCmd(url)
}

// openIssue opens the selected card's Jira ticket, from issue_url. It works on a
// card with no worktree too: the ticket is all such a card has.
func (m *Model) openIssue() tea.Cmd {
	sp := m.selected()
	if sp == nil {
		m.status = "no space selected"
		return nil
	}
	key := m.ticketOf(sp)
	if key == "" {
		m.status = m.spaceName(sp) + " has no ticket run"
		return nil
	}
	url := links.Issue(key)
	if url == "" {
		m.status = "issue_url is missing from config.toml"
		return nil
	}
	m.status = "opening " + key
	return openURLCmd(url)
}

type prototypeMsg struct {
	page string
	spec string
	err  error
}

// openPrototype opens the selected ticket's HTML prototype, the one the run's
// spec names. The board holds no run, so it is read the way the orchestrator
// key reads it: from the worktree, off the UI thread.
func (m *Model) openPrototype() tea.Cmd {
	sp := m.selected()
	if sp == nil || !m.requireWorktree() {
		if sp == nil {
			m.status = "no space selected"
		}
		return nil
	}
	if m.pipeInfo[sp.Key].Key == "" {
		m.status = m.spaceName(sp) + " has no ticket run"
		return nil
	}
	m.status = "looking for the prototype of " + m.spaceName(sp) + "…"
	src, client, worktree := m.pipe, m.client, sp.Key
	return func() tea.Msg {
		live, err := ticket.ReadLive(client)
		if err != nil {
			return prototypeMsg{err: err}
		}
		run, err := src.Run(worktree, live)
		if err != nil {
			return prototypeMsg{err: fmt.Errorf("no run found in %s", worktree)}
		}
		return prototypeMsg{page: run.Prototype, spec: run.Spec}
	}
}

// applyPrototype opens what openPrototype found, or says why there is none.
func (m *Model) applyPrototype(msg prototypeMsg) tea.Cmd {
	switch {
	case msg.err != nil:
		m.status = msg.err.Error()
	case msg.page == "":
		m.status = "no prototype found for " + firstNonEmpty(msg.spec, "this ticket")
	default:
		m.status = "opening the prototype"
		return openURLCmd(msg.page)
	}
	return nil
}

type orchestratorMsg struct {
	text string
	err  error
}

// goOrchestrator goes to the selected ticket's orchestrator, or opens a new one in its
// workspace when none is open.
func (m *Model) goOrchestrator() tea.Cmd {
	sp := m.selected()
	if sp == nil || !m.requireWorktree() {
		return nil
	}
	if m.pipeInfo[sp.Key].Key == "" {
		m.status = sp.Label + " has no ticket run"
		return nil
	}
	start := m.working("looking for the orchestrator of " + m.spaceName(sp) + "…")
	src, client, worktree := m.pipe, m.client, sp.Key
	return tea.Batch(start, func() tea.Msg {
		live, err := ticket.ReadLive(client)
		if err != nil {
			return orchestratorMsg{err: err}
		}
		run, err := src.Run(worktree, live)
		if err != nil {
			return orchestratorMsg{err: fmt.Errorf("no run found in %s", worktree)}
		}
		text, err := orchestrator.Go(client, run)
		return orchestratorMsg{text: text, err: err}
	})
}

// acceptSelected archives a ticket in the ready_qa column.
func (m *Model) acceptSelected() (tea.Model, tea.Cmd) {
	sp := m.selected()
	if sp == nil || !m.requireWorktree() {
		return m, nil
	}
	info := m.pipeInfo[sp.Key]
	// The column on screen, which may be one the card was moved to by hand.
	if !m.acceptable(sp.StatusID) {
		m.status = "only tickets in " + m.acceptColumns() + " can be accepted"
		return m, nil
	}
	rec := store.Accepted{
		At:       time.Now().UTC(),
		Ticket:   info.Key,
		Title:    firstNonEmpty(info.Title, sp.Label),
		Branch:   m.branchFor(sp.Key),
		PR:       info.PR,
		MergedTo: info.MergedTo,
		Deployed: info.Deployed,
		Worktree: sp.Key,
		Repo:     m.scope,
	}
	notes := ""
	if info.RunDir != "" {
		// A run's notes are a few files of text: copied here, they are in
		// the Archive before anything can remove the worktree.
		dest := archive.Path(m.stateDir, info.Key)
		if _, err := archive.Snapshot(sp.Key, info.RunDir, dest); err == nil {
			rec.Archive = dest
		} else if _, old := archive.ReadMeta(dest); old == nil {
			rec.Archive = dest
			notes = ", with the notes as last copied (" + err.Error() + ")"
		} else {
			notes = ", but its notes were not copied: " + err.Error()
		}
	}
	m.board.Accept(sp.Key, rec)
	m.save()
	m.status = info.Key + " accepted: it is in the Archive (A)" + notes
	m.rebuild()
	return m, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type archived struct {
	key string
	rec store.Accepted
}

// archive lists accepted tickets, newest first, narrowed by the filter.
func (m *Model) archive() []archived {
	var out []archived
	needle := strings.ToLower(m.filter)
	for key, e := range m.board.Entries {
		if e.Accepted == nil || e.Accepted.Repo != m.scope {
			continue
		}
		a := *e.Accepted
		hay := strings.ToLower(strings.Join([]string{a.Ticket, a.Title, a.Branch, fmt.Sprint(a.PR)}, " "))
		if needle != "" && !strings.Contains(hay, needle) {
			continue
		}
		out = append(out, archived{key: key, rec: a})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rec.At.After(out[j].rec.At) })
	return out
}

func (m *Model) handleArchiveKey(key string) (tea.Model, tea.Cmd) {
	items := m.archive()
	switch key {
	case "q", "esc", "A":
		if key == "esc" && m.filter != "" {
			m.filter = ""
			return m, nil
		}
		m.archiveView = false
		m.rebuild()
	case "j", "down":
		m.archiveIdx++
	case "k", "up":
		m.archiveIdx--
	case "g":
		m.archiveIdx = 0
	case "G":
		m.archiveIdx = len(items) - 1
	case "/":
		m.startSearch()
	case "o", "enter":
		if a, ok := m.archivedAt(items); ok {
			if url := links.Issue(a.rec.Ticket); url != "" {
				return m, openURLCmd(url)
			}
			m.status = "issue_url is missing from config.toml"
		}
	case "p":
		if a, ok := m.archivedAt(items); ok {
			if url := links.PullRequest(a.rec.PR); url != "" {
				return m, openURLCmd(url)
			}
			m.status = "no pull request, or pipeline.pr_url is missing from config.toml"
		}
	case "u":
		if a, ok := m.archivedAt(items); ok {
			m.board.Unaccept(a.key)
			m.save()
			m.status = a.rec.Ticket + " is back on the board"
		}
	}
	m.archiveIdx = max(0, min(m.archiveIdx, len(m.archive())-1))
	return m, nil
}

func (m *Model) archivedAt(items []archived) (archived, bool) {
	if m.archiveIdx < 0 || m.archiveIdx >= len(items) {
		return archived{}, false
	}
	return items[m.archiveIdx], true
}

func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg { _ = openURL(url); return nil }
}

// viewArchive is the list of accepted tickets.
func (m *Model) viewArchive() string {
	items := m.archive()
	var b strings.Builder
	title := m.glyph(look.Archive, "Archive")
	head := " " + titleStyle.Render(title) + dimStyle.Render(fmt.Sprintf("  %d accepted", len(items)))
	if m.filter != "" {
		head += dimStyle.Render("  /" + m.filter)
	}
	b.WriteString(head + "\n\n")

	if len(items) == 0 {
		b.WriteString(screen.Say("  No accepted tickets yet"+pressWith(m.pipelineKey("accept"), "on a card in "+m.acceptColumns()+" on the board")+".", dimStyle, 0) + "\n")
	}
	width := max(m.width-4, 40)
	for i, a := range items {
		prefix := "   "
		if i == m.archiveIdx {
			prefix = cursorStyle.Render(" ❯ ")
		}
		pr := ""
		if a.rec.PR > 0 {
			pr = fmt.Sprintf("PR #%d", a.rec.PR)
		}
		line := fmt.Sprintf("%-10s %-10s %s", a.rec.Ticket, a.rec.At.Local().Format("2006-01-02"), a.rec.Title)
		b.WriteString(prefix + labelStyle.Render(truncate(line, width)) + "\n")
		facts := strings.Join(nonEmpty(pr, prefixed("in ", a.rec.MergedTo), prefixed("shipped to ", a.rec.Deployed), a.rec.Branch), " · ")
		if facts != "" {
			b.WriteString("     " + dimStyle.Render(truncate(facts, width-2)) + "\n")
		}
		if i == m.archiveIdx {
			for _, line := range m.archivedNotes(a.rec) {
				b.WriteString("     " + line + "\n")
			}
		}
	}
	k := m.hintKey
	hints := []hint{{Key: k("open-issue"), Label: "Ticket"}, {Key: k("open-pull-request"), Label: "PR"},
		{Key: k("restore"), Label: "Back to the board"}, {Key: k("filter"), Label: "Search"}, {Key: k("archive"), Label: "Close"}}
	b.WriteString("\n")
	footer := " " + m.buttons(0, 1, hints, m.width-2)
	m.placeFooter(linesIn(b.String()))
	b.WriteString(footer)
	return lipgloss.NewStyle().MaxHeight(max(m.height, 1)).Render(b.String())
}

type archivedRead struct {
	run  *ticket.Run
	meta archive.Meta
	err  error
}

// archivedRun reads the run as its copy in the Archive kept it, once per
// copy: a copy changes only when the ticket is accepted again.
func (m *Model) archivedRun(path string) archivedRead {
	if r, ok := m.archRuns[path]; ok {
		return r
	}
	var r archivedRead
	if r.meta, r.err = archive.ReadMeta(path); r.err == nil {
		r.run, r.err = m.pipe.Run(path, ticket.Live{})
	}
	if m.archRuns == nil {
		m.archRuns = map[string]archivedRead{}
	}
	m.archRuns[path] = r
	return r
}

// archivedNotes are the lines under the selected ticket in the Archive: what
// its run said when it was copied.
func (m *Model) archivedNotes(a store.Accepted) []string {
	width := max(m.width-8, 30)
	if a.Archive == "" || m.pipe == nil {
		return []string{dimStyle.Render("no copy of the run's notes")}
	}
	r := m.archivedRun(a.Archive)
	if r.err != nil {
		return []string{dimStyle.Render(truncate("the copy of the run's notes cannot be read: "+r.err.Error(), width))}
	}
	out := []string{dimStyle.Render(truncate("notes copied "+r.meta.At.Local().Format("2006-01-02 15:04")+" · "+a.Archive, width))}
	add := func(label string, lines []string, most int) {
		for i, line := range lines {
			if i == most {
				out = append(out, dimStyle.Render(fmt.Sprintf("  … %d more", len(lines)-most)))
				break
			}
			if i == 0 {
				line = label + line
			} else {
				line = strings.Repeat(" ", len(label)) + line
			}
			out = append(out, labelStyle.Render(truncate(line, width)))
		}
	}
	add("Objective: ", r.run.Objective, 2)
	add("Last step: ", r.run.CurrentStep, 2)
	var forYou, others []string
	for _, q := range r.run.ForYou {
		forYou = append(forYou, q.Words)
	}
	for _, q := range r.run.Questions {
		others = append(others, firstNonEmpty(q.To, "?")+": "+q.Words)
	}
	add("Left for you: ", forYou, 2)
	add("Left for others: ", others, 2)
	if len(r.meta.Skipped) > 0 {
		out = append(out, dimStyle.Render(fmt.Sprintf("%d large file(s) were not copied", len(r.meta.Skipped))))
	}
	return out
}

func prefixed(p, v string) string {
	if v == "" {
		return ""
	}
	return p + v
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
