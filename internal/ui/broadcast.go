package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// The broadcast says one message to the orchestrators of the tickets on the
// board -- "rebase onto master", say -- each getting it as a message of its
// own. The box lists them, all ticked, with what each is doing; the message
// is written, or picked from [broadcast] presets, and goes out after a
// (Y)es/(N)o that names who gets it. {target} and {key} become each ticket's
// target branch and key.
//
// Keeping the target branches up to date (U) is here too: both are about the
// board's tickets as a whole rather than one card.

// recipient is one orchestrator the broadcast can reach, with what the
// quick choices pick it by: its ticket's column and whether it has a pull
// request still open.
type recipient struct {
	key, pane, status, target string
	stage                     string
	openPR                    bool
	on                        bool
}

type broadcastState struct {
	presets []string
	to      []recipient
	loading bool
	list    bool // the list of orchestrators has the keys, not the message
	idx     int  // the highlighted orchestrator
	preset  int  // the ready-made message in the message, -1 for none
}

// SetBroadcast sets the ready-made messages the broadcast offers.
func (m *Model) SetBroadcast(presets []string) { m.bc.presets = presets }

type recipientsMsg struct {
	to  []recipient
	err error
}

type broadcastMsg struct{ sent, failed []string }

// startBroadcast opens the box and looks for the orchestrators of the
// tickets the board shows, a search or a column filter narrowing them.
func (m *Model) startBroadcast() tea.Cmd {
	var worktrees []string
	cards := map[string]pipeline.Info{}
	for _, st := range m.board.Statuses {
		if m.statusFilter != "" && st.ID != m.statusFilter {
			continue
		}
		for _, sp := range m.groups[st.ID] {
			if _, jiraOnly := jiraCardKey(sp.Key); jiraOnly || !sp.Live || m.pipeInfo[sp.Key].Key == "" {
				continue
			}
			worktrees = append(worktrees, sp.Key)
			cards[sp.Key] = m.pipeInfo[sp.Key]
		}
	}
	m.bc.to, m.bc.loading, m.bc.list, m.bc.idx, m.bc.preset = nil, true, false, 0, -1
	m.mode = modeBroadcast
	m.input.SetValue("")
	m.input.Focus()
	src, client := m.pipe, m.client
	return func() tea.Msg {
		if client == nil {
			return recipientsMsg{}
		}
		live, err := ticket.ReadLive(client)
		if err != nil {
			return recipientsMsg{err: err}
		}
		var to []recipient
		for _, wt := range worktrees {
			run, err := src.Run(wt, live)
			if err != nil || run.OrchestratorPane == "" {
				continue
			}
			info := cards[wt]
			to = append(to, recipient{key: run.Key, pane: run.OrchestratorPane, status: run.OrchestratorStatus, target: run.Target,
				stage: info.Stage, openPR: info.PR > 0 && info.MergedTo == "", on: true})
		}
		sort.Slice(to, func(i, j int) bool { return to[i].key < to[j].key })
		return recipientsMsg{to: to}
	}
}

func (m *Model) applyRecipients(msg recipientsMsg) {
	m.bc.loading = false
	m.bc.to = msg.to
	if msg.err != nil {
		m.status = msg.err.Error()
	}
}

// chosen are the ticked orchestrators.
func (m *Model) chosen() []recipient {
	var out []recipient
	for _, r := range m.bc.to {
		if r.on {
			out = append(out, r)
		}
	}
	return out
}

// expand is text as r gets it: {target} its ticket's target branch, or the
// board's most downstream one when the run names none, and {key} its key.
func (m *Model) expand(text string, r recipient) string {
	target := r.target
	if target == "" {
		if t := m.pipe.Targets(); len(t) > 0 {
			target = t[0]
		}
	}
	if target == "" {
		target = "{target}"
	}
	return strings.NewReplacer("{target}", target, "{key}", r.key).Replace(text)
}

func (m *Model) handleBroadcastKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k := msg.String(); k {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		m.status = "nothing was sent"
		return m, nil
	case "tab", "shift+tab":
		m.bc.list = !m.bc.list && len(m.bc.to) > 0
		return m, nil
	case "enter":
		m.askBroadcast()
		return m, nil
	case "up", "down":
		// In the message the arrows go through the ready-made messages, the
		// way a shell goes through its history.
		if !m.bc.list && len(m.bc.presets) > 0 {
			n := len(m.bc.presets)
			step := 1
			if k == "up" {
				step = n - 1
			}
			next := (m.bc.preset + step) % n
			if m.bc.preset < 0 {
				next = map[bool]int{true: n - 1, false: 0}[k == "up"]
			}
			m.usePreset(next)
			return m, nil
		}
		m.broadcastListKey(k)
		return m, nil
	default:
		if m.bc.list {
			m.broadcastListKey(k)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// broadcastListKey moves through and ticks the orchestrators. Like a
// picker's, these keys are the box's own.
func (m *Model) broadcastListKey(k string) {
	n := len(m.bc.to)
	switch k {
	case "j", "down":
		m.bc.idx++
	case "k", "up":
		m.bc.idx--
	case " ":
		if m.bc.idx < n {
			m.bc.to[m.bc.idx].on = !m.bc.to[m.bc.idx].on
		}
	case "h", "left", "l", "right":
		// Sideways goes through the quick choices, each in force as it is
		// reached, the way sideways goes through the board's columns.
		picks := m.picks()
		cur := -1
		for i, p := range picks {
			if m.inForce(p) {
				cur = i
				break
			}
		}
		next := (cur + 1) % len(picks)
		if k == "h" || k == "left" {
			next = (cur - 1 + len(picks)) % len(picks)
			if cur < 0 {
				next = len(picks) - 1
			}
		}
		m.choose(picks[next])
	default:
		// A quick choice's key picks it.
		for _, p := range m.picks() {
			if p.key == k {
				m.choose(p)
			}
		}
	}
	m.bc.idx = max(0, min(m.bc.idx, n-1))
}

// pick is a quick choice of orchestrators: its key in the list, and which
// it ticks.
type pick struct {
	key, label, color string
	match             func(recipient) bool
}

// picks are the quick choices that would tick someone: everyone, nobody,
// the ones with a pull request open, by what the orchestrator is doing, and
// by the column of its ticket.
func (m *Model) picks() []pick {
	out := []pick{
		{"a", "All", "252", func(recipient) bool { return true }},
		{"n", "None", "245", func(recipient) bool { return false }},
		{"p", "Open PR", "78", func(r recipient) bool { return r.openPR }},
		{"i", "Idle", "245", func(r recipient) bool { return r.status == "idle" }},
		{"w", "Working", "214", func(r recipient) bool { return r.status == "working" }},
		{"b", "Needs you", "203", func(r recipient) bool { return r.status == "blocked" }},
	}
	for _, c := range m.columns {
		id := c.ID
		out = append(out, pick{"", c.Label, c.Color, func(r recipient) bool { return r.stage == id }})
	}
	// The columns are numbered as they show, so their keys are 1, 2, 3...
	var shown []pick
	column := 0
	for i, p := range out {
		if i >= 2 && m.picked(p) == 0 {
			continue
		}
		if p.key == "" && column < 9 {
			column++
			p.key = strconv.Itoa(column)
		}
		shown = append(shown, p)
	}
	return shown
}

// picked is how many orchestrators p ticks.
func (m *Model) picked(p pick) int {
	n := 0
	for _, r := range m.bc.to {
		if p.match(r) {
			n++
		}
	}
	return n
}

// inForce is whether the ticks are exactly p's.
func (m *Model) inForce(p pick) bool {
	for _, r := range m.bc.to {
		if r.on != p.match(r) {
			return false
		}
	}
	return len(m.bc.to) > 0
}

// choose ticks exactly the orchestrators p picks.
func (m *Model) choose(p pick) {
	for i := range m.bc.to {
		m.bc.to[i].on = p.match(m.bc.to[i])
	}
}

func (m *Model) usePreset(i int) {
	m.bc.preset = i
	m.input.SetValue(m.bc.presets[i])
	m.input.CursorEnd()
	m.bc.list = false
}

// askBroadcast asks before sending: it is an order to several agents at once.
func (m *Model) askBroadcast() {
	text := strings.TrimSpace(m.input.Value())
	to := m.chosen()
	switch {
	case m.bc.loading:
		m.status = "still looking for the orchestrators"
		return
	case text == "":
		m.status = "write a message, or pick a ready-made one"
		return
	case len(to) == 0:
		m.status = "tick at least one orchestrator"
		return
	}
	var keys []string
	busy := 0
	for _, r := range to {
		keys = append(keys, r.key)
		if r.status == "working" {
			busy++
		}
	}
	said := "“" + m.expand(text, to[0]) + "”"
	if m.expand(text, to[0]) != text {
		said += " (as " + to[0].key + " gets it)"
	}
	lines := []string{said, "To: " + strings.Join(keys, ", ") + "."}
	if busy > 0 {
		lines = append(lines, fmt.Sprintf("%d of them working: they get it when they are done.", busy))
	}
	noun := "orchestrators"
	if len(to) == 1 {
		noun = "orchestrator"
	}
	m.confirm(fmt.Sprintf("Send to %d %s?", len(to), noun), lines, func() (tea.Model, tea.Cmd) {
		m.mode = modeNormal
		m.input.Blur()
		return m, m.sendBroadcast(text, to)
	})
}

// sendBroadcast submits text to each orchestrator in to.
func (m *Model) sendBroadcast(text string, to []recipient) tea.Cmd {
	start := m.working(fmt.Sprintf("sending to %d…", len(to)))
	client := m.client
	msgs := make([]string, len(to))
	for i, r := range to {
		msgs[i] = m.expand(text, r)
	}
	return tea.Batch(start, func() tea.Msg {
		var out broadcastMsg
		for i, r := range to {
			if client == nil || client.SubmitToAgent(r.pane, msgs[i]) != nil {
				out.failed = append(out.failed, r.key)
				continue
			}
			out.sent = append(out.sent, r.key)
		}
		return out
	})
}

func (m *Model) applyBroadcast(msg broadcastMsg) {
	var parts []string
	if len(msg.sent) > 0 {
		parts = append(parts, "sent to "+strings.Join(msg.sent, ", "))
	}
	if len(msg.failed) > 0 {
		parts = append(parts, "could not reach "+strings.Join(msg.failed, ", "))
	}
	m.status = strings.Join(parts, " · ")
}

// viewBroadcast is the broadcast box over the board.
func (m *Model) viewBroadcast(base string) string {
	inner := max(min(m.width-detailModalMargin, 96)-4, 34)
	var lines []string
	var zones []zone // rows counted from the body's first line
	clickable := func(z zone) { z.Y = len(lines); z.X0, z.X1 = 0, inner; zones = append(zones, z) }

	lines = append(lines, look.Label.Render("MESSAGE"))
	clickable(zone{Kind: screen.OnControl, ID: "bc-message"})
	lines = append(lines, screen.Input("›", m.input.Value(), "write to every orchestrator ticked below", !m.bc.list, m.inputIn(screen.InputRoom("›", inner)), inner))

	if len(m.bc.presets) > 0 {
		lines = append(lines, "", look.Label.Render("READY-MADE")+dimStyle.Render("  a click, or ↑↓ in the message, puts it there"))
		for i, p := range m.bc.presets {
			clickable(zone{Kind: screen.OnControl, ID: "preset:" + strconv.Itoa(i)})
			lead, style := "  ", lipgloss.NewStyle()
			if i == m.bc.preset {
				lead, style = cursorStyle.Render("❯ "), cursorStyle
			}
			lines = append(lines, lead+style.Render(truncate(p, inner-4)))
		}
	}

	lines = append(lines, "", look.Label.Render("TO")+dimStyle.Render(fmt.Sprintf("  %d of %d", len(m.chosen()), len(m.bc.to))))
	// The quick choices, laid out at 0, 0 and moved under the heading.
	var picks screen.Zones
	if len(m.bc.to) > 0 {
		var items []screen.Item
		for i, p := range m.picks() {
			count := m.picked(p)
			if p.label == "None" {
				count = -1
			}
			items = append(items, screen.Badge(p.key, p.label, count, p.color, m.inForce(p), "pick:"+strconv.Itoa(i)))
		}
		rows := screen.ItemRows(&picks, 0, 0, items, 1, inner)
		picks.Shift(0, 0, len(lines))
		lines = append(lines, rows...)
	}
	switch {
	case m.bc.loading:
		lines = append(lines, dimStyle.Render("  finding the orchestrators…"))
	case len(m.bc.to) == 0:
		lines = append(lines, dimStyle.Render("  no ticket on the board has an orchestrator open"))
	}
	for i, r := range m.bc.to {
		mark := "[ ] "
		if r.on {
			mark = "[x] "
		}
		lead := "  "
		style := lipgloss.NewStyle()
		if m.bc.list && i == m.bc.idx {
			lead, style = cursorStyle.Render("❯ "), cursorStyle
		}
		al := look.Agent(r.status)
		target := ""
		if r.target != "" {
			target = dimStyle.Render(" → " + r.target)
		}
		clickable(zone{Kind: zoneStatus, Choice: i})
		lines = append(lines, lead+style.Render(mark+pad(r.key, 12))+al.Style.Render(pad(al.Word, 11))+target)
	}
	// What the keys do depends on which part has them.
	help := screen.Key("↑↓") + " ready-made · " + screen.Key("tab") + " to the list · or click"
	if m.bc.list {
		help = screen.Key("←→") + " quick choices · " + screen.Key("↑↓") + " move · " + screen.Key("space") + " ticks one · " + screen.Key("tab") + " to the message"
	}
	lines = append(lines, "", screen.Say(help, dimStyle, inner))

	var buttons screen.Zones
	n := len(m.chosen())
	bottom := screen.ButtonRows(&buttons, 0, 0, []hint{{Key: "enter", Label: fmt.Sprintf("Send to %d", n)}, {Key: "esc", Label: "Cancel"}}, inner)
	title := titleStyle.Render("Broadcast to the orchestrators")
	box := screen.Modal(screen.Closable(title, inner), lines, bottom, inner)
	x, y := screen.Center(lipgloss.Width(box), lipgloss.Height(box), m.width, m.height)

	m.addZone(zone{Kind: zoneModal, Y: y, H: lipgloss.Height(box), X0: x, X1: x + lipgloss.Width(box)})
	for _, z := range picks {
		z.Y += y + screen.ModalBody
		z.X0, z.X1 = z.X0+x+2, z.X1+x+2
		m.addZone(z)
	}
	for _, z := range zones {
		z.Y += y + screen.ModalBody
		z.X0, z.X1 = z.X0+x+2, z.X1+x+2
		m.addZone(z)
	}
	mark := len(m.zones)
	m.zones = append(m.zones, buttons...)
	m.zones.Shift(mark, x+2, y+screen.ModalBody+len(lines)+1)
	m.addZone(screen.CloseZone(x, y, inner, "esc"))
	return screen.Overlay(base, box, x, y, m.height)
}

// clickBroadcast acts on a click while the box is open: an orchestrator
// ticks, a ready-made message fills the message, the buttons press their
// keys; a click elsewhere does nothing, so the message is not lost.
func (m *Model) clickBroadcast(x, y int) (tea.Model, tea.Cmd) {
	z, ok := m.zoneAt(x, y)
	switch {
	case !ok:
	case z.Kind == zoneStatus && z.Choice < len(m.bc.to):
		m.bc.to[z.Choice].on = !m.bc.to[z.Choice].on
		m.bc.list, m.bc.idx = true, z.Choice
	case z.Kind == screen.OnControl && z.ID == "bc-message":
		m.bc.list = false
	case z.Kind == screen.OnControl && strings.HasPrefix(z.ID, "pick:"):
		if i, err := strconv.Atoi(strings.TrimPrefix(z.ID, "pick:")); err == nil && i < len(m.picks()) {
			m.choose(m.picks()[i])
		}
	case z.Kind == screen.OnControl && strings.HasPrefix(z.ID, "preset:"):
		if i, err := strconv.Atoi(strings.TrimPrefix(z.ID, "preset:")); err == nil && i < len(m.bc.presets) {
			m.usePreset(i)
		}
	case z.Kind == zoneButton:
		return m.handleKey(screen.KeyMsg(z.Key))
	}
	return m, nil
}

type syncedMsg struct{ r pipeline.Synced }

// syncNow brings the local target branches up to their remote at once.
func (m *Model) syncNow() tea.Cmd {
	targets := m.pipe.Targets()
	if len(targets) == 0 {
		m.status = "no [[pipeline.target]] branches to bring up to date"
		return nil
	}
	start := m.working("bringing " + strings.Join(targets, " and ") + " up to date…")
	src := m.pipe
	return tea.Batch(start, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return syncedMsg{src.Sync(ctx)}
	})
}

func (m *Model) applySynced(msg syncedMsg) tea.Cmd {
	if s := msg.r.String(); s != "" {
		m.status = s
	} else {
		m.status = strings.Join(m.pipe.Targets(), " and ") + " already match origin"
	}
	return m.loadPipeline(false)
}
