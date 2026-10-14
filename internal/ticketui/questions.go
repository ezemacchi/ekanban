package ticketui

import (
	"strconv"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/orchestrator"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// The cursor reaches the questions as well as the roles: j past a column's
// last role goes on to the first question, k on the first question comes
// back. They are listed in three sections, in the order the cursor goes:
// the questions for you, the others' (grouped by who answers them), and the
// closed ones. A selected question is drawn in full with its buttons, as a
// selected card is: enter opens its chat, or replies to one for you; r
// (reply) types the answer's opening into the orchestrator; D (discuss)
// starts a session on its brief.

// The questions' sections; each folds.
const (
	forYouTitle = "Questions for you"
	forYouFold  = "section:" + forYouTitle
	openTitle   = "Open questions"
	openFold    = "section:" + openTitle
)

// qKind is which section a question is in.
type qKind int

const (
	kindForYou qKind = iota
	kindOther
	kindClosed
)

// others are the questions for other people, grouped by who answers them,
// the unknown last, each group in the run's order.
func (m *Model) others() []ticket.Question {
	if m.run == nil {
		return nil
	}
	return ticket.ByWho(m.run.Questions)
}

// questionSpan is how many questions of each section the cursor can reach:
// those drawn, in sections that are not folded.
func (m *Model) questionSpan() (forYou, others, closed int) {
	if m.run == nil {
		return 0, 0, 0
	}
	if !m.isFolded(forYouFold) {
		forYou = min(len(m.run.ForYou), questionsShown)
	}
	if !m.isFolded(openFold) {
		others = min(len(m.run.Questions), questionsShown)
	}
	if !m.isFolded(closedFold) {
		closed = min(len(m.run.Closed), questionsShown)
	}
	return forYou, others, closed
}

func (m *Model) questionCount() int {
	a, b, c := m.questionSpan()
	return a + b + c
}

// question is the i-th question the cursor can reach, and its section. A
// closed one is its line, its words without links.
func (m *Model) question(i int) (ticket.Question, qKind, bool) {
	forYou, others, closed := m.questionSpan()
	switch {
	case i < 0:
	case i < forYou:
		return m.run.ForYou[i], kindForYou, true
	case i < forYou+others:
		return m.others()[i-forYou], kindOther, true
	case i < forYou+others+closed:
		line := m.run.Closed[i-forYou-others]
		words, _ := links.InText(line)
		return ticket.Question{Line: line, Words: words}, kindClosed, true
	}
	return ticket.Question{}, kindClosed, false
}

// selectedQuestion is the question under the cursor, false when the cursor
// is on the roles.
func (m *Model) selectedQuestion() (ticket.Question, qKind, bool) {
	if !m.onQuestion {
		return ticket.Question{}, kindClosed, false
	}
	return m.question(m.q)
}

// down moves to the next role, and from a column's last role on to the
// questions, unfolding the open ones when they are folded.
func (m *Model) down() {
	if m.onQuestion {
		m.q++
		return
	}
	if m.row < m.navCount(m.col)-1 {
		m.row++
		return
	}
	if m.run != nil {
		if len(m.run.ForYou) > 0 && m.isFolded(forYouFold) {
			m.toggle(forYouFold)
		}
		if len(m.run.Questions) > 0 && m.isFolded(openFold) {
			m.toggle(openFold)
		}
	}
	if m.questionCount() > 0 {
		m.onQuestion, m.q = true, 0
	}
}

// up moves to the previous question, and from the first back to the roles.
func (m *Model) up() {
	switch {
	case !m.onQuestion:
		m.row--
	case m.q == 0:
		m.onQuestion = false
	default:
		m.q--
	}
}

// askSelected is enter on a question: it asks whether to open the chat of
// another's question, and replies to one for you, which has no chat.
func (m *Model) askSelected() tea.Cmd {
	q, kind, ok := m.selectedQuestion()
	if !ok {
		return nil
	}
	switch chats := q.Chats(); {
	case kind == kindClosed:
		m.status = "the question is closed: its chat is over"
	case kind == kindForYou:
		return m.reply()
	case len(chats) == 0:
		m.status = "the question carries no link"
	default:
		l := chats[0]
		m.sure = &l
	}
	return nil
}

// reply types into the run's orchestrator the opening of an answer to the
// selected question, naming its brief, unsent, and goes there: the user adds
// the answer and sends it, whatever the orchestrator was doing.
func (m *Model) reply() tea.Cmd {
	q, kind, ok := m.selectedQuestion()
	switch {
	case !ok:
		m.status = "select a question to reply to it"
		return nil
	case kind == kindClosed:
		m.status = "the question is closed"
		return nil
	case m.run.OrchestratorPane == "":
		m.status = "no orchestrator open to reply to" + keyHint(m.hintKey("orchestrator"), "opens one")
		return nil
	case m.run.OrchestratorStatus == "blocked":
		m.status = "the orchestrator is waiting at a prompt: answer it there first"
		return nil
	}
	text, pane, client := m.questionsCfg.ReplyText(q), m.run.OrchestratorPane, m.client
	m.status = "reply started in the orchestrator: write the answer and send it"
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		if err := client.SendToAgent(pane, text); err != nil {
			return orchestratorMsg{err: err}
		}
		_ = client.FocusAgent(pane)
		return nil
	}
}

// discuss opens a session on the selected question's brief: a new tab with
// the agent its needs asks for.
func (m *Model) discuss() tea.Cmd {
	q, kind, ok := m.selectedQuestion()
	switch {
	case !ok:
		m.status = "select a question to discuss it"
		return nil
	case kind == kindClosed:
		m.status = "the question is closed"
		return nil
	case q.Brief == nil:
		m.status = "the question has no brief to discuss it from"
		return nil
	}
	run, client, cfg := m.run, m.client, m.questionsCfg
	start := m.working("opening a discussion…")
	return tea.Batch(start, func() tea.Msg {
		if client == nil {
			return orchestratorMsg{}
		}
		text, err := orchestrator.Discuss(client, run, q, cfg)
		return orchestratorMsg{text: text, err: err}
	})
}

// clipboardWrite is replaced in tests.
var clipboardWrite = clipboard.WriteAll

// yank copies the selected question as the run wrote it.
func (m *Model) yank() tea.Cmd {
	q, _, ok := m.selectedQuestion()
	if !ok {
		m.status = "select a question to copy it"
		return nil
	}
	if err := clipboardWrite(q.Line); err != nil {
		m.status = "could not copy: " + err.Error()
		return nil
	}
	m.status = "copied the question"
	return nil
}

// questionRows are the rows of question i, text wide: one line with its
// chats after it, or, selected, its whole text, what is wrong with it, and
// its buttons. Its lines are q:<i> zones, so a click selects it. Its chats
// are added to m.asks.
func (m *Model) questionRows(i int, q ticket.Question, kind qKind, text int) [][]screen.Item {
	id := "q:" + strconv.Itoa(i)
	style := plainStyle
	if kind == kindClosed {
		style = dimStyle
	}
	var chats []links.Link
	if kind != kindClosed {
		chats = q.Chats()
	}
	warn := ""
	if q.Problem != "" {
		warn = " " + waitStyle.Render("!")
		if m.icons.On {
			warn = " " + waitStyle.Render(look.Warning)
		}
	}
	if !m.onQuestion || m.q != i {
		var buttons []screen.Item
		room := text - 4 - lipgloss.Width(warn)
		for _, l := range chats {
			bt := screen.Control(m.linkLabel(l.Label, 24), "ask:"+strconv.Itoa(len(m.asks)))
			m.asks = append(m.asks, l)
			buttons = append(buttons, bt)
			room -= lipgloss.Width(bt.Text) + 2
		}
		row := []screen.Item{screen.Control("  - "+screen.Inline(q.Words, style, max(room, 20))+warn, id)}
		return [][]screen.Item{append(row, buttons...)}
	}

	var rows [][]screen.Item
	for j, l := range screen.Wrap(q.Words, text-4) {
		lead := "    "
		if j == 0 {
			lead = "  " + cursorStyle.Render("❯") + " "
		}
		rows = append(rows, []screen.Item{screen.Control(lead+screen.Inline(l, style, text-4), id)})
	}
	if q.Problem != "" {
		rows = append(rows, []screen.Item{screen.Control("    "+waitStyle.Render(look.Truncate(m.icons.With(look.Warning, q.Problem), text-4)), id)})
	}
	if q.Brief != nil {
		about := "brief " + q.BriefPath
		if q.Brief.Needs != "" {
			about += " · needs " + q.Brief.Needs
		}
		rows = append(rows, []screen.Item{screen.Control("    "+dimStyle.Render(look.Truncate(about, text-4)), id)})
	}
	if kind == kindClosed {
		m.qButtons = false
		return rows
	}
	k := m.hintKey
	buttons := []screen.Item{screen.Text("  ")}
	if kind == kindForYou {
		buttons = append(buttons, screen.Button(screen.Hint{Key: k("jump"), Label: "Reply"}))
	}
	for j, l := range chats {
		if j == 0 && kind == kindOther {
			buttons = append(buttons, screen.Button(screen.Hint{Key: k("jump"), Label: m.icons.With(look.Chat, l.Label) + " ↗"}))
		} else {
			buttons = append(buttons, screen.Control(m.linkLabel(l.Label, 30), "ask:"+strconv.Itoa(len(m.asks))))
		}
		m.asks = append(m.asks, l)
	}
	if kind == kindOther {
		buttons = append(buttons, screen.Button(screen.Hint{Key: k("reply"), Label: "Reply"}))
	}
	if q.Brief != nil {
		buttons = append(buttons, screen.Button(screen.Hint{Key: k("discuss"), Label: "Discuss"}))
	}
	buttons = append(buttons, screen.Button(screen.Hint{Key: k("close"), Label: "Close"}))
	buttons = append(buttons, screen.Button(screen.Hint{Key: k("yank"), Label: "Copy"}))
	m.qButtons = true
	return append(rows, buttons)
}

// plainStyle is an open question's text: the terminal's own colour.
var plainStyle = lipgloss.NewStyle()

// linkLabel is a question's link as a button without a key, its label cut to
// n cells: the chat glyph with glyphs on, the arrow without.
func (m *Model) linkLabel(label string, n int) string {
	label = look.Truncate(label, n)
	if m.icons.On {
		return look.Button.Render(" " + look.Chat + " " + label + " ↗ ")
	}
	return look.Button.Render(" ↗ " + label + " ")
}
