// Package state is what `ekanban state` reports: one row per ticket worktree
// of a repository, with what an agent or a script needs to decide what to do
// next -- the branch against its target, the pull request, the column, the
// orchestrator and the open questions -- without opening a board.
//
// The pipeline decides the column; this package only gathers the facts beside
// it and shapes them for JSON.
package state

import (
	"context"
	"strconv"
	"strings"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Git runs git in dir and returns its trimmed output.
type Git func(dir string, args ...string) (string, error)

// Report is the whole answer.
type Report struct {
	Repo    string   `json:"repo"`
	Targets []string `json:"targets"`
	// Offline says why build server data is missing; Herdr why live agents
	// are. Both "" when nothing is missing.
	Offline string   `json:"offline,omitempty"`
	Herdr   string   `json:"herdr,omitempty"`
	Tickets []Ticket `json:"tickets"`
}

// Ticket is one ticket worktree.
type Ticket struct {
	Key      string `json:"key"`
	Title    string `json:"title,omitempty"`
	Worktree string `json:"worktree"`
	Branch   string `json:"branch,omitempty"`
	// RunDir is the run's folder, "" when the ticket has none.
	RunDir string `json:"run_dir,omitempty"`
	Git    Branch `json:"git"`

	Column      string `json:"column"`
	ColumnLabel string `json:"column_label,omitempty"`
	Phase       string `json:"phase,omitempty"`
	// Note explains a column chosen without full data.
	Note string `json:"note,omitempty"`

	PR *PR `json:"pr,omitempty"`

	Workspace    string  `json:"workspace,omitempty"`
	Orchestrator *Agent  `json:"orchestrator,omitempty"`
	Agents       []Agent `json:"agents"`
	// Waiting: an agent of the run is asking something. Lost: a role was
	// dispatched but has neither agent nor result.
	Waiting bool `json:"waiting"`
	Lost    bool `json:"lost"`

	Questions Questions `json:"questions"`
}

// Branch is the worktree's branch against its target and its remote.
type Branch struct {
	Target string `json:"target,omitempty"`
	// TargetFrom says where Target came from: "run" (the run's own field)
	// or "default" (the first configured target).
	TargetFrom string `json:"target_from,omitempty"`
	// Compared is the ref Ahead and Behind count against, "" when it could
	// not be compared (no remote-tracking branch of the target).
	Compared string `json:"compared,omitempty"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	// Dirty is how many paths have uncommitted changes.
	Dirty int `json:"dirty"`
	// Unpushed is how many commits of HEAD are on no remote branch.
	Unpushed int `json:"unpushed"`
}

// PR is the ticket's pull request.
type PR struct {
	Number   int    `json:"number"`
	Open     bool   `json:"open"`
	Build    string `json:"build,omitempty"`
	MergedTo string `json:"merged_to,omitempty"`
	Deployed string `json:"deployed,omitempty"`
	Diverged bool   `json:"diverged,omitempty"`
}

// Agent is one live agent of a run.
type Agent struct {
	Who    string `json:"who"`
	Pane   string `json:"pane"`
	Status string `json:"status"`
	Title  string `json:"title,omitempty"`
	// Completion is Herdr's id of the turn the agent last finished; it
	// changes with every finished turn, looked at or not. Absent while the
	// agent works.
	Completion uint64 `json:"completion_seq,omitempty"`
	// Role is the role's id, or "orchestrator"; absent for an orchestrator
	// found only as the workspace's one agent that plays no role.
	Role string `json:"role,omitempty"`
}

// Questions are a run's open questions, by the state file section they sit
// in: for the user, and for other people.
type Questions struct {
	ForYou []Question `json:"for_you"`
	Others []Question `json:"others"`
}

// Question is one open question.
type Question struct {
	Text string `json:"text"`
	To   string `json:"to,omitempty"`
	// Brief is the brief's path from the worktree, "" without one.
	Brief   string `json:"brief,omitempty"`
	Problem string `json:"problem,omitempty"`
}

// Row builds a ticket's row from what the pipeline classified and the git
// facts beside it.
func Row(worktree, branch, workspace string, info pipeline.Info, git Branch, cols columns.Set) Ticket {
	t := Ticket{
		Key:       info.Key,
		Title:     info.Title,
		Worktree:  worktree,
		Branch:    branch,
		RunDir:    info.RunDir,
		Git:       git,
		Column:    info.Stage,
		Phase:     info.Phase,
		Note:      info.Unknown,
		Workspace: workspace,
		Waiting:   info.Waiting,
		Lost:      info.Lost,
		Agents:    []Agent{},
		Questions: Questions{ForYou: questions(info.ForYou), Others: questions(info.Questions)},
	}
	for _, c := range cols {
		if c.ID == info.Stage {
			t.ColumnLabel = c.Label
		}
	}
	if info.PR > 0 {
		t.PR = &PR{Number: info.PR, Open: info.PROpen, Build: info.Build, MergedTo: info.MergedTo, Deployed: info.Deployed, Diverged: info.Diverged}
	}
	if info.Orchestrator.Pane != "" {
		o := agent(info.Orchestrator)
		t.Orchestrator = &o
	}
	for _, a := range info.Agents {
		t.Agents = append(t.Agents, agent(a))
	}
	return t
}

func agent(a ticket.Agent) Agent {
	return Agent{Who: a.Who, Pane: a.Pane, Status: a.Status, Title: a.Title, Completion: a.Completion, Role: a.Role}
}

func questions(qs []ticket.Question) []Question {
	out := []Question{}
	for _, q := range qs {
		out = append(out, Question{Text: q.Words, To: q.To, Brief: q.FromWorktree, Problem: q.Problem})
	}
	return out
}

// Of classifies the worktree at path the way the board does and reads its
// branch. ok is false when path holds no ticket run. pr is the pull request
// the board remembers for it, for when the build server has dropped the job.
func Of(ctx context.Context, src *pipeline.Source, live ticket.Live, path string, pr int, git Git) (Ticket, bool) {
	info := src.Classify(ctx, path, live, pr)
	if info.Key == "" {
		return Ticket{}, false
	}
	target, from := TargetOf(info.Target, src.Targets())
	return Row(path, ticket.ReadBranch(path), live.WorkspaceOf(path), info, Facts(git, path, target, from), src.Columns()), true
}

// TargetOf is the branch a worktree is compared with: the run's target when
// it names one, else the first configured target.
func TargetOf(runTarget string, targets []string) (target, from string) {
	if t := strings.TrimSpace(runTarget); t != "" {
		return t, "run"
	}
	if len(targets) > 0 {
		return targets[0], "default"
	}
	return "", ""
}

// Facts reads the worktree's branch against origin/<target>, its uncommitted
// changes and its commits on no remote. A fact git cannot answer stays zero.
func Facts(git Git, worktree, target, from string) Branch {
	b := Branch{Target: target, TargetFrom: from}
	if target != "" {
		ref := "origin/" + target
		if out, err := git(worktree, "rev-list", "--left-right", "--count", ref+"...HEAD"); err == nil {
			if behind, ahead, ok := counts(out); ok {
				b.Compared, b.Behind, b.Ahead = ref, behind, ahead
			}
		}
	}
	if out, err := git(worktree, "status", "--porcelain"); err == nil {
		b.Dirty = lines(out)
	}
	if out, err := git(worktree, "rev-list", "--count", "HEAD", "--not", "--remotes"); err == nil {
		b.Unpushed, _ = strconv.Atoi(strings.TrimSpace(out))
	}
	return b
}

// counts reads `rev-list --left-right --count A...B`: the commits only in A,
// then those only in B.
func counts(out string) (left, right int, ok bool) {
	f := strings.Fields(out)
	if len(f) != 2 {
		return 0, 0, false
	}
	l, err1 := strconv.Atoi(f[0])
	r, err2 := strconv.Atoi(f[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return l, r, true
}

func lines(out string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// Worktrees are the paths `git worktree list --porcelain` names.
func Worktrees(porcelain string) []string {
	var out []string
	for _, line := range strings.Split(porcelain, "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "worktree "); ok {
			out = append(out, path)
		}
	}
	return out
}
