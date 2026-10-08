// Package pipeline places each ticket worktree in a delivery column, computed
// from what already exists rather than set by hand:
//
//	todo         To Do      no run yet, or nothing dispatched
//	in_progress  Working    the team is working; the landing check has not passed
//	on_review    Reviewing  landed, pull request open, waiting on reviewers and CI
//	to_deploy    Shipping   merged into a target branch, not in its last publish yet
//	ready_qa     To QA      the target's last publish includes it
//
// The ids are fixed; the names are defaults that config.toml can change.
//
// Classify gathers Facts; the rules in rules.go decide the column. Sources
// are read only: the run's files (package ticket), the build server (a CI,
// see ci.go) and git's remote-tracking branches, with merge commits read the
// way the code host words them (host.go). Every address comes from Settings,
// filled from config.toml.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/team"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// Column ids, also used as status ids on the board.
const (
	ToDo       = "todo"
	InProgress = "in_progress"
	OnReview   = "on_review"
	ToDeploy   = "to_deploy"
	ReadyQA    = "ready_qa"
)

// DefaultColumns are the columns in order. [[pipeline.column]] in config.toml
// renames, recolours or reorders them; the ids are what rules return.
var DefaultColumns = columns.Set{
	{ID: ToDo, Label: "To Do", Color: "244", Icon: look.Circle},
	{ID: InProgress, Label: "Working", Color: "39", Icon: look.Cog},
	{ID: OnReview, Label: "Reviewing", Color: "141", Icon: look.PullReq},
	{ID: ToDeploy, Label: "Shipping", Color: "214", Icon: look.Rocket},
	{ID: ReadyQA, Label: "To QA", Color: "78", Icon: look.Check},
}

// Info is what the board shows about one ticket worktree.
type Info struct {
	Stage string
	Key   string // ticket key, from the run directory or the branch
	Title string // first line of the run's objective
	// Phase is where the team is inside In Progress ("Implementer, round 2").
	Phase   string
	Waiting bool // an agent of the run is asking something
	Lost    bool // a role was dispatched but has neither agent nor result
	// Agents are the run's live agents with their Herdr status and title.
	Agents []ticket.Agent

	PR       int
	PROpen   bool
	Build    string // last PR job result: SUCCESS, FAILURE, RUNNING, ...
	MergedTo string // master or predev
	// Deployed names the publish that includes the merge ("dev #212").
	Deployed string
	// Unknown explains a column chosen without full data (build server offline).
	Unknown string
}

// Target is a branch pull requests merge into, and the publish job that
// deploys it.
type Target struct {
	Branch  string // "master"
	Env     string // where its publish deploys, shown on the card: "dev"
	Publish string // build server job of the publish; empty: never "deployed"
}

// Settings is where to look. A nil CI means no build server: pull requests are
// found from the run and merge commits only.
type Settings struct {
	CI           CI       // nil: none
	Host         CodeHost // nil: "any"
	TicketKey    *regexp.Regexp
	Targets      []Target
	Rules        []Rule            // nil: DefaultRules
	Teams        []team.Team       // nil: the built-in teams
	Orchestrator team.Orchestrator // config.toml's [orchestrator], over each team's
	Columns      columns.Set       // nil: DefaultColumns
	Layout       *ticket.Layout    // nil: ticket.DefaultLayout
}

// CIName names the build server on the board, "CI" when there is none.
func (s *Source) CIName() string {
	if s == nil || s.set.CI == nil {
		return "CI"
	}
	return s.set.CI.Name()
}

// Columns are the board's columns in order.
func (s *Source) Columns() columns.Set {
	if s == nil || len(s.set.Columns) == 0 {
		return DefaultColumns
	}
	return s.set.Columns
}

// Source holds what was last read from the build server and git for one
// repository.
type Source struct {
	repo string
	set  Settings
	// hide is applied to every git process so none flashes a console window.
	hide func(*exec.Cmd)

	mu        sync.Mutex
	jobs      []PRBuild
	published map[string]Publish
	offline   string
	// deployed remembers publishes already seen to include a merge; a merge
	// never leaves a publish once in it.
	deployed map[int]string
}

// New reads the build server and git for repo, any checkout of the worktrees.
func New(repo string, set Settings, hide func(*exec.Cmd)) *Source {
	if hide == nil {
		hide = func(*exec.Cmd) {}
	}
	if set.Rules == nil {
		set.Rules, _ = Chain(nil)
	}
	if set.Host == nil {
		set.Host, _ = Host("")
	}
	if set.TicketKey == nil {
		set.TicketKey, _ = TicketKey("")
	}
	return &Source{
		repo:      repo,
		set:       set,
		hide:      hide,
		published: map[string]Publish{},
		deployed:  map[int]string{},
	}
}

// Refresh fetches the target branches and rereads the build server. An
// offline server is not an error: columns that need it say so instead.
func (s *Source) Refresh(ctx context.Context) {
	if branches := s.targetBranches(); len(branches) > 0 {
		s.git(ctx, append([]string{"fetch", "--quiet", "origin"}, branches...)...)
	}
	ci := s.set.CI
	if ci == nil {
		return
	}

	jobs, err := ci.PRBuilds(ctx)
	pubs := map[string]Publish{}
	if err == nil {
		for _, t := range s.set.Targets {
			if t.Publish == "" {
				continue
			}
			if p, perr := ci.LastPublish(ctx, t); perr == nil {
				pubs[t.Branch] = p
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.offline = ci.Name() + " is not answering (VPN?)"
		return
	}
	s.offline = ""
	s.jobs = jobs
	s.published = pubs
}

// Run reads worktree's run with the board's teams and orchestrator.
func (s *Source) Run(worktree string, live ticket.Live) (*ticket.Run, error) {
	return ticket.Load(worktree, ticket.Options{Teams: s.set.Teams, Orchestrator: s.set.Orchestrator, Layout: s.set.Layout}, live)
}

// Classify places one worktree. knownPR is a pull request number remembered
// from an earlier look, for when Jenkins has dropped the job of a closed one.
func (s *Source) Classify(ctx context.Context, worktree string, live ticket.Live, knownPR int) (info Info) {
	branch := ticket.ReadBranch(worktree)
	branchKey := s.keyIn(branch)
	run, err := s.Run(worktree, live)
	// Only a run named by a ticket key, and the branch's key when it has one,
	// is this ticket's run; checkouts also hold older runs of other work.
	key := s.set.TicketKey
	if err == nil && (key.FindString(run.Key) != run.Key || run.Key == "" || branchKey != "" && run.Key != branchKey) {
		run, err = nil, os.ErrNotExist
	}
	info.Key = branchKey
	if err == nil {
		info.Key = run.Key
		if len(run.Objective) > 0 {
			info.Title = run.Objective[0]
		}
		describeRun(&info, run)
	}
	if info.Key == "" {
		// Not a ticket worktree (the main checkout on master, for one).
		return info
	}

	s.mu.Lock()
	offline := s.offline
	s.mu.Unlock()

	facts := Facts{Key: info.Key, HasRun: err == nil, Offline: offline}
	if err == nil {
		facts.NotStarted, facts.Landed = notStarted(run), run.Landed
	}
	defer func() {
		facts.PR, facts.PROpen, facts.Build = info.PR, info.PROpen, info.Build
		facts.MergedTo, facts.Deployed = info.MergedTo, info.Deployed
		d := Decide(s.set.Rules, facts)
		info.Stage, info.Unknown = d.Stage, d.Note
	}()

	info.PR = knownPR
	if job, ok := s.jobFor(ctx, branch); ok {
		info.PR, info.PROpen, info.Build = job.Number, job.Open, job.Result
	}
	if info.PR == 0 && run != nil {
		info.PR = prMentioned(run)
	}
	if info.PR == 0 {
		info.PR = s.prFromMergeCommit(ctx, branch)
	}

	if info.PR > 0 {
		// The merge must contain the worktree's own commit: a run can name a
		// pull request of the branch it was stacked on.
		head := s.gitIn(ctx, worktree, "rev-parse", "HEAD")
		sha, target := s.mergeOf(ctx, info.PR)
		if sha != "" && (head == "" || !s.isAncestor(ctx, head, sha)) {
			info.PR, info.PROpen, info.Build = 0, false, ""
			sha = ""
		}
		if sha != "" {
			info.MergedTo = target
			info.PROpen = false
			info.Deployed = s.deployedIn(ctx, info.PR, sha, target)
		}
	}
	return info
}

func (s *Source) targetBranches() []string {
	var out []string
	for _, t := range s.set.Targets {
		if t.Branch != "" {
			out = append(out, t.Branch)
		}
	}
	return out
}

// targetRefs are the remote-tracking refs merges are looked for in.
func (s *Source) targetRefs() []string {
	var out []string
	for _, b := range s.targetBranches() {
		out = append(out, "origin/"+b)
	}
	return out
}

// Offline reports why build server data is missing, or "".
func (s *Source) Offline() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offline
}

func notStarted(run *ticket.Run) bool {
	for _, c := range run.Cards {
		if c.Rounds > 0 || c.Column != ticket.Pending || c.PaneID != "" {
			return false
		}
	}
	return run.OrchestratorStatus == ""
}

func describeRun(info *Info, run *ticket.Run) {
	info.Agents = run.Agents()
	if run.OrchestratorStatus == "blocked" {
		info.Waiting = true
	}
	var current *ticket.Card
	for i := range run.Cards {
		c := &run.Cards[i]
		if c.Column == ticket.Waiting {
			info.Waiting = true
		}
		if c.Stuck {
			info.Lost = true
		}
		if c.Column == ticket.Working || c.Column == ticket.Waiting {
			current = c
		}
	}
	if current == nil {
		// The next role nobody has finished is where the run stands.
		for i := range run.Cards {
			if run.Cards[i].Column != ticket.Done {
				current = &run.Cards[i]
				break
			}
		}
	}
	if current != nil {
		info.Phase = current.Role.Label
		if current.Rounds > 1 {
			info.Phase += ", round " + strconv.Itoa(current.Rounds)
		}
	}
}

func (s *Source) jobFor(ctx context.Context, branch string) (PRBuild, bool) {
	if branch == "" {
		return PRBuild{}, false
	}
	s.mu.Lock()
	jobs := append([]PRBuild(nil), s.jobs...)
	s.mu.Unlock()

	var hits []PRBuild
	for _, j := range jobs {
		if j.Branch == branch {
			hits = append(hits, j)
		}
	}
	if len(hits) == 0 {
		// Jobs built in merge mode report origin/PR-<n>; match the commit.
		if sha := s.git(ctx, "rev-parse", "--verify", "--quiet", "origin/"+branch); sha != "" {
			for _, j := range jobs {
				if j.SHA == sha {
					hits = append(hits, j)
				}
			}
		}
	}
	if len(hits) == 0 {
		return PRBuild{}, false
	}
	best := hits[0]
	for _, h := range hits[1:] {
		if h.Open && !best.Open || h.Open == best.Open && h.Number > best.Number {
			best = h
		}
	}
	return best, true
}

var prRef = regexp.MustCompile(`(?i)\bPR[- #]?(\d{2,5})\b|pull request #(\d{2,5})|pull-requests/(\d{2,5})`)

var prField = regexp.MustCompile(`(?i)^(pr|pull request|pull-request)\s*:`)

// prMentioned finds the run's own pull request in its state file: a "PR:" header
// field or the Landed section. Elsewhere the file names other pull requests
// too (overlapping branches, earlier attempts).
func prMentioned(run *ticket.Run) int {
	raw, err := os.ReadFile(run.StatePath())
	if err != nil {
		return 0
	}
	var own strings.Builder
	inLanded := false
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "## ") {
			inLanded = run.Layout().IsLandedHeading(strings.TrimSpace(line[3:]))
			continue
		}
		if inLanded || prField.MatchString(strings.TrimSpace(line)) {
			own.WriteString(line + "\n")
		}
	}
	for _, m := range prRef.FindAllStringSubmatch(own.String(), -1) {
		for _, g := range m[1:] {
			if n, err := strconv.Atoi(g); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// prFromMergeCommit finds the pull request whose merge commit names this
// branch, when the code host's subjects name it.
func (s *Source) prFromMergeCommit(ctx context.Context, branch string) int {
	refs := s.targetRefs()
	grep := s.set.Host.BranchSubject(branch)
	if branch == "" || grep == "" || len(refs) == 0 {
		return 0
	}
	out := s.git(ctx, append(append([]string{"log"}, refs...), "--format=%s", "-F", "--grep", grep, "-1")...)
	return numberIn(out)
}

// mergeOf finds the merge commit of a pull request and the branch it went into.
func (s *Source) mergeOf(ctx context.Context, pr int) (sha, target string) {
	refs := s.targetRefs()
	subjects := s.set.Host.MergeSubjects(pr)
	if len(refs) == 0 || len(subjects) == 0 {
		return "", ""
	}
	args := append(append([]string{"log"}, refs...), "--format=%H", "-F")
	for _, subject := range subjects {
		args = append(args, "--grep", subject)
	}
	out := s.git(ctx, append(args, "-1")...)
	if out == "" {
		return "", ""
	}
	sha = strings.Fields(out)[0]
	// Targets are listed most-downstream first: a merge into one that later
	// flows into another belongs to the first that contains it.
	for _, t := range s.set.Targets {
		if s.isAncestor(ctx, sha, "origin/"+t.Branch) {
			return sha, t.Branch
		}
	}
	return "", ""
}

func (s *Source) deployedIn(ctx context.Context, pr int, sha, target string) string {
	s.mu.Lock()
	if d := s.deployed[pr]; d != "" {
		s.mu.Unlock()
		return d
	}
	env := target
	for _, t := range s.set.Targets {
		if t.Branch == target && t.Env != "" {
			env = t.Env
		}
	}
	pub, ok := s.published[target]
	s.mu.Unlock()
	if !ok || pub.SHA == "" || !s.isAncestor(ctx, sha, pub.SHA) {
		return ""
	}
	d := fmt.Sprintf("%s #%d", env, pub.Number)
	s.mu.Lock()
	s.deployed[pr] = d
	s.mu.Unlock()
	return d
}

func (s *Source) isAncestor(ctx context.Context, a, b string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", s.repo, "merge-base", "--is-ancestor", a, b)
	s.hide(cmd)
	return cmd.Run() == nil
}

func (s *Source) git(ctx context.Context, args ...string) string {
	return s.gitIn(ctx, s.repo, args...)
}

func (s *Source) gitIn(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	s.hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// keyIn is the ticket key a branch names, matched as written and then in
// upper case (branch names are often lower case).
func (s *Source) keyIn(branch string) string {
	if k := s.set.TicketKey.FindString(branch); k != "" {
		return k
	}
	return s.set.TicketKey.FindString(strings.ToUpper(branch))
}
