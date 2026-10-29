package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/bitbucket"
	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// A card says three things, in this order of weight: what it is (its key in
// the border and its title), whether it asks for you (the border turns red),
// and how it is doing (its facts). A card at rest shows its heaviest fact; the
// selected card shows all of them and the buttons for what can be done with
// it. The detail shows the same facts again, labelled.

// fact is one thing a card knows about its work.
type fact struct {
	glyph, text string
	style       lipgloss.Style
	// weight is how much the fact says about the card: a card at rest shows
	// its heaviest. Facts that ask for you weigh the most.
	weight    int
	attention bool
	url       string // what a click on it opens in the detail
	kind      string // which field of the detail it belongs to
	// short is the fact on a card when glyphs are on, already styled: the
	// glyph says what text spells out ("Jira:", "Jenkins green"). Empty, the
	// card says text.
	short string
}

// said is a short form: glyph and text in style.
func said(glyph, text string, style lipgloss.Style) string {
	return style.Render(glyph + " " + text)
}

// Fact weights.
const (
	weightAside   = iota // read from somewhere that could not be reached
	weightContext        // the tracker's status, the team's phase
	weightState          // a pull request, a closed workspace
	weightAgent          // an agent at work, or finished
	weightYou            // something asks for you
)

// Detail fields a fact can belong to.
const (
	kindTicket = "ticket"
	kindAgent  = "agent"
	kindPR     = "pull req"
)

// line draws the fact width cells wide.
func (m *Model) line(f fact, width int) string {
	return screen.Say(m.glyph(f.glyph, f.text), f.style, width)
}

// cardLine draws the fact on a card: its short form when glyphs are on.
func (m *Model) cardLine(f fact, width int) string {
	if m.icons && f.short != "" {
		return screen.TruncateStyled(f.short, width)
	}
	return m.line(f, width)
}

// badge is the fact as its glyph alone, in its colour; "" without glyphs.
func (m *Model) badge(f fact) string {
	if !m.icons || f.glyph == "" {
		return ""
	}
	return f.style.Render(f.glyph)
}

// statusLine is what a card at rest says about its work on one line: its
// heaviest fact, then the others as badges at the right, so every state shows
// at a glance and selecting the card spells them out. Badges give way, the
// lightest first, before the heaviest fact is cut below minFactWidth.
func (m *Model) statusLine(facts []fact, width int) string {
	if len(facts) == 0 {
		return ""
	}
	top := 0
	for i, f := range facts {
		if f.weight > facts[top].weight {
			top = i
		}
	}
	type mark struct {
		text   string
		weight int
	}
	var marks []mark
	seen := map[string]bool{}
	for i, f := range facts {
		if b := m.badge(f); i != top && b != "" && !seen[b] {
			seen[b] = true
			marks = append(marks, mark{b, f.weight})
		}
	}
	room := func() int { return width - 2*len(marks) }
	for len(marks) > 0 && room() < minFactWidth {
		light := 0
		for i, k := range marks {
			if k.weight < marks[light].weight {
				light = i
			}
		}
		marks = append(marks[:light], marks[light+1:]...)
	}
	if len(marks) == 0 {
		return m.cardLine(facts[top], width)
	}
	right := make([]string, len(marks))
	for i, k := range marks {
		right[i] = k.text
	}
	badges := strings.Join(right, " ")
	left := m.cardLine(facts[top], width-lipgloss.Width(badges)-1)
	return screen.JoinEnds(left, badges, width)
}

// minFactWidth is the least a card's heaviest fact keeps before its badges.
const minFactWidth = 12

// facts is what the board knows about a card's work, in reading order.
func (m *Model) facts(sp *space) []fact {
	if !m.pipelineOn() {
		return m.spaceFacts(sp)
	}
	if _, ok := reviewCardKey(sp.Key); ok {
		return m.reviewFacts(sp)
	}
	if k, ok := jiraCardKey(sp.Key); ok {
		var out []fact
		if f, ok := m.jiraFact(k); ok {
			out = append(out, f)
		}
		if r, ok := m.reviewFor(sp); ok {
			out = append(out, m.reviewFact(r))
		}
		text := "no worktree" + strings.TrimRight(pressWith(m.hintKey("handoff"), "to start it"), " ")
		return append(out, fact{glyph: look.Pause, text: text, style: dimStyle, weight: weightState, kind: kindTicket})
	}
	info, ok := m.pipeInfo[sp.Key]
	if !ok {
		return []fact{{text: m.spinner.Frame() + " working it out", style: dimStyle, weight: weightAside}}
	}
	var out []fact
	if f, ok := m.jiraFact(info.Key); ok {
		out = append(out, f)
	}
	if r, ok := m.reviewFor(sp); ok {
		out = append(out, m.reviewFact(r))
	}
	if f, ok := m.forYouFact(sp.Key); ok {
		out = append(out, f)
	}
	if f, ok := m.questionsFact(sp.Key); ok {
		out = append(out, f)
	}
	if _, ok := m.placed(sp.Key); ok {
		// Where the board would put it; the badge already says it was moved.
		board := m.statusLabelOf(info.Stage)
		out = append(out, fact{glyph: look.Hand, text: "board: " + board, style: dimStyle, weight: weightState,
			short: said(look.Hand, board, dimStyle)})
	}
	if !sp.Live {
		// No workspace, so no agents: waiting and lost do not apply.
		info.Waiting, info.Lost = false, false
		text := "workspace closed"
		if k := m.hintKey("jump"); k != "" {
			text += " · " + screen.Key(screen.KeyName(k)) + " reopens"
		}
		out = append(out, fact{glyph: look.Pause, text: text, style: dimStyle, weight: weightState, kind: kindAgent})
	}
	if a, ok := ticket.Loudest(info.Agents); ok && sp.Live {
		al := look.Agent(a.Status)
		text, short := a.Who+" "+al.Word, a.Who
		if a.Title != "" {
			text += " · " + a.Title
			short += " · " + a.Title
		}
		f := fact{glyph: al.Glyph, text: text, style: al.Style, weight: weightAgent, kind: kindAgent}
		if al.Glyph != "" {
			f.short = said(al.Glyph, short, al.Style)
		}
		switch a.Status {
		case "blocked":
			f.weight, f.attention = weightYou, true
			info.Waiting = false // said already
		case "idle":
			f.weight = weightContext
		}
		out = append(out, f)
	}
	if info.Waiting {
		out = append(out, fact{glyph: look.Question, text: "asking you something", style: look.Attention, weight: weightYou, attention: true, kind: kindAgent})
	}
	if info.Lost {
		out = append(out, fact{glyph: look.Warning, text: "agents were lost", style: look.Attention, weight: weightYou, attention: true, kind: kindAgent})
	}
	if info.Phase != "" && sp.StatusID == pipeline.InProgress {
		out = append(out, fact{glyph: look.Team, text: info.Phase, style: dimStyle, weight: weightContext, kind: kindAgent})
	}
	if info.PR > 0 {
		out = append(out, m.prFact(info))
		// Whether Bitbucket would let it in, which a green build does not say.
		if f, ok := m.bitbucketFact(info); ok {
			out = append(out, f)
		}
	}
	if info.Diverged {
		out = append(out, fact{glyph: look.Warning, text: divergedText, style: dimStyle, weight: weightContext, kind: kindPR})
	}
	if info.Deployed != "" {
		out = append(out, fact{glyph: look.Rocket, text: "shipped to " + info.Deployed, style: prPassStyle, weight: weightContext, kind: kindPR,
			short: said(look.Rocket, info.Deployed, prPassStyle)})
	}
	if info.QA != "" {
		if info.InQA {
			out = append(out, fact{glyph: look.Check, text: "on " + info.QA, style: prPassStyle, weight: weightContext, kind: kindPR})
		} else {
			out = append(out, fact{glyph: look.Warning, text: info.QA + " runs an older build", style: dimStyle, weight: weightContext, kind: kindPR})
		}
	}
	if info.Unknown != "" {
		out = append(out, fact{glyph: look.Wifi, text: info.Unknown, style: dimStyle, weight: weightAside})
	}
	return out
}

func (m *Model) isPlaced(key string) bool {
	_, ok := m.placed(key)
	return ok
}

// handBadge marks a card moved by hand in its border: the Nerd Font hand, or
// one that any font draws.
func handBadge(icons bool) string {
	if icons {
		return look.Hand
	}
	return "✋"
}

// placedText says, for the copied context, that a card was moved by hand and
// where the board would put it. Another session reads it, so it is spelt out.
func (m *Model) placedText(key string) (string, bool) {
	if _, ok := m.placed(key); !ok {
		return "", false
	}
	return "moved by hand; the board says " + m.columns.Label(m.pipeInfo[key].Stage), true
}

// divergedText is said of a merge that names the branch but does not hold the
// worktree's commit: the branch was rewritten on the server before merging.
const divergedText = "differs from the merge"

// prFact is the pull request with its build.
func (m *Model) prFact(info pipeline.Info) fact {
	f := fact{glyph: look.PullReq, text: fmt.Sprintf("PR #%d", info.PR), style: dimStyle, weight: weightState,
		url: links.PullRequest(info.PR), kind: kindPR}
	switch info.Build {
	case "SUCCESS":
		f.text += " · " + m.pipe.CIName() + " green"
		f.style = prPassStyle
	case "FAILURE", "UNSTABLE":
		f.text += " · " + m.pipe.CIName() + " " + strings.ToLower(info.Build)
		f.style, f.weight, f.attention = prFailStyle, weightYou, true
	case "RUNNING":
		f.text += " · " + m.pipe.CIName() + " running"
		f.style = prPendingStyle
	}
	if info.MergedTo != "" {
		f.text += " · in " + info.MergedTo
	}
	// #180 ✓  master: the build is its glyph, where it went the target's.
	f.short = said(look.PullReq, fmt.Sprintf("#%d", info.PR), f.style)
	if b := look.Build(info.Build); b.Glyph != "" {
		f.short += " " + b.Style.Render(b.Glyph)
	}
	if info.MergedTo != "" {
		f.short += "  " + said(look.Target, info.MergedTo, dimStyle)
	}
	return f
}

// bitbucketFact is Bitbucket's verdict on the pull request, false when there
// is nothing to say.
func (m *Model) bitbucketFact(info pipeline.Info) (fact, bool) {
	if !m.bbOn() || info.PR <= 0 || info.MergedTo != "" {
		return fact{}, false
	}
	r, j, ok := m.judge(info)
	if !ok {
		if msg, failed := m.bb.errs[info.PR]; failed {
			return fact{glyph: look.Wifi, text: "Bitbucket: " + msg, style: dimStyle, weight: weightAside, kind: kindPR}, true
		}
		if _, asked := m.bb.at[info.PR]; !asked && m.bb.loading {
			return fact{text: m.spinner.Frame() + " reading Bitbucket", style: dimStyle, weight: weightAside, kind: kindPR}, true
		}
		return fact{}, false
	}
	more := ""
	if len(j.Why) > 1 {
		more = fmt.Sprintf(" +%d", len(j.Why)-1)
	}
	switch j.Verdict {
	case bitbucket.Fit:
		text := "ready to merge"
		if r.Target != "" {
			text += " into " + r.Target
		}
		return fact{glyph: look.Check, text: text, style: prPassStyle, weight: weightState, kind: kindPR,
			short: said(look.Check, "mergeable", prPassStyle)}, true
	case bitbucket.Waiting:
		return fact{glyph: look.Clock, text: "waiting: " + j.Why[0] + more, style: prPendingStyle, weight: weightState, kind: kindPR,
			short: said(look.Clock, j.Why[0]+more, prPendingStyle)}, true
	}
	return fact{glyph: look.Warning, text: "blocked: " + j.Why[0] + more, style: prFailStyle, weight: weightYou, attention: true, kind: kindPR,
		short: said(look.Warning, j.Why[0]+more, prFailStyle)}, true
}

// jiraFact is the ticket's status in Jira, false when Jira is off or the
// ticket is not on its list.
func (m *Model) jiraFact(key string) (fact, bool) {
	i, ok := m.jr.issues[key]
	if !m.jiraOn() || !ok {
		return fact{}, false
	}
	style := jiraStatusStyle(i.Category)
	return fact{glyph: look.Jira, text: "Jira: " + i.Status, style: style, weight: weightContext,
		url: links.Issue(key), kind: kindTicket, short: said(look.Jira, i.Status, style)}, true
}

// spaceFacts are a card's facts on the manual board: its pull request and
// what its agent is doing.
func (m *Model) spaceFacts(sp *space) []fact {
	var out []fact
	if pr, ok := m.prFor(sp.Key); ok {
		f := fact{text: prCell(pr), style: prStyle(pr), weight: weightState, url: pr.URL, kind: kindPR}
		if prBad(pr) {
			f.weight, f.attention = weightYou, true
		}
		out = append(out, f)
	}
	if al := look.Agent(sp.AgentStatus); sp.Live && al.Word != "" {
		f := fact{glyph: al.Glyph, text: "agent " + al.Word, style: al.Style, weight: weightContext, kind: kindAgent}
		switch sp.AgentStatus {
		case "blocked":
			f.style, f.weight, f.attention = look.Attention, weightYou, true
		case "working", "done":
			f.weight = weightAgent
		}
		out = append(out, f)
	} else if !sp.Live {
		out = append(out, fact{text: "offline", style: dimStyle, weight: weightContext, kind: kindAgent})
	}
	return out
}

// attention is why a card asks for you, "" when it does not.
func (m *Model) attention(sp *space) string {
	for _, f := range m.facts(sp) {
		if f.attention {
			return screen.Plain(f.text)
		}
	}
	return m.stackAttention(sp)
}

// needingYou are the cards that ask for you, in board order.
func (m *Model) needingYou() []*space {
	var out []*space
	for _, st := range m.board.Statuses {
		for _, sp := range m.groups[st.ID] {
			if m.attention(sp) != "" {
				out = append(out, sp)
			}
		}
	}
	return out
}

// nextNeedingYou selects the card after the selected one that asks for you,
// going round to the first.
func (m *Model) nextNeedingYou() {
	cards := m.needingYou()
	if len(cards) == 0 {
		m.status = "nothing needs you"
		return
	}
	next := cards[0]
	current := m.selectedKey()
	for i, sp := range cards {
		if sp.Key == current {
			next = cards[(i+1)%len(cards)]
			break
		}
	}
	// It may sit in a folded column or group: open it.
	if m.board.IsCollapsed(next.StatusID) {
		m.board.ToggleCollapsed(next.StatusID)
		m.save()
		m.rebuild()
	}
	m.restoreCursor(next.Key)
	m.restoreColumnCursor(next.Key)
	m.clampCursor()
	m.status = m.spaceName(next) + ": " + m.attention(next)
}

// heaviest is the fact that says most about a card: the one it shows at
// rest, and the one a list row or a table row shows.
func (m *Model) heaviest(sp *space) (fact, bool) {
	facts := m.facts(sp)
	if len(facts) == 0 {
		return fact{}, false
	}
	top := facts[0]
	for _, f := range facts[1:] {
		if f.weight > top.weight {
			top = f
		}
	}
	return top, true
}

// about is what a row says about a space after its name: your note first,
// since it is why the space is filed where it is, then what the ticket is
// about, then where it lives.
func (m *Model) about(sp *space, width int) string {
	switch {
	case sp.Note != "":
		return noteStyle.Render(truncate(sp.Note, width))
	case m.headline(sp) != "":
		return labelStyle.Render(truncate(m.headline(sp), width))
	}
	return dimStyle.Render(truncate(abbreviate(sp.Key), width))
}

// marked is a row's name with the mark of whatever asks for you in front.
func (m *Model) marked(sp *space, name string) string {
	switch {
	case m.attention(sp) != "":
		return look.Attention.Render("●") + " " + name
	case m.hasBell(sp.Key):
		return bellGlyph + " " + name
	}
	return name
}

// headline is what a card is about, under its key: the ticket's title.
func (m *Model) headline(sp *space) string {
	if !m.pipelineOn() {
		return ""
	}
	if info, ok := m.pipeInfo[sp.Key]; ok && info.Title != "" {
		return info.Title
	}
	if s := m.summaryOf(sp); s != "" {
		return s
	}
	if _, ok := reviewCardKey(sp.Key); ok {
		r, _ := m.reviewFor(sp)
		return r.Title
	}
	return ""
}

// Lines of the headline a card shows: at rest, and selected.
const (
	headlineAtRest    = 2
	headlineSelected  = 4
	noteAtRestLines   = 1
	noteSelectedLines = 6
)

// card is a space as a kanban card width cells wide.
func (m *Model) card(sp *space, selected bool, width int) screen.Card {
	text := screen.CardInner(width)
	c := screen.Card{
		Title:     m.spaceLabel(sp),
		Selected:  selected,
		Grabbed:   sp.Key == m.grabbed,
		Dim:       !sp.Live,
		Attention: m.attention(sp) != "",
	}
	switch {
	case c.Attention:
		c.Badge = look.Attention.Render("●")
	case m.hasBell(sp.Key):
		c.Badge = bellGlyph
	case m.pipelineOn() && m.isPlaced(sp.Key):
		// A card moved by hand says so even at rest, where it shows one fact.
		c.Badge = dimStyle.Render(handBadge(m.icons))
	default:
		if i, ok := m.jr.issues[m.ticketOf(sp)]; ok && i.Type != "" {
			c.Badge = dimStyle.Render(i.Type)
			if g := look.IssueType(i.Type); m.icons && g != "" {
				c.Badge = dimStyle.Render(g)
			}
		}
	}
	// Open questions show from across the board, whatever else the border says.
	if q := m.questionMark(sp.Key); q != "" {
		c.Badge = strings.TrimSpace(q + " " + c.Badge)
	}

	headLines, noteLines := headlineAtRest, noteAtRestLines
	if selected {
		headLines, noteLines = headlineSelected, noteSelectedLines
	}
	c.Body = append(c.Body, clip(screen.Wrap(m.headline(sp), text), headLines, text, labelStyle)...)
	if sp.Note != "" {
		note := screen.Wrap(m.glyph(look.Pencil, sp.Note), text)
		c.Body = append(c.Body, clip(note, noteLines, text, noteStyle)...)
	}

	// At rest, one line: the heaviest fact and the others' badges. Selected,
	// each fact on its own.
	facts := m.facts(sp)
	if !selected {
		if l := m.statusLine(facts, text); l != "" {
			c.Body = append(c.Body, l)
		}
	} else {
		for _, f := range facts {
			c.Body = append(c.Body, m.cardLine(f, text))
		}
	}

	m.stackOnCard(&c, sp, selected)

	if selected && m.mode == modeStatusPick {
		picker := m.pickerLines(text)
		c.Choices, c.NChoices = len(c.Body)+1, len(m.board.Statuses) // after the caption
		c.Body = append(c.Body, picker...)
	}
	if selected {
		c.Actions = m.cardButtons(sp)
	}
	return c
}

// clip styles the first n lines, ending the last in "…" when lines go on;
// width is what a line may take.
func clip(lines []string, n, width int, style lipgloss.Style) []string {
	var out []string
	for i, l := range lines {
		if i == n-1 && len(lines) > n {
			l = truncate(strings.TrimRight(l, " ")+"…", width)
		}
		if i == n {
			break
		}
		out = append(out, style.Render(l))
	}
	return out
}

// cardAction is something that can be done with a card: its action, its
// short label (a button's), whether the selected card shows it as a button,
// and its entry in the card's menu. The card, its menu and the detail all
// offer these, so they cannot drift apart.
type cardAction struct {
	action, label string
	onCard        bool
	menu          string
}

// cardActions are what can be done with sp, in the order its menu lists
// them.
func (m *Model) cardActions(sp *space) []cardAction {
	own := m.ticketActions(sp)
	extra := m.stackActions(sp)
	if len(extra) == 0 {
		return own
	}
	// The stack's open action does the ticket's: it takes its place.
	kept := own[:0:0]
	for _, a := range own {
		if a.action != "open-issue" {
			kept = append(kept, a)
		}
	}
	return append(kept, extra...)
}

// ticketActions are cardActions without the stack's.
func (m *Model) ticketActions(sp *space) []cardAction {
	if !m.pipelineOn() {
		out := []cardAction{
			{"jump", "Go", true, "Go to the space"},
			{"status-picker", "Status", true, "Set the status"},
			{"note", "Note", true, "Edit the note"},
		}
		if pr, ok := m.prFor(sp.Key); ok {
			out = append(out, cardAction{"open-pr", "PR", false, "Open the pull request"})
			if pr.Checks == gh.ChecksFail {
				out = append(out, cardAction{"send-failure", "Send failure", false, "Send the failing check to the agent"})
			}
		}
		return append(out,
			cardAction{"detail", "Detail", false, "Detail"},
			cardAction{"grab", "Move", false, "Move it"},
			cardAction{"rename", "Rename", false, "Rename"},
			cardAction{"message", "Message", false, "Message the agent"},
			cardAction{"forget", "Forget", false, "Forget it"})
	}
	if _, review := reviewCardKey(sp.Key); review {
		out := []cardAction{{"open-pull-request", "Review", true, "Open the pull request to review it"}}
		if m.ticketOf(sp) != "" {
			out = append(out, cardAction{"open-issue", "Ticket", true, "Open the ticket"})
		}
		return append(out,
			cardAction{"detail", "Detail", false, "Detail"},
			cardAction{"yank", "Copy", false, "Copy context"},
			cardAction{"note", "Note", false, "Edit the note"})
	}
	if _, jiraOnly := jiraCardKey(sp.Key); jiraOnly {
		out := []cardAction{
			{"handoff", "Hand off", true, "Hand off: start the work"},
			{"open-issue", "Ticket", true, "Open the ticket"},
		}
		if _, ok := m.reviewFor(sp); ok {
			out = append(out, cardAction{"open-pull-request", "Review", true, "Open the pull request to review it"})
		}
		return append(out,
			cardAction{"detail", "Detail", false, "Detail"},
			cardAction{"yank", "Copy", false, "Copy context"},
			cardAction{"note", "Note", false, "Edit the note"})
	}
	info := m.pipeInfo[sp.Key]
	out := []cardAction{{"jump", "Go", true, "Go to the workspace"}}
	if info.Key != "" {
		out = append(out, cardAction{"open-issue", "Ticket", true, "Open the ticket"})
	}
	if info.PR > 0 {
		out = append(out, cardAction{"open-pull-request", "PR", true, "Open the pull request"})
	}
	if _, ok := m.placed(sp.Key); ok {
		out = append(out, cardAction{"follow-board", "Undo", true, "Undo the move: back to " + m.columns.Label(info.Stage)})
	}
	if m.acceptable(sp.StatusID) {
		out = append(out, cardAction{"accept", "Accept", false, "Accept: move it to the Archive"})
	}
	if info.Key != "" && m.acceptable(sp.StatusID) {
		out = append(out, cardAction{"close-ticket", "Close", false, "Close it: accept it, close its workspace and remove the worktree"})
	}
	if m.whyNotPlace(sp) == "" {
		out = append(out, cardAction{"status-picker", "Move", false, "Move to another column by hand"})
	}
	if len(m.questionsOf(sp.Key))+len(m.forYouOf(sp.Key)) > 0 {
		out = append(out, cardAction{"questions", "Questions", false, "Read the open questions"})
	}
	if info.Key != "" {
		out = append(out,
			cardAction{"orchestrator", "Orchestrator", false, "Go to the orchestrator"},
			cardAction{"prototype", "Prototype", false, "Open the prototype"})
	}
	return append(out,
		cardAction{"detail", "Detail", false, "Detail"},
		cardAction{"yank", "Copy", false, "Copy context"},
		cardAction{"note", "Note", false, "Edit the note"})
}

// cardButtons are the selected card's buttons: its main actions, then ⋯ for
// the rest.
func (m *Model) cardButtons(sp *space) []screen.Hint {
	var out []screen.Hint
	for _, a := range m.cardActions(sp) {
		if a.onCard {
			out = append(out, screen.Hint{Key: m.hintKey(a.action), Label: a.label})
		}
	}
	return append(out, screen.Hint{Key: m.hintKey("menu"), Label: "⋯"})
}

// detailButtons are the detail's buttons: every action but the detail
// itself.
func (m *Model) detailButtons(sp *space) []screen.Hint {
	var out []screen.Hint
	if sp != nil {
		for _, a := range m.cardActions(sp) {
			if a.action != "detail" {
				out = append(out, screen.Hint{Key: m.hintKey(a.action), Label: a.label})
			}
		}
	}
	return out
}
