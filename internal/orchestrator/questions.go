package orchestrator

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ezemacchi/ekanban/internal/ticket"
)

// A run's open questions are answered in two ways from the boards. Reply
// types a message into the orchestrator naming the question's brief, so the
// answer reaches it whatever it is doing. Discuss opens a session of its own
// on one brief, for when the person asked answers with a question back: the
// brief's needs says whether a light or a large model starts it.

// Agent is a program a board starts: a Herdr agent kind and its arguments.
type Agent struct {
	Kind string   `toml:"kind"`
	Args []string `toml:"args"`
}

// Questions is the [questions] table. The messages and the prompt are the
// project's process and may come from a repository's .ekanban.toml; Small
// and Large choose programs to start, so they are read from config.toml only.
type Questions struct {
	// Reply is what r types into the orchestrator for another's question,
	// ReplyYou for a question for the user: {to} is who answers it, {brief}
	// its brief's path from the worktree ("the question" quoted when it has
	// no brief), {question} its words.
	Reply    string `toml:"reply"`
	ReplyYou string `toml:"reply_you"`
	// Discuss is the prompt a discussion session starts from: {brief} is
	// the brief's full path; {key}, {run}, {worktree} and {state} as in the
	// orchestrator's prompt.
	Discuss string `toml:"discuss"`
	// Tab is the label a discussion's tab starts with.
	Tab string `toml:"tab"`
	// Small starts a discussion whose brief says needs: small, Large any
	// other. No kind: Discuss says what to set.
	Small Agent `toml:"small"`
	Large Agent `toml:"large"`
}

// DefaultQuestions are the messages the skills teach the orchestrator to
// read (mission-control-local, "Messages about a question").
var DefaultQuestions = Questions{
	Reply:    "Answer from {to} to the open question {brief}: ",
	ReplyYou: "My answer to the open question {brief}: ",
	Discuss:  "Read {brief} in full. It is the context of one open question on {key}. Follow its ## Kickoff.",
	Tab:      "question",
}

// WithDefaults fills what q leaves empty.
func (q Questions) WithDefaults() Questions {
	d := DefaultQuestions
	if strings.TrimSpace(q.Reply) == "" {
		q.Reply = d.Reply
	}
	if strings.TrimSpace(q.ReplyYou) == "" {
		q.ReplyYou = d.ReplyYou
	}
	if strings.TrimSpace(q.Discuss) == "" {
		q.Discuss = d.Discuss
	}
	if strings.TrimSpace(q.Tab) == "" {
		q.Tab = d.Tab
	}
	return q
}

// ReplyText is what Reply types into the orchestrator for question q,
// unsent: the user adds the answer and sends it.
func (q Questions) ReplyText(question ticket.Question) string {
	q = q.WithDefaults()
	tmpl := q.Reply
	if question.ForYou() {
		tmpl = q.ReplyYou
	}
	brief := question.FromWorktree
	if brief == "" {
		brief = `"` + question.Words + `"`
	}
	to := question.To
	if to == "" {
		to = "the person asked"
	}
	return strings.NewReplacer("{to}", to, "{brief}", brief, "{question}", question.Words).Replace(tmpl)
}

// Discuss opens a tab beside run for question q, starts in it the agent
// q's brief needs, and gives it the discussion prompt. The string says what
// happened.
func Discuss(c Herdr, run *ticket.Run, question ticket.Question, q Questions) (string, error) {
	q = q.WithDefaults()
	if question.Brief == nil {
		return "", fmt.Errorf("the question has no brief to discuss it from")
	}
	if run.Workspace == "" {
		return "", fmt.Errorf("no Herdr workspace is open on %s, so there is nowhere to open the discussion", run.Worktree)
	}
	size, agent := "large", q.Large
	if question.Brief.Needs == "small" {
		size, agent = "small", q.Small
	}
	if agent.Kind == "" {
		return "", fmt.Errorf("the brief needs a %s model; set [questions] %s in config.toml (kind and args)", size, size)
	}
	slug := strings.TrimSuffix(path.Base(filepath.ToSlash(question.BriefPath)), ".md")
	agents, err := c.Agents()
	if err != nil {
		return "", err
	}
	name := freeAgentName(strings.ToLower(run.Key)+"-q-"+slug, agents)
	tab := q.Tab + " " + slug
	_, pane, err := c.CreateTab(run.Workspace, run.Worktree, tab)
	if err != nil {
		return "", fmt.Errorf("open the discussion tab: %w", err)
	}
	if err := c.StartAgent(name, agent.Kind, pane, agent.Args); err != nil {
		return "", fmt.Errorf("start the discussion in tab %s: %w", tab, err)
	}
	prompt := strings.NewReplacer("{brief}", slash(question.BriefFile)).Replace(q.Discuss)
	file := filepath.Join(filepath.Dir(question.BriefFile), slug+"-discuss.md")
	if err := os.WriteFile(file, []byte(Expand(prompt, run)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write the discussion's prompt: %w", err)
	}
	if err := promptReady(c, name, "Read "+slash(file)+" in full and follow it. It is your complete instructions."); err != nil {
		return "", fmt.Errorf("send the discussion its prompt: %w", err)
	}
	if err := submitted(c, pane); err != nil {
		return "", fmt.Errorf("%w; press enter in its tab", err)
	}
	_ = c.FocusAgent(pane)
	return fmt.Sprintf("opened a discussion of %s (%s model) in tab %s", slug, size, tab), nil
}
