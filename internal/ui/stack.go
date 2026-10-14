package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ezemacchi/ekanban/internal/jira"
	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/stack"
)

// Stacked cards: related tickets drawn under a card (internal/stack), walked
// with the keys and the mouse before the cursor leaves the card. This file is
// the board's whole side of it, and every entry point does nothing when
// [stack] names no source, so a board without it is the board as it was.

// scopeStack is the scope of the stack's actions, in force while the selected
// card has a stack.
const scopeStack = "stack"

// StackActions are the actions of a card with a stack. They are in the
// computed board's key map only while [stack] is on, so help, the buttons and
// `[keys]` name them only for someone using the feature.
var StackActions = []keys.Action{
	{Name: "stack-down", Keys: []string{"j", "down"}, Canon: "stack-down", Scope: scopeStack, Help: "on a card with a stack: the next related ticket; after the last, the next card", Short: "next related"},
	{Name: "stack-up", Keys: []string{"k", "up"}, Canon: "stack-up", Scope: scopeStack, Help: "on a card with a stack: the previous related ticket; above the card, the previous card", Short: "previous related"},
	{Name: "stack-open", Keys: []string{"t"}, Canon: "stack-open", Scope: scopeStack, Help: "on a card with a stack: open the related ticket under the cursor in the tracker (the card's own ticket when the cursor is on the card)", Short: "open related"},
}

// stackRoom is what the cache keeps for one card.
type stackRoom struct {
	items []stack.Item
	more  int // items past [stack] max_items, left out
}

// stackFetchBatch is how many tickets one pass reads ahead, and stackEvery
// how long a ticket read ahead is trusted: a stack is background information,
// so it asks Jira less often than an open detail does.
const (
	stackFetchBatch = 4
	stackEvery      = 5 * time.Minute
)

// SetStack turns stacked cards on, from [stack]; nothing is turned on when
// it names no source. It returns what it fell back from.
func (m *Model) SetStack(cfg stack.Config) []string {
	s, problems := stack.New(cfg)
	m.stk = s
	m.stackCache = nil
	// The actions join the computed board's key map, whichever of SetKeys
	// and this came first.
	if m.keys.pipeline != nil {
		km, p := keys.New(m.pipelineActions(), m.bindings)
		m.keys.pipeline = km
		problems = append(problems, p...)
	}
	return problems
}

// pipelineActions are the computed board's actions, with the stack's when it
// is on.
func (m *Model) pipelineActions() []keys.Action {
	if !m.stk.On() {
		return PipelineActions
	}
	return append(append([]keys.Action(nil), PipelineActions...), StackActions...)
}

// stackOf is sp's stack: the strips it draws, and how many more there were.
// Nothing on a board with the feature off, and nothing on a card that is not
// a ticket's.
func (m *Model) stackOf(sp *space) ([]stack.Item, int) {
	if sp == nil || !m.stk.On() || !m.pipelineOn() {
		return nil, 0
	}
	if r, ok := m.stackCache[sp.Key]; ok {
		return r.items, r.more
	}
	r := m.readStack(sp)
	if m.stackCache == nil {
		m.stackCache = map[string]stackRoom{}
	}
	m.stackCache[sp.Key] = r
	return r.items, r.more
}

func (m *Model) readStack(sp *space) stackRoom {
	if _, ok := reviewCardKey(sp.Key); ok {
		return stackRoom{}
	}
	key := m.ticketOf(sp)
	if key == "" || !m.stackOwner(sp, key) {
		return stackRoom{}
	}
	items := m.stk.Of(stack.Card{Key: key}, stackLookup{m})
	var more int
	if max := m.stk.Max(); len(items) > max {
		more = len(items) - max
		items = items[:max]
	}
	return stackRoom{items, more}
}

// stackOwner is whether sp is the card of its ticket that carries the stack.
// A ticket can have several worktrees: the one it was first merged from, and
// one made to fix what QA failed. The unmerged ones carry the stack, since
// that is where the fix is being made; when all are merged, the newest folder
// (the last by name, as a fix's folder follows the first) does.
func (m *Model) stackOwner(sp *space, key string) bool {
	if _, ok := jiraCardKey(sp.Key); ok {
		return true
	}
	info, ok := m.pipeInfo[sp.Key]
	if !ok || info.Key == "" {
		return false
	}
	merged, newest := info.MergedTo != "", true
	for other, o := range m.pipeInfo {
		if other == sp.Key || !strings.EqualFold(o.Key, key) {
			continue
		}
		switch {
		case o.MergedTo == "" && merged:
			return false
		case o.MergedTo != "" && merged && other > sp.Key:
			newest = false
		}
	}
	return !merged || newest
}

// stackColumn is the column [stack] column moves sp to, "" for none: a card
// whose work was merged but whose stack says it is not finished goes back to
// be picked up. A card that is still being worked on stays where it is.
func (m *Model) stackColumn(sp *space) string {
	col := m.stk.Column()
	if col == "" {
		return ""
	}
	if _, ok := m.columns.Find(col); !ok {
		return ""
	}
	if _, placed := m.placed(sp.Key); placed {
		return "" // moved by hand: that is where it stays
	}
	if _, jiraOnly := jiraCardKey(sp.Key); !jiraOnly && m.pipeInfo[sp.Key].MergedTo == "" {
		return ""
	}
	if items, _ := m.stackOf(sp); len(items) == 0 {
		return ""
	}
	return col
}

// stackAttention is what a stack asks of you, "2 failed", or "" when none of
// its strips does.
func (m *Model) stackAttention(sp *space) string {
	items, more := m.stackOf(sp)
	n, word := more, ""
	for _, it := range items {
		if it.Attention {
			n++
			if word == "" {
				word = it.Word
			}
		}
	}
	if word == "" {
		return ""
	}
	return fmt.Sprintf("%d %s", n, word)
}

// stackWork is what a handoff of sp has to fix: the keys of its stack's
// items that ask for attention (all of them when none does), comma-separated,
// and what the source says about each, a line apiece. Both are "" without a
// stack.
func (m *Model) stackWork(sp *space) (tests, evidence string) {
	items, _ := m.stackOf(sp)
	var picked []stack.Item
	for _, it := range items {
		if it.Attention {
			picked = append(picked, it)
		}
	}
	if len(picked) == 0 {
		picked = items
	}
	var keys, notes []string
	for _, it := range picked {
		keys = append(keys, it.Key)
		line := it.Key
		if it.Summary != "" {
			line += " " + it.Summary
		}
		if it.Evidence != "" {
			line += ": " + it.Evidence
		}
		notes = append(notes, line)
	}
	return strings.Join(keys, ","), strings.Join(notes, "\n")
}

// stackLookup lends the stack's sources the tickets the board has read.
type stackLookup struct{ m *Model }

func (l stackLookup) Ticket(key string) (stack.Ticket, bool) {
	d, ok := l.m.jr.detail[key]
	if !ok {
		return stack.Ticket{}, false
	}
	linked := func(ls []jira.Link) []stack.Linked {
		out := make([]stack.Linked, 0, len(ls))
		for _, x := range ls {
			out = append(out, stack.Linked{Key: x.Key, Summary: x.Summary, Status: x.Status, Type: x.Type, URL: links.Issue(x.Key)})
		}
		return out
	}
	t := stack.Ticket{Key: d.Key, Tests: linked(d.Tests), Links: linked(d.Links)}
	for _, c := range d.Comments {
		t.Notes = append(t.Notes, stack.Note{Author: c.Author, At: c.At, Body: c.Body})
	}
	return t, true
}

// loadStackDetails reads the tickets of the board ahead of time, a few at a
// pass, so a stack is known without anyone opening the card's detail.
func (m *Model) loadStackDetails() tea.Cmd {
	if !m.stk.On() || !m.jiraOn() {
		return nil
	}
	seen := map[string]bool{}
	var wanted []string
	add := func(k string) {
		if k != "" && !seen[k] && m.stk.Needs(stack.Card{Key: k}) {
			seen[k] = true
			wanted = append(wanted, k)
		}
	}
	for _, info := range m.pipeInfo {
		add(info.Key)
	}
	for k := range m.jr.issues {
		add(k)
	}
	sort.Strings(wanted)
	var cmds []tea.Cmd
	for _, k := range wanted {
		if len(cmds) == stackFetchBatch {
			break
		}
		if m.jr.fetching[k] || time.Since(m.jr.detailAt[k]) < stackEvery {
			continue
		}
		cmds = append(cmds, m.fetchDetail(k))
	}
	return tea.Batch(cmds...)
}

// keyScopes are the key scopes in force: the stack's while the selected card
// has one and the cursor is not carrying a card.
func (m *Model) keyScopes() []string {
	var out []string
	if q := m.keyScope(); q != "" {
		out = append(out, q)
	}
	if !m.stk.On() || m.grabbed != "" || m.archiveView || m.layout != layoutKanban && m.mode != modeDetail {
		return out
	}
	if items, _ := m.stackOf(m.selected()); len(items) > 0 {
		out = append(out, scopeStack)
	}
	return out
}

// stackWalk moves one step through the selected card's stack. At either end
// of it the cursor goes on to the neighbouring card, as j and k always did:
// up into a stacked card lands on its last strip, down lands on the card.
func (m *Model) stackWalk(delta int) {
	sp := m.selected()
	items, _ := m.stackOf(sp)
	if sp == nil || m.stk.Step(sp.Key, len(items), delta) {
		return
	}
	before := sp.Key
	m.moveCursor(delta)
	next := m.selected()
	if next == nil || next.Key == before {
		return // the end of the board: stay on the last strip
	}
	if n, _ := m.stackOf(next); len(n) > 0 {
		if delta < 0 {
			m.stk.Set(next.Key, len(n), len(n)-1)
		} else {
			m.stk.Set(next.Key, len(n), -1)
		}
	}
}

// stackKey takes the stack's keys on the board. It returns the key the board
// goes on with when the stack leaves it to something else, and whether it
// handled the key itself.
func (m *Model) stackKey(key string) (string, tea.Cmd, bool) {
	switch key {
	case "stack-down", "stack-up":
		delta := 1
		if key == "stack-up" {
			delta = -1
		}
		m.stackWalk(delta)
		return "", m.selectedBellCleared(), true
	case "stack-open":
		if cmd, ok := m.stackOpen(); ok {
			return "", cmd, true
		}
		return "open-issue", nil, false // the cursor is on the card itself
	}
	return key, nil, false
}

// stackOpen opens the ticket of the strip under the cursor; false when the
// cursor is on the card.
func (m *Model) stackOpen() (tea.Cmd, bool) {
	sp := m.selected()
	items, _ := m.stackOf(sp)
	if sp == nil || len(items) == 0 {
		return nil, false
	}
	at := m.stk.At(sp.Key, len(items))
	if at < 0 {
		return nil, false
	}
	it := items[at]
	url := it.URL
	if url == "" {
		url = links.Issue(it.Key)
	}
	if url == "" {
		m.status = "issue_url is missing from config.toml"
		return nil, true
	}
	m.status = "opening " + it.Key
	return openURLCmd(url), true
}

// stackClick is a click on strip i of the card at col, row: the first selects
// it, the second opens the detail on it.
func (m *Model) stackClick(col, row, i int) (tea.Model, tea.Cmd) {
	already := m.col == col && m.rowInCol == row
	m.col, m.rowInCol = col, row
	m.clampColumnCursor()
	sp := m.selected()
	items, _ := m.stackOf(sp)
	if sp == nil || i < 0 || i >= len(items) {
		return m, nil
	}
	already = already && m.stk.At(sp.Key, len(items)) == i
	m.stk.Set(sp.Key, len(items), i)
	if already {
		return m.pressKey(m.hintKey("detail"))
	}
	return m, nil
}

// stackStrips are the card's strips as the screen draws them.
func stackStrips(items []stack.Item) []screen.StackLine {
	out := make([]screen.StackLine, len(items))
	for i, it := range items {
		out[i] = screen.StackLine{Key: it.Key, Word: it.Word, Attention: it.Attention}
	}
	return out
}

// stackCue is where the keys lead from the cursor, in both directions.
func (m *Model) stackCue(sp *space, items []stack.Item) string {
	at := m.stk.At(sp.Key, len(items))
	up, down := stack.Neighbours(m.ticketOf(sp), items, at)
	var parts []string
	if at >= 0 {
		parts = append(parts, "↑ "+up)
	}
	if down == "" {
		down = "next card"
	}
	parts = append(parts, "↓ "+down)
	return dimStyle.Render(strings.Join(parts, " · "))
}

// stackOnCard adds a card's stack to what is drawn: its strips when selected
// (or when collapse is off), else one line that says how many there are.
func (m *Model) stackOnCard(c *screen.Card, sp *space, selected bool) {
	items, more := m.stackOf(sp)
	if len(items) == 0 {
		return
	}
	if !selected && m.stk.Collapse() {
		style := dimStyle
		if m.stackAttention(sp) != "" {
			style = look.Attention
		}
		c.Body = append(c.Body, style.Render(stack.Summary(items, more)))
		return
	}
	c.Stack, c.StackMore = stackStrips(items), more
	c.StackAt = m.stk.At(sp.Key, len(items))
	if selected {
		c.Cue = m.stackCue(sp, items)
	}
}

// stackActions are the card's stack buttons and menu entries: previous and
// next, and the ticket under the cursor when it is a strip.
func (m *Model) stackActions(sp *space) []cardAction {
	items, _ := m.stackOf(sp)
	if len(items) == 0 {
		return nil
	}
	var out []cardAction
	// A card with a worktree has no Hand off of its own; with a stack to fix
	// and [jira.handoff] rework_command set, it has one.
	if _, jiraOnly := jiraCardKey(sp.Key); !jiraOnly && m.jiraOn() && len(m.jr.cfg.Handoff.ReworkCommand) > 0 {
		if tests, _ := m.stackWork(sp); tests != "" {
			out = append(out, cardAction{"handoff", "Fix", true, "Hand off: fix the related tickets"})
		}
	}
	// Arrows, not words: a card is narrow, and a button that does not fit is
	// dropped, so these must be short to survive beside Hand off.
	out = append(out,
		cardAction{"stack-up", "↑", true, "Previous related ticket"},
		cardAction{"stack-down", "↓", true, "Next related ticket"})
	// `t` is the stack's while it has one, so the ticket's own button moves
	// to it: it opens the strip under the cursor, or the card's ticket.
	if at := m.stk.At(sp.Key, len(items)); at >= 0 {
		return append(out, cardAction{"stack-open", "Open", true, "Open " + items[at].Key + " in the tracker"})
	}
	return append(out, cardAction{"stack-open", "Ticket", true, "Open the ticket"})
}

// stackDetail is the detail's stack: where the cursor is in it and where the
// keys lead, then the cards of the stack. onStrip says the cursor is on a
// strip, whose own detail replaces the card's.
func (m *Model) stackDetail(sp *space, width int) (lines []detailLine, onStrip bool) {
	items, more := m.stackOf(sp)
	if len(items) == 0 {
		return nil, false
	}
	at := m.stk.At(sp.Key, len(items))
	key := m.ticketOf(sp)
	up, down := stack.Neighbours(key, items, at)
	head := fmt.Sprintf("Stack %d of %d", at+2, len(items)+1)
	if up != "" {
		head += " · ↑ " + up
	}
	if down == "" {
		down = "next card"
	}
	head += " · ↓ " + down
	lines = append(lines, plain(dimStyle.Render(truncate(head, width)))...)

	row := func(i int, k, word, url string, attention bool) {
		mark, name := "  ", titleStyle
		if i == at {
			mark, name = cursorStyle.Render("❯ "), cursorStyle
		}
		text := mark + name.Render(k)
		if word != "" {
			style := dimStyle
			if attention {
				style = look.Attention
			}
			text += " " + style.Render(word)
		}
		lines = append(lines, detailLine{text: text, url: url})
	}
	row(-1, key, "this card", "", false)
	for i, it := range items {
		row(i, it.Key, it.Word, it.URL, it.Attention)
	}
	if more > 0 {
		lines = append(lines, plain(dimStyle.Render(fmt.Sprintf("  +%d more", more)))...)
	}
	lines = append(lines, plain(dimStyle.Render(strings.Repeat("─", width)))...)
	if at < 0 {
		return lines, false
	}
	return append(lines, m.stackItemDetail(items[at], width)...), true
}

// stackItemDetail is the detail of one strip: what the source knows of it.
func (m *Model) stackItemDetail(it stack.Item, width int) []detailLine {
	var lines []detailLine
	value := max(width-screen.FieldLabel, 8)
	field := func(label string, text []string, url string) {
		if len(text) == 0 {
			return
		}
		for i, l := range screen.Field(label, text) {
			d := detailLine{text: l}
			if i == 0 {
				d.url = url
			}
			lines = append(lines, d)
		}
	}
	for _, l := range screen.Wrap(it.Summary, width) {
		lines = append(lines, detailLine{text: titleStyle.Render(l)})
	}
	if len(lines) > 0 {
		lines = append(lines, detailLine{})
	}
	if it.Word != "" {
		style := dimStyle
		if it.Attention {
			style = look.Attention
		}
		field("result", []string{style.Render(truncate(it.Word, value))}, "")
	}
	if it.Status != "" {
		field("status", []string{dimStyle.Render(truncate(it.Status, value))}, "")
	}
	if it.Evidence != "" {
		var ev []string
		for _, l := range screen.Wrap(it.Evidence, value) {
			ev = append(ev, noteStyle.Render(l))
		}
		field("why", ev, "")
	}
	url := it.URL
	if url == "" {
		url = links.Issue(it.Key)
	}
	field("ticket", []string{dimStyle.Render(truncate(it.Key, value))}, url)
	return lines
}
