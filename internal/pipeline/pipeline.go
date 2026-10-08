// Package pipeline places each ticket worktree in a delivery column, computed
// from what already exists rather than set by hand:
//
//	To Do           no run yet, or nothing dispatched
//	In Progress     the team is working; the landing check has not passed
//	On Review       landed, pull request open, waiting on reviewers and CI
//	To be deployed  merged into a target branch, not in its last publish yet
//	Ready for QA    the target's last publish includes it
//
// Classify gathers Facts; the rules in rules.go decide the column. Sources
// are read only: the run's files (package ticket), Jenkins, which answers
// without credentials, and git's remote-tracking branches. Every address
// comes from Settings, filled from config.toml.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ezemacchi/herdr-phin-board/internal/herdr"
	"github.com/ezemacchi/herdr-phin-board/internal/store"
	"github.com/ezemacchi/herdr-phin-board/internal/ticket"
)

// Column ids, also used as status ids on the board.
const (
	ToDo       = "todo"
	InProgress = "in_progress"
	OnReview   = "on_review"
	ToDeploy   = "to_deploy"
	ReadyQA    = "ready_qa"
)

// Statuses are the columns in order.
var Statuses = []store.Status{
	{ID: ToDo, Label: "To Do", Color: "244"},
	{ID: InProgress, Label: "In Progress", Color: "39"},
	{ID: OnReview, Label: "On Review", Color: "141"},
	{ID: ToDeploy, Label: "To be deployed", Color: "214"},
	{ID: ReadyQA, Label: "Ready for QA", Color: "78"},
}

// Info is what the board shows about one ticket worktree.
type Info struct {
	Stage string
	Key   string // Jira key, from the run directory or the branch
	Title string // first line of the run's objective
	// Phase is where the team is inside In Progress ("Implementer, vuelta 2").
	Phase   string
	Waiting bool // an agent of the run is asking something
	Lost    bool // a role was dispatched but has neither agent nor result

	PR       int
	PROpen   bool
	Build    string // last PR job result: SUCCESS, FAILURE, RUNNING, ...
	MergedTo string // master or predev
	// Deployed names the publish that includes the merge ("dev #212").
	Deployed string
	// Unknown explains a column chosen without full data (Jenkins offline).
	Unknown string
}

// Target is a branch pull requests merge into, and the publish job that
// deploys it.
type Target struct {
	Branch  string // "master"
	Env     string // where its publish deploys, shown on the card: "dev"
	Publish string // Jenkins job URL of the publish; empty: never "deployed"
}

// Settings is where to look. Empty PRJobs means no Jenkins: pull requests are
// found from the run and merge commits only.
type Settings struct {
	PRJobs  string // Jenkins multibranch job whose children are PR-<n>
	Targets []Target
	Rules   []Rule // nil: DefaultRules
}

type prJob struct {
	Number int
	Open   bool
	Result string
	Branch string
	SHA    string
}

type publish struct {
	Number int
	SHA    string
}

// Source holds what was last read from Jenkins and git for one repository.
type Source struct {
	repo string
	set  Settings
	http *http.Client
	// hide is applied to every git process so none flashes a console window.
	hide func(*exec.Cmd)

	mu        sync.Mutex
	jobs      []prJob
	published map[string]publish
	offline   string
	// deployed remembers publishes already seen to include a merge; a merge
	// never leaves a publish once in it.
	deployed map[int]string
}

// New reads Jenkins and git for repo, any checkout of the worktrees.
func New(repo string, set Settings, hide func(*exec.Cmd)) *Source {
	if hide == nil {
		hide = func(*exec.Cmd) {}
	}
	if set.Rules == nil {
		set.Rules, _ = Chain(nil)
	}
	return &Source{
		repo:      repo,
		set:       set,
		http:      &http.Client{Timeout: 20 * time.Second},
		hide:      hide,
		published: map[string]publish{},
		deployed:  map[int]string{},
	}
}

// Refresh fetches the target branches and rereads Jenkins. Offline Jenkins is
// not an error: columns that need it say so instead.
func (s *Source) Refresh(ctx context.Context) {
	if branches := s.targetBranches(); len(branches) > 0 {
		s.git(ctx, append([]string{"fetch", "--quiet", "origin"}, branches...)...)
	}
	if s.set.PRJobs == "" {
		return
	}

	jobs, err := s.readJobs(ctx)
	pubs := map[string]publish{}
	if err == nil {
		for _, t := range s.set.Targets {
			if t.Publish == "" {
				continue
			}
			if p, perr := s.readPublish(ctx, t.Publish); perr == nil {
				pubs[t.Branch] = p
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.offline = "Jenkins no responde (¿VPN?)"
		return
	}
	s.offline = ""
	s.jobs = jobs
	s.published = pubs
}

// Classify places one worktree. knownPR is a pull request number remembered
// from an earlier look, for when Jenkins has dropped the job of a closed one.
func (s *Source) Classify(ctx context.Context, worktree string, agents []herdr.Agent, knownPR int) (info Info) {
	branch := ticket.ReadBranch(worktree)
	branchKey := keyIn(branch)
	run, err := ticket.Load(worktree, "", agents, nil)
	// Only a run named by a Jira key, and the branch's key when it has one,
	// is this ticket's run; checkouts also hold older runs of other work.
	if err == nil && (!jiraKey.MatchString(run.Key) || jiraKey.FindString(run.Key) != run.Key || branchKey != "" && run.Key != branchKey) {
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
		info.PR = prMentioned(run.Dir)
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

// Offline reports why Jenkins data is missing, or "".
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
			info.Phase += ", vuelta " + strconv.Itoa(current.Rounds)
		}
	}
}

func (s *Source) jobFor(ctx context.Context, branch string) (prJob, bool) {
	if branch == "" {
		return prJob{}, false
	}
	s.mu.Lock()
	jobs := append([]prJob(nil), s.jobs...)
	s.mu.Unlock()

	var hits []prJob
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
		return prJob{}, false
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

// prMentioned finds the run's own pull request in STATE.md: a "PR:" header
// field or the Landed section. Elsewhere the file names other pull requests
// too (overlapping branches, earlier attempts).
func prMentioned(dir string) int {
	raw, err := os.ReadFile(filepath.Join(dir, "STATE.md"))
	if err != nil {
		return 0
	}
	var own strings.Builder
	inLanded := false
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "## ") {
			inLanded = strings.HasPrefix(strings.ToLower(strings.TrimSpace(line[3:])), "landed")
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

// prFromMergeCommit finds the pull request whose Bitbucket merge commit names
// this branch: "Merge pull request #<n> in SP/<repo> from <branch> to ...".
func (s *Source) prFromMergeCommit(ctx context.Context, branch string) int {
	refs := s.targetRefs()
	if branch == "" || len(refs) == 0 {
		return 0
	}
	out := s.git(ctx, append(append([]string{"log"}, refs...), "--format=%s", "-F", "--grep", "from "+branch+" to ", "-1")...)
	if m := regexp.MustCompile(`#(\d+)`).FindStringSubmatch(out); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// mergeOf finds the merge commit of a pull request and the branch it went into.
func (s *Source) mergeOf(ctx context.Context, pr int) (sha, target string) {
	refs := s.targetRefs()
	if len(refs) == 0 {
		return "", ""
	}
	// Bitbucket and GitHub merge-commit subjects.
	args := append(append([]string{"log"}, refs...), "--format=%H", "-F",
		"--grep", fmt.Sprintf("Pull request #%d:", pr),
		"--grep", fmt.Sprintf("Merge pull request #%d in ", pr),
		"--grep", fmt.Sprintf("Merge pull request #%d from ", pr), "-1")
	out := s.git(ctx, args...)
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

type jenkinsRevision struct {
	LastBuiltRevision *struct {
		SHA1   string `json:"SHA1"`
		Branch []struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"lastBuiltRevision"`
}

func revisionOf(actions []jenkinsRevision) (sha, branch string) {
	for _, a := range actions {
		if a.LastBuiltRevision != nil {
			sha = a.LastBuiltRevision.SHA1
			if len(a.LastBuiltRevision.Branch) > 0 {
				branch = strings.TrimPrefix(a.LastBuiltRevision.Branch[0].Name, "origin/")
			}
			return
		}
	}
	return
}

func (s *Source) readJobs(ctx context.Context) ([]prJob, error) {
	var body struct {
		Jobs []struct {
			Name      string `json:"name"`
			Color     string `json:"color"`
			LastBuild *struct {
				Result   string            `json:"result"`
				Building bool              `json:"building"`
				Actions  []jenkinsRevision `json:"actions"`
			} `json:"lastBuild"`
		} `json:"jobs"`
	}
	tree := "jobs[name,color,lastBuild[result,building,actions[lastBuiltRevision[SHA1,branch[name]]]]]"
	if err := s.getJSON(ctx, strings.TrimRight(s.set.PRJobs, "/")+"/api/json?tree="+tree, &body); err != nil {
		return nil, err
	}
	var jobs []prJob
	for _, j := range body.Jobs {
		n, err := strconv.Atoi(strings.TrimPrefix(j.Name, "PR-"))
		if err != nil || !strings.HasPrefix(j.Name, "PR-") {
			continue
		}
		job := prJob{Number: n, Open: j.Color != "disabled"}
		if j.LastBuild != nil {
			job.Result = j.LastBuild.Result
			if j.LastBuild.Building {
				job.Result = "RUNNING"
			}
			job.SHA, job.Branch = revisionOf(j.LastBuild.Actions)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *Source) readPublish(ctx context.Context, url string) (publish, error) {
	var body struct {
		Builds []struct {
			Number  int               `json:"number"`
			Result  string            `json:"result"`
			Actions []jenkinsRevision `json:"actions"`
		} `json:"builds"`
	}
	tree := "builds[number,result,actions[lastBuiltRevision[SHA1]]]{0,6}"
	if err := s.getJSON(ctx, strings.TrimRight(url, "/")+"/api/json?tree="+tree, &body); err != nil {
		return publish{}, err
	}
	for _, b := range body.Builds {
		if b.Result == "SUCCESS" {
			sha, _ := revisionOf(b.Actions)
			return publish{Number: b.Number, SHA: sha}, nil
		}
	}
	return publish{}, fmt.Errorf("no successful publish")
}

func (s *Source) getJSON(ctx context.Context, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from Jenkins", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

var jiraKey = regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)

func keyIn(branch string) string { return jiraKey.FindString(strings.ToUpper(branch)) }
