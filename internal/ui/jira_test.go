package ui

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ezemacchi/ekanban/internal/jira"
	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/screen"
	"github.com/ezemacchi/ekanban/internal/store"
)

// jiraBoard is a pipeline board with Jira on and three listed tickets: one
// not started, one in implementation, one done. ABC-1 also has a worktree.
func jiraBoard(t *testing.T) *Model {
	t.Helper()
	t.Setenv("EK_TEST_MAIL", "a@b")
	t.Setenv("EK_TEST_TOKEN", "t")
	cfg := jira.Config{
		URL: "https://site.example", EmailEnv: "EK_TEST_MAIL", TokenEnv: "EK_TEST_TOKEN",
		Handoff: jira.Handoff{Command: []string{"handoff", "-Key", "{key}", "-Target", "{target}", "-Team", "{team}", "-Type", "{type}"}, Teams: []string{"standalone", "full-team"}},
	}
	client, why := jira.New(cfg)
	if client == nil {
		t.Fatal(why)
	}
	m := pipelineBoard(t)
	m.SetJira(client, client.Config(), []string{"predev", "master"}, "", "", nil)
	send(t, m, jiraMsg{issues: []jira.Issue{
		{Key: "ABC-1", Summary: "Fix the grid", Status: "In Quality Review", Category: "indeterminate", Type: "Story"},
		{Key: "ABC-2", Summary: "Show the dialog", Status: "Ready", Category: "new", Type: "Story"},
		{Key: "ABC-3", Summary: "Wrong total", Status: "In Implementation", Category: "indeterminate", Type: "Bug"},
		{Key: "ABC-4", Summary: "Old thing", Status: "Closed", Category: "done", Type: "Story"},
	}})
	return m
}

func selectJiraCard(t *testing.T, m *Model, ticket string) {
	t.Helper()
	for col, st := range m.board.Statuses {
		for i, sp := range m.groups[st.ID] {
			if sp.Key == jiraPrefix+ticket {
				m.col, m.rowInCol = col, i
				return
			}
		}
	}
	t.Fatalf("no card for %s", ticket)
}

func cardsIn(m *Model, column string) []string {
	var out []string
	for _, sp := range m.groups[column] {
		out = append(out, m.ticketOf(sp))
	}
	return out
}

// A ticket with no worktree sits where its Jira status puts it: not started
// in To Do, in implementation in Working, and a done one is left off. A ticket
// a worktree already holds is not listed twice.
func TestJiraTicketsAreCardsPlacedByStatus(t *testing.T) {
	m := jiraBoard(t)
	if got := cardsIn(m, pipeline.ToDo); !equal(got, []string{"ABC-2"}) {
		t.Fatalf("To Do holds %v", got)
	}
	if got := cardsIn(m, pipeline.InProgress); !equal(got, []string{"ABC-3"}) {
		t.Fatalf("Working holds %v", got)
	}
	for _, st := range m.board.Statuses {
		for _, k := range cardsIn(m, st.ID) {
			if k == "ABC-4" {
				t.Fatalf("a done ticket is on the board, in %s", st.ID)
			}
		}
	}
	if got := cardsIn(m, pipeline.ReadyQA); !equal(got, []string{"ABC-1"}) {
		t.Fatalf("To QA holds %v; the worktree's ticket must show once, in its own column", got)
	}
}

// A card says what Jira says, so an In Implementation ticket is never
// mistaken for one that was not started.
//
// At rest a ticket with no worktree says how to start it; selected, a card
// shows its Jira status too.
func TestCardsShowTheJiraStatus(t *testing.T) {
	m := jiraBoard(t)
	m.width = 220
	if out := ansi.Strip(m.View()); !strings.Contains(out, "no worktree — press H") {
		t.Fatalf("board is missing how to start a ticket:\n%s", out)
	}
	for _, c := range []struct{ key, want string }{{"ABC-2", "Jira: Ready"}, {"ABC-3", "Jira: In Implementation"}} {
		selectJiraCard(t, m, c.key)
		if out := ansi.Strip(m.View()); !strings.Contains(out, c.want) {
			t.Fatalf("selected %s is missing %q:\n%s", c.key, c.want, out)
		}
	}
	selectSpace(t, m, tmp+"api")
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Jira: In Quality Review") {
		t.Fatalf("the worktree's card is missing its Jira status:\n%s", out)
	}
}

// What the configuration says about a status wins over its category.
func TestConfiguredStatusMovesATicket(t *testing.T) {
	m := jiraBoard(t)
	m.jr.cfg.Columns = map[string]string{"In Implementation": "on_review", "Ready": ""}
	m.rebuild()
	if got := cardsIn(m, pipeline.OnReview); !equal(got, []string{"ABC-3"}) {
		t.Fatalf("Reviewing holds %v", got)
	}
	if got := cardsIn(m, pipeline.ToDo); len(got) != 0 {
		t.Fatalf("a ticket mapped to nothing is on the board: %v", got)
	}
}

// A Jira card has no folder: the actions that need one say so instead of
// opening a workspace named after a ticket.
func TestJiraCardRefusesWhatNeedsAWorktree(t *testing.T) {
	m := jiraBoard(t)
	selectJiraCard(t, m, "ABC-2")
	for _, k := range []string{"enter", "R", "m", "x", "a", "o"} {
		if _, cmd := m.Update(key(k)); cmd != nil && k != "o" {
			t.Fatalf("%s did something on a ticket with no worktree", k)
		}
		if got := screen.Plain(m.status); !strings.Contains(got, "no worktree yet") && k != "a" {
			t.Fatalf("%s: status %q", k, got)
		}
		if m.mode != modeNormal {
			t.Fatalf("%s opened mode %v", k, m.mode)
		}
	}
}

// H asks for the target and the team, shows the command, and runs it only on
// the last enter. A single choice is made alone.
func TestHandoffAsksTargetThenTeamThenConfirms(t *testing.T) {
	m := jiraBoard(t)
	selectJiraCard(t, m, "ABC-3")
	send(t, m, key("H"))
	if m.mode != modeHandoff {
		t.Fatalf("H did not open the handoff, status %q", m.status)
	}
	m.width, m.height = 100, 30
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Hand off ABC-3") || !strings.Contains(out, "1 predev") || !strings.Contains(out, "2 master") {
		t.Fatalf("target step:\n%s", out)
	}
	send(t, m, key("2"))
	if out := ansi.Strip(m.View()); !regexp.MustCompile(`TARGET +master`).MatchString(out) || !strings.Contains(out, "standalone") {
		t.Fatalf("team step:\n%s", out)
	}
	send(t, m, key("enter")) // first team: standalone
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "handoff -Key ABC-3 -Target master -Team standalone -Type fix") {
		t.Fatalf("confirm step does not show the command, a bug being a fix:\n%s", out)
	}
	if _, cmd := m.Update(key("esc")); cmd != nil || m.mode != modeNormal {
		t.Fatal("esc did not cancel")
	}

	// One target and one team need no questions.
	m.jr.targets, m.jr.cfg.Handoff.Teams = []string{"master"}, []string{"full-team"}
	send(t, m, key("H"))
	if got := m.jr.handoff; got.step != 2 || got.target != "master" || got.team != "full-team" {
		t.Fatalf("single choices were not taken: %+v", got)
	}
	if _, cmd := m.Update(key("enter")); cmd == nil || m.mode != modeNormal {
		t.Fatal("enter on the confirm step did not run the handoff")
	}
}

func TestHandoffNeedsAWorktreelessTicketAndACommand(t *testing.T) {
	m := jiraBoard(t)
	selectSpace(t, m, tmp+"api")
	send(t, m, key("H"))
	if m.mode != modeNormal || !strings.Contains(screen.Plain(m.status), "already has a worktree") {
		t.Fatalf("a worktree's ticket was handed off: %q", m.status)
	}
	selectJiraCard(t, m, "ABC-2")
	m.jr.cfg.Handoff.Command = nil
	send(t, m, key("H"))
	if m.mode != modeNormal || !strings.Contains(m.status, "[jira.handoff] command") {
		t.Fatalf("no command, status %q", m.status)
	}
}

func TestHandoffResultReadsTheScriptsJSON(t *testing.T) {
	text, ok := handoffResult([]byte("noise\n{\"code\":0,\"message\":\"workspace wK ready\"}\n"), nil)
	if !ok || text != "workspace wK ready" {
		t.Fatalf("%q %v", text, ok)
	}
	text, ok = handoffResult([]byte("{\"code\":3,\"message\":\"a worktree exists\"}\n"), errors.New("exit status 3"))
	if ok || text != "a worktree exists" {
		t.Fatalf("%q %v", text, ok)
	}
	text, ok = handoffResult([]byte("boom\n"), errors.New("exit status 1"))
	if ok || !strings.Contains(text, "boom") {
		t.Fatalf("%q %v", text, ok)
	}
}

func TestHandoffEnvSaysItRunsInsideHerdr(t *testing.T) {
	got := strings.Join(handoffEnv([]string{"PATH=x"}), "\n")
	if !strings.Contains(got, "HERDR_ENV=1") {
		t.Fatalf("%s", got)
	}
	if got := strings.Join(handoffEnv([]string{"HERDR_ENV=1", "HERDR_BIN_PATH=C:/h"}), "\n"); strings.Count(got, "HERDR_ENV") != 1 {
		t.Fatalf("duplicated: %s", got)
	}
}

// One key opens the pull request of the card, from pipeline.pr_url: the
// pipeline board has no GitHub cache to read it from.
func TestPullRequestKeyOpensTheCardsPullRequest(t *testing.T) {
	links.Configure("", "https://git.example/pr/{pr}")
	t.Cleanup(func() { links.Configure("", "") })
	m := pipelineBoard(t)
	selectSpace(t, m, tmp+"api")
	if _, cmd := m.Update(key("P")); cmd == nil || !strings.Contains(m.status, "opening PR #5") {
		t.Fatalf("P did not open the pull request: %q", m.status)
	}
	if m.hintKey("open-pull-request") != "P" {
		t.Fatal("the footer does not name the key")
	}
	m.layout = layoutKanban
	m.width = 200
	if out := ansi.Strip(m.View()); !strings.Contains(out, "P PR") {
		t.Fatalf("no button for it:\n%s", out)
	}

	// The same through the chord that was already documented.
	m.status = ""
	send(t, m, key("g"))
	if _, cmd := m.Update(key("p")); cmd == nil || !strings.Contains(m.status, "opening PR #5") {
		t.Fatalf("gp: %q", m.status)
	}

	links.Configure("", "")
	send(t, m, key("P"))
	if !strings.Contains(m.status, "pipeline.pr_url is missing") {
		t.Fatalf("no pr_url: %q", m.status)
	}
}

func TestPullRequestKeyWithoutOneSaysSo(t *testing.T) {
	m := jiraBoard(t)
	selectJiraCard(t, m, "ABC-2")
	send(t, m, key("P"))
	if got := m.status; !strings.Contains(got, "no pull request for ABC-2 yet") {
		t.Fatalf("status %q", got)
	}
}

func TestYankTextCarriesTheContext(t *testing.T) {
	d := jira.Detail{
		Tests:    []jira.Link{{Key: "ABC-8", Summary: "Shows the dialog", Status: "Defined"}},
		Comments: []jira.Comment{{Author: "Bo", At: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC), Body: "Failed:\n\n| TC | Issue |\n"}},
	}
	text := yankText(yankFacts{
		Key: "ABC-7", Issue: jira.Issue{Summary: "Concurrent preparers", Status: "In Implementation", Type: "Story"}, HasIssue: true,
		Detail: &d, IssueURL: "https://site.example/browse/ABC-7", Column: "Working",
		Info:     pipeline.Info{PR: 9, Build: "FAILURE", MergedTo: ""},
		PRURL:    "https://git.example/pr/9",
		Worktree: `C:\repos\App-wt-ABC-7`, Branch: "feat/ABC-7-x", CI: "Jenkins", Note: "waiting on QA",
	})
	for _, want := range []string{
		"ABC-7 · Concurrent preparers",
		"Jira: https://site.example/browse/ABC-7 · In Implementation · Story",
		"Board column: Working",
		`Worktree: C:\repos\App-wt-ABC-7`,
		"Branch: feat/ABC-7-x",
		"Pull request: #9 https://git.example/pr/9 · Jenkins failure",
		"Note: waiting on QA",
		"Linked tests (1):\n- ABC-8 Shows the dialog [Defined]",
		"Latest comment, Bo,",
		"| TC | Issue |",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("copied text is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\n\n\n") || strings.Contains(text, "Shipped to") {
		t.Fatalf("empty facts leaked into the text:\n%s", text)
	}
}

// y copies a ticket with no worktree too: it is how QA's findings reach a new
// session.
func TestYankCopiesAJiraCard(t *testing.T) {
	var copied string
	clipboardWrite = func(s string) error { copied = s; return nil }
	t.Cleanup(func() { clipboardWrite = nil })

	m := jiraBoard(t)
	m.jr.client = nil // no network in tests: the held detail is used
	m.jr.detail["ABC-3"] = jira.Detail{Comments: []jira.Comment{{Author: "Bo", Body: "Total is off by one"}}}
	selectJiraCard(t, m, "ABC-3")
	_, cmd := m.Update(key("y"))
	if cmd == nil {
		t.Fatal("y did nothing")
	}
	// The command batches the spinner and the copy; run them for their effect.
	run(cmd)
	if !strings.Contains(copied, "ABC-3 · Wrong total") || !strings.Contains(copied, "Total is off by one") || strings.Contains(copied, "Worktree:") {
		t.Fatalf("copied:\n%s", copied)
	}
}

func TestYankIsOnTheHelpAndRebinds(t *testing.T) {
	m := pipelineBoard(t)
	m.SetKeys(map[string][]string{"yank": {"Y"}})
	selectSpace(t, m, tmp+"api")
	m.layout = layoutKanban
	m.width = 220
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Y Copy") {
		t.Fatalf("footer does not name the rebound key:\n%s", out)
	}
	if k := m.keyMap().Resolve("y"); k != "" {
		t.Fatalf("y still answers after being rebound: %q", k)
	}
}

var _ = store.Key

// run executes a command and the ones it batches, for tests that care about
// a command's effect rather than the message it sends.
func run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			run(c)
		}
	}
}

// t opens the card's ticket in Jira, on a card with a worktree and on one
// without: the ticket is all a Jira-only card has.
func TestTicketKeyOpensTheCardsJiraTicket(t *testing.T) {
	links.Configure("https://site.example/browse/{key}", "")
	t.Cleanup(func() { links.Configure("", "") })
	var opened []string
	original := openURL
	openURL = func(url string) error { opened = append(opened, url); return nil }
	t.Cleanup(func() { openURL = original })

	m := jiraBoard(t)
	for ticket, want := range map[string]string{"ABC-1": "https://site.example/browse/ABC-1", "ABC-2": "https://site.example/browse/ABC-2"} {
		if ticket == "ABC-1" {
			selectSpace(t, m, tmp+"api") // the card a worktree holds
		} else {
			selectJiraCard(t, m, ticket)
		}
		_, cmd := m.Update(key("t"))
		if cmd == nil || !strings.Contains(m.status, "opening "+ticket) {
			t.Fatalf("t on %s: status %q", ticket, m.status)
		}
		run(cmd)
		if got := opened[len(opened)-1]; got != want {
			t.Fatalf("t on %s opened %q, want %q", ticket, got, want)
		}
	}

	links.Configure("", "")
	send(t, m, key("t"))
	if !strings.Contains(m.status, "issue_url is missing") {
		t.Fatalf("no issue_url: %q", m.status)
	}
}

// The detail modal answers t and p as the board does.
func TestTicketAndPrototypeKeysWorkInTheDetail(t *testing.T) {
	links.Configure("https://site.example/browse/{key}", "")
	t.Cleanup(func() { links.Configure("", "") })
	m := jiraBoard(t)
	selectJiraCard(t, m, "ABC-2")
	send(t, m, key("d"))
	if m.mode != modeDetail {
		t.Fatalf("d opened mode %v", m.mode)
	}
	if _, cmd := m.Update(key("t")); cmd == nil || !strings.Contains(m.status, "opening ABC-2") {
		t.Fatalf("t in the detail: %q", m.status)
	}
	m.width = 200
	if out := ansi.Strip(m.View()); !strings.Contains(out, "t Ticket") {
		t.Fatalf("detail is missing the ticket button:\n%s", out)
	}
	send(t, m, key("p"))
	if !strings.Contains(m.status, "ABC-2 has no worktree yet") {
		t.Fatalf("p in the detail on a card with no worktree: %q", m.status)
	}
}

// p opens the prototype the run names; with none, it says which spec has none.
func TestPrototypeKeyOpensThePrototypeOrSaysThereIsNone(t *testing.T) {
	var opened []string
	original := openURL
	openURL = func(url string) error { opened = append(opened, url); return nil }
	t.Cleanup(func() { openURL = original })

	m := jiraBoard(t)
	selectJiraCard(t, m, "ABC-2")
	if _, cmd := m.Update(key("p")); cmd != nil || !strings.Contains(m.status, "ABC-2 has no worktree yet") {
		t.Fatalf("p on a card with no worktree: %q", m.status)
	}

	_, cmd := m.Update(prototypeMsg{page: `C:\spec\E7_US_42\index.html`, spec: "E7_US_42"})
	if cmd == nil || m.status != "opening the prototype" {
		t.Fatalf("a found prototype: %q", m.status)
	}
	run(cmd)
	if len(opened) != 1 || opened[0] != `C:\spec\E7_US_42\index.html` {
		t.Fatalf("opened %v", opened)
	}

	if _, cmd := m.Update(prototypeMsg{spec: "E7_US_42"}); cmd != nil || m.status != "no prototype found for E7_US_42" {
		t.Fatalf("no prototype: %q", m.status)
	}
	if _, cmd := m.Update(prototypeMsg{err: errors.New("no run found in x")}); cmd != nil || m.status != "no run found in x" {
		t.Fatalf("no run: %q", m.status)
	}
}

// The footer and the help name t and p, and they rebind like any action.
func TestTicketAndPrototypeAreOnTheFooterAndHelp(t *testing.T) {
	m := pipelineBoard(t)
	m.SetKeys(map[string][]string{"prototype": {"X"}})
	selectSpace(t, m, tmp+"api")
	m.layout = layoutKanban
	m.width = 240
	out := ansi.Strip(m.View())
	for _, want := range []string{"t Ticket", "P PR"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the card is missing %q:\n%s", want, out)
		}
	}
	send(t, m, key("."))
	if menu := ansi.Strip(m.View()); !regexp.MustCompile(`Open the prototype +X`).MatchString(menu) {
		t.Fatalf("the card's menu does not offer the prototype on X:\n%s", menu)
	}
	send(t, m, key("esc"))
	if k := m.keyMap().Resolve("p"); k != "" {
		t.Fatalf("p still answers after being rebound: %q", k)
	}
	send(t, m, key("?"))
	help := ansi.Strip(m.View())
	for _, want := range []string{"open the ticket in Jira", "open the ticket's HTML prototype"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help is missing %q:\n%s", want, help)
		}
	}
}
