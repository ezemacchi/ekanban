package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/store"
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
	m.status = strings.Join(problems, " · ")
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

type pipelineMsg struct {
	infos map[string]pipeline.Info
	at    time.Time
}

// loadPipeline classifies every space in scope in the background. The build
// server and git are refreshed at most once per pipelineEvery; the run files and agents
// are reread every time, so a role finishing shows within one tick.
func (m *Model) loadPipeline(force bool) tea.Cmd {
	if m.pipe == nil || m.pipeLoading {
		return nil
	}
	refresh := force || time.Since(m.pipeAt) >= pipelineEvery
	keys := map[string]int{}
	for _, ws := range m.live {
		key := store.Key(ws.Cwd)
		if key != "" && m.inScope(key) {
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
		agents, _ := client.Agents()
		infos := map[string]pipeline.Info{}
		for key, pr := range keys {
			infos[key] = src.Classify(ctx, key, agents, pr)
		}
		at := time.Time{}
		if refresh {
			at = time.Now()
		}
		return pipelineMsg{infos: infos, at: at}
	})
}

// pipelineBusy is when the spinner turns: a load is running, or nothing has
// been read yet.
func (m *Model) pipelineBusy() bool {
	return m.pipelineOn() && (m.pipeLoading || m.pipeAt.IsZero())
}

func (m *Model) applyPipeline(msg pipelineMsg) {
	m.pipeLoading = false
	if !msg.at.IsZero() {
		m.pipeAt = msg.at
	}
	changed := false
	for key, info := range msg.infos {
		m.pipeInfo[key] = info
		if info.PR > 0 && m.board.Entries[key].PR != info.PR {
			m.board.SetPR(key, info.PR)
			changed = true
		}
	}
	if changed {
		m.save()
	}
	m.rebuild()
}

// pipelineStage is a space's column, and whether it belongs on the board.
func (m *Model) pipelineStage(sp *space) (string, bool) {
	if e, ok := m.board.Entries[sp.Key]; ok && e.Accepted != nil {
		return "", false
	}
	if !sp.Live {
		return "", false
	}
	info, ok := m.pipeInfo[sp.Key]
	if !ok {
		// Not classified yet: show it rather than flicker it in later.
		return pipeline.ToDo, true
	}
	if info.Key == "" {
		return "", false
	}
	return info.Stage, true
}

// pipelineLines are the facts under a card's name.
func (m *Model) pipelineLines(sp *space, width int) []string {
	info, ok := m.pipeInfo[sp.Key]
	if !ok {
		return []string{dimStyle.Render(truncate(m.spinner.Frame()+" working it out", width))}
	}
	var lines []string
	if info.Waiting {
		lines = append(lines, prPendingStyle.Render(truncate(m.glyph(look.Question, "asking you something"), width)))
	}
	if info.Lost {
		lines = append(lines, prPendingStyle.Render(truncate(m.glyph(look.Warning, "agents were lost"), width)))
	}
	if info.Phase != "" && info.Stage == pipeline.InProgress {
		lines = append(lines, dimStyle.Render(truncate(m.glyph(look.Team, info.Phase), width)))
	}
	if info.PR > 0 {
		pr := fmt.Sprintf("PR #%d", info.PR)
		style := dimStyle
		switch info.Build {
		case "SUCCESS":
			pr += " · " + m.pipe.CIName() + " green"
			style = prPassStyle
		case "FAILURE", "UNSTABLE":
			pr += " · " + m.pipe.CIName() + " " + strings.ToLower(info.Build)
			style = prFailStyle
		case "RUNNING":
			pr += " · " + m.pipe.CIName() + " running"
			style = prPendingStyle
		}
		if info.MergedTo != "" {
			pr += " · in " + info.MergedTo
		}
		lines = append(lines, style.Render(truncate(m.glyph(look.PullReq, pr), width)))
	}
	if info.Deployed != "" {
		lines = append(lines, prPassStyle.Render(truncate(m.glyph(look.Rocket, "shipped to "+info.Deployed), width)))
	}
	if info.Unknown != "" {
		lines = append(lines, dimStyle.Render(truncate(m.glyph(look.Wifi, info.Unknown), width)))
	}
	return lines
}

// computedColumns is the answer to a key that would move a card by hand.
func (m *Model) computedColumns() string {
	return "columns are computed from the run, " + m.pipe.CIName() + " and git"
}

// handlePipelineKey takes the keys whose meaning changes in pipeline mode:
// everything that would set a column by hand is refused, a accepts, A opens
// the archive, r also rereads the build server.
func (m *Model) handlePipelineKey(key string) (bool, tea.Model, tea.Cmd) {
	switch key {
	case "s", "S":
		m.status = m.computedColumns()
		return true, m, nil
	case "h", "left", "l", "right":
		if m.grabbed != "" {
			m.status = m.computedColumns()
			return true, m, nil
		}
	case "a":
		model, cmd := m.acceptSelected()
		return true, model, cmd
	case "A":
		m.archiveView = true
		m.archiveIdx = 0
		return true, m, nil
	case "r":
		m.branchesAt = time.Time{}
		return true, m, tea.Batch(m.refresh(), m.loadBranches(), m.loadPipeline(true))
	}
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		m.status = m.computedColumns()
		return true, m, nil
	}
	return false, m, nil
}

// acceptSelected archives a ticket in the ready_qa column.
func (m *Model) acceptSelected() (tea.Model, tea.Cmd) {
	sp := m.selected()
	if sp == nil {
		return m, nil
	}
	info := m.pipeInfo[sp.Key]
	if info.Stage != pipeline.ReadyQA {
		m.status = "only tickets in " + m.columns.Label(pipeline.ReadyQA) + " can be accepted"
		return m, nil
	}
	m.board.Accept(sp.Key, store.Accepted{
		At:       time.Now().UTC(),
		Ticket:   info.Key,
		Title:    firstNonEmpty(info.Title, sp.Label),
		Branch:   m.branchFor(sp.Key),
		PR:       info.PR,
		MergedTo: info.MergedTo,
		Deployed: info.Deployed,
		Worktree: sp.Key,
		Repo:     m.scope,
	})
	m.save()
	m.status = info.Key + " accepted: it is in the Archive (A)"
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
		m.mode = modeFilter
		m.input.SetValue(m.filter)
		m.input.CursorEnd()
		m.input.Focus()
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
		b.WriteString(dimStyle.Render("  No accepted tickets yet. On the board, press a on a card in "+m.columns.Label(pipeline.ReadyQA)+".") + "\n")
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
	}
	k := m.hintKey
	b.WriteString("\n" + dimStyle.Render(" "+k("down")+"/"+k("up")+" move · "+k("open-issue")+" tracker · "+k("open-pull-request")+
		" pull request · "+k("restore")+" back to the board · "+k("filter")+" search · "+k("archive")+" back"))
	return lipgloss.NewStyle().MaxHeight(max(m.height, 1)).Render(b.String())
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
