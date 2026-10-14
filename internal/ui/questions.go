package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/keys"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/orchestrator"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// A ticket whose run has open questions says so on its card, so you need not
// open each ticket's board to find out: a question for you turns the card
// red, like an agent asking, and the others' show as ?N in its border. Q
// reads them in the chooser, those for you first, the whole of the
// highlighted one beside the list. There enter opens another's chat (once you
// say yes) or replies to one for you, r (reply) types the answer's opening
// into the orchestrator, and D (discuss) starts a session on its brief.

// questionStyle is the colour of the others' open questions: they wait on
// someone, not necessarily on you, so not the red of what asks for you.
var questionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

// scopeQuestion is the scope of the actions on a question, in force while
// the questions chooser is open.
const scopeQuestion = "question"

// SetQuestions sets [questions]: what Reply types, which agents Discuss
// starts.
func (m *Model) SetQuestions(q orchestrator.Questions) { m.questionsCfg = q.WithDefaults() }

// questionsOf are the open questions for other people of the ticket in
// space key, forYouOf those for the user.
func (m *Model) questionsOf(key string) []ticket.Question {
	if !m.pipelineOn() {
		return nil
	}
	return m.pipeInfo[key].Questions
}

func (m *Model) forYouOf(key string) []ticket.Question {
	if !m.pipelineOn() {
		return nil
	}
	return m.pipeInfo[key].ForYou
}

// questionLines are the questions as their lines read.
func questionLines(qs []ticket.Question) []string {
	out := make([]string, len(qs))
	for i, q := range qs {
		out[i] = "- " + q.Line
	}
	return out
}

// questionMark is the mark a card with open questions for others carries in
// its top border: the question glyph and how many, or ?N without glyphs.
func (m *Model) questionMark(key string) string {
	n := len(m.questionsOf(key))
	if n == 0 {
		return ""
	}
	if m.icons {
		return questionStyle.Render(fmt.Sprintf("%s %d", look.Question, n))
	}
	return questionStyle.Render(fmt.Sprintf("?%d", n))
}

// questionsFact is the card's line about the others' open questions, with
// the key that reads them.
func (m *Model) questionsFact(key string) (fact, bool) {
	n := len(m.questionsOf(key))
	if n == 0 {
		return fact{}, false
	}
	text := plural(n, "question", "questions")
	if k := m.hintKey("questions"); k != "" {
		text += " · " + screen.Key(k) + " reads them"
	}
	return fact{glyph: look.Question, text: text, style: questionStyle, weight: weightContext, kind: kindTicket}, true
}

// forYouFact is the card's line about the questions for the user: they ask
// for the user like an agent waiting, so the card turns red and ! finds it.
func (m *Model) forYouFact(key string) (fact, bool) {
	n := len(m.forYouOf(key))
	if n == 0 {
		return fact{}, false
	}
	text := plural(n, "question", "questions") + " for you"
	if k := m.hintKey("questions"); k != "" {
		text += " · " + screen.Key(k)
	}
	return fact{glyph: look.Question, text: text, style: look.Attention, weight: weightYou, attention: true, kind: kindTicket}, true
}

// questionKeyMap is the key map the questions chooser reads keys with.
func (m *Model) questionKeyMap() *keys.Map {
	if km := m.keyMap(); km != nil {
		return km
	}
	km, _ := keys.New(m.defaultActions(), nil)
	return km
}

// openQuestions opens the selected ticket's open questions in the chooser:
// those for the user first, then the others'.
func (m *Model) openQuestions() tea.Cmd {
	sp := m.selected()
	if sp == nil {
		return nil
	}
	key := m.pipeInfo[sp.Key].Key
	qs := append(append([]ticket.Question(nil), m.forYouOf(sp.Key)...), m.questionsOf(sp.Key)...)
	if len(qs) == 0 {
		m.status = orDash(key) + " has no open question"
		return nil
	}
	rows := make([]screen.ChooserRow, len(qs))
	for i, q := range qs {
		mark := "  "
		switch {
		case q.Problem != "":
			mark = questionStyle.Render(orWord(m.icons, look.Warning, "!")) + " "
		case q.ForYou():
			mark = look.Attention.Render(orWord(m.icons, look.Question, "?")) + " "
		case len(q.Chats()) > 0:
			mark = look.Key.Render(orWord(m.icons, look.Chat, "↗")) + " "
		}
		rows[i] = screen.ChooserRow{Text: mark + screen.Inline(q.Words, lipgloss.NewStyle(), 0)}
	}
	km := m.questionKeyMap()
	hint := func(name, label string) screen.Hint {
		return screen.Hint{Key: km.KeyIn(name, scopeQuestion), Label: label}
	}
	m.openChooser(chooserState{
		ch: screen.Chooser{Title: orDash(key) + " · open questions", Rows: rows, Text: true,
			Buttons: []screen.Hint{{Key: "enter", Label: "Open"}, hint("reply", "Reply"), hint("discuss", "Discuss"), hint("close", "Close it"), {Key: "esc", Label: "Back"}}},
		show: func(i int) tea.Cmd {
			m.ap.ch.Preview = m.questionPreview(qs[i])
			return nil
		},
		use: func(i int) (tea.Model, tea.Cmd) {
			if qs[i].ForYou() {
				return m, m.replyTo(qs[i])
			}
			return m.openQuestionLink(qs[i])
		},
		key: func(k string, i int) (tea.Model, tea.Cmd, bool) {
			switch km.Resolve(k, scopeQuestion) {
			case "reply":
				m.mode = m.ap.back
				return m, m.replyTo(qs[i]), true
			case "discuss":
				return m, m.discussQuestion(qs[i]), true
			case "close":
				m.mode = m.ap.back
				m.startCloseQuestion(qs[i])
				return m, nil, true
			}
			return m, nil, false
		},
	})
	return m.showSelected()
}

// questionPreview is a question in full: who answers it, its words, what is
// wrong with it, its brief and the chats it carries.
func (m *Model) questionPreview(q ticket.Question) []string {
	var out []string
	switch {
	case q.ForYou():
		out = append(out, look.Attention.Render("for you"), "")
	case q.To != "":
		out = append(out, dimStyle.Render("for "+q.To), "")
	}
	out = append(out, q.Words)
	if q.Problem != "" {
		out = append(out, "", questionStyle.Render(orWord(m.icons, look.Warning, "!")+" "+q.Problem))
	}
	if q.Brief != nil {
		about := "brief " + q.FromWorktree
		if q.Brief.Needs != "" {
			about += " · needs " + q.Brief.Needs
		}
		out = append(out, "", dimStyle.Render(about))
	}
	chats := q.Chats()
	if len(chats) > 0 {
		out = append(out, "")
	}
	for i, l := range chats {
		line := "↗ " + l.Describe()
		if i == 0 {
			line += "  ⏎"
		}
		out = append(out, look.Key.Render(line))
	}
	return out
}

// openQuestionLink asks, then opens the first chat the question carries. The
// chooser stays open under the question, and comes back if the answer is no.
func (m *Model) openQuestionLink(q ticket.Question) (tea.Model, tea.Cmd) {
	back := m.mode
	m.mode = modeChooser
	chats := q.Chats()
	if len(chats) == 0 {
		m.status = "the question carries no link"
		return m, nil
	}
	l := chats[0]
	m.confirm("Open the link?", []string{l.Describe()}, func() (tea.Model, tea.Cmd) {
		m.mode = back
		m.status = "opening " + l.Label
		return m, func() tea.Msg { _ = look.OpenURL(l.URL); return nil }
	})
	return m, nil
}

// replyTo types into the ticket's orchestrator the opening of an answer to
// q, naming its brief, unsent, and goes there; with no orchestrator open,
// the agent chooser asks which agent gets it. It is m's path with the text
// already written.
func (m *Model) replyTo(q ticket.Question) tea.Cmd {
	return m.sendToAgent(m.questionsCfg.ReplyText(q))
}

// discussQuestion starts a session on q's brief, beside the selected ticket.
func (m *Model) discussQuestion(q ticket.Question) tea.Cmd {
	if q.Brief == nil {
		m.status = "the question has no brief to discuss it from"
		return nil
	}
	sp := m.selected()
	if sp == nil || m.client == nil {
		return nil
	}
	m.mode = m.ap.back
	client, src, worktree, cfg := m.client, m.pipe, sp.Key, m.questionsCfg
	start := m.working("opening a discussion…")
	return tea.Batch(start, func() tea.Msg {
		live, err := ticket.ReadLive(client)
		if err != nil {
			return orchestratorMsg{err: err}
		}
		run, err := src.Run(worktree, live)
		if err != nil {
			return orchestratorMsg{err: err}
		}
		text, err := orchestrator.Discuss(client, run, q, cfg)
		return orchestratorMsg{text: text, err: err}
	})
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
