// Package ticket reads one team run from its worktree, so the board can show
// which role is doing what. The team and its roles come from package team.
//
// Runs started by different versions of the local skills write STATE.md
// differently, so nothing here depends on one layout. A role's state comes
// from what every run leaves behind: its dispatch files (was it sent, how many
// times), its result file (did it finish), and the live Herdr agents working
// in the worktree (is it running or asking something).
package ticket

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/links"
	"github.com/ezemacchi/ekanban/internal/look"
	"github.com/ezemacchi/ekanban/internal/team"
)

// Column is a kanban column.
type Column int

const (
	Pending Column = iota
	Working
	Waiting
	Done
)

var columnIDs = map[Column]string{Pending: "pending", Working: "working", Waiting: "waiting", Done: "done"}

// ID is the column's id in config.toml's [[ticket.column]].
func (c Column) ID() string { return columnIDs[c] }

// ColumnByID is the column with id, false when there is none.
func ColumnByID(id string) (Column, bool) {
	for c, cid := range columnIDs {
		if cid == id {
			return c, true
		}
	}
	return 0, false
}

// DefaultColumns are the ticket board's columns in display order.
// [[ticket.column]] renames, reorders or changes their icons; a role can only
// be in one of these four, so no column can be added.
var DefaultColumns = columns.Set{
	{ID: "pending", Label: "Pending", Icon: look.Circle},
	{ID: "working", Label: "Working", Icon: look.Cog},
	{ID: "waiting", Label: "Waiting on you", Icon: look.Question},
	{ID: "done", Label: "Done", Icon: look.Check},
}

// done reports whether role left its result: the team definition says how.
func (r *Run) done(role team.Role) bool {
	if role.DoneFile != "" {
		if _, err := os.Stat(filepath.Join(r.Dir, role.DoneFile)); err == nil {
			return true
		}
	}
	if role.DoneField != "" && r.Field(role.DoneField) != "" {
		return true
	}
	return role.DoneWhenLanded && r.Landed
}

// Options are what Load needs besides the worktree.
type Options struct {
	// SpecRoot is the specifications clone prototypes are looked up in;
	// empty skips them.
	SpecRoot string
	// Teams are the team definitions; nil means the built-in ones.
	Teams []team.Team
	// Lead overrides fields of the team's [lead]: config.toml's [lead].
	Lead team.Lead
	// Layout is where runs live and how their state file reads; the zero
	// value is DefaultLayout.
	Layout *Layout
}

// RunLayout is Layout, or DefaultLayout when it is not set.
func (o Options) RunLayout() Layout {
	if o.Layout == nil {
		return DefaultLayout()
	}
	return *o.Layout
}

// Live is what Herdr shows right now.
type Live struct {
	Agents []herdr.Agent
	Tabs   map[string]string // tab id -> label
	// Workspaces maps a workspace id to the checkout it was opened on.
	Workspaces map[string]string
}

// ReadLive asks Herdr for its agents, workspaces and tab labels.
func ReadLive(c *herdr.Client) (Live, error) {
	live := Live{Tabs: map[string]string{}, Workspaces: map[string]string{}}
	agents, err := c.Agents()
	if err != nil {
		return live, err
	}
	live.Agents = agents
	if ws, err := c.Workspaces(); err == nil {
		for _, w := range ws {
			live.Workspaces[w.ID] = w.Checkout()
		}
	}
	asked := map[string]bool{}
	for _, a := range agents {
		if asked[a.WorkspaceID] {
			continue
		}
		asked[a.WorkspaceID] = true
		if tabs, err := c.Tabs(a.WorkspaceID); err == nil {
			for _, t := range tabs {
				live.Tabs[t.ID] = t.Label
			}
		}
	}
	return live, nil
}

// WorkspaceOf is the open workspace of worktree, or "".
func (l Live) WorkspaceOf(worktree string) string {
	ids := make([]string, 0, len(l.Workspaces))
	for id := range l.Workspaces {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if l.Workspaces[id] != "" && samePath(l.Workspaces[id], worktree) {
			return id
		}
	}
	return ""
}

// AgentPanes are the panes of every agent, for status subscriptions.
func (l Live) AgentPanes() []string {
	var out []string
	for _, a := range l.Agents {
		if a.Agent != nil {
			out = append(out, a.PaneID)
		}
	}
	sort.Strings(out)
	return out
}

// Card is one role on the board.
type Card struct {
	Role   team.Role
	Column Column
	Rounds int
	Note   string
	// Stuck marks a role that was dispatched but has neither an agent nor a
	// result, usually because the agent was lost.
	Stuck bool
	// PaneID is the role's live agent, when there is one; Status and Title
	// are its Herdr status and terminal title.
	PaneID string
	Status string
	Title  string
}

// Agent is one live agent of the run, the lead or a role's.
type Agent struct {
	Who    string // the role's or the lead's label
	Pane   string
	Status string
	Title  string
}

// Change is an agent whose status moved since the last reading; From is ""
// for an agent that was not there.
type Change struct {
	Agent
	From string
}

// Changes compares now with the statuses seen before, by pane, and records
// now in seen. A nil seen is the first reading: it is filled and reports
// nothing, so opening a board does not announce every agent.
func Changes(seen map[string]string, now []Agent) (map[string]string, []Change) {
	first := seen == nil
	next := make(map[string]string, len(now))
	var out []Change
	for _, a := range now {
		next[a.Pane] = a.Status
		from, ok := seen[a.Pane]
		// done to idle is only the user looking at the finished work.
		if from == "done" && a.Status == "idle" {
			continue
		}
		if !first && (!ok || from != a.Status) {
			out = append(out, Change{a, from})
		}
	}
	return next, out
}

// Says is the change in a few words: "Reviewer finished".
func (c Change) Says() string {
	if c.From == "" {
		return c.Who + " started"
	}
	if w := look.Agent(c.Status).Word; w != "" {
		return c.Who + " " + w
	}
	return c.Who + " " + c.Status
}

// Loudest is the agent asking most of the user (look.AgentRank), the first
// one on a tie; false when none is working, blocked or done.
func Loudest(agents []Agent) (Agent, bool) {
	var top Agent
	for _, a := range agents {
		if look.AgentRank(a.Status) > look.AgentRank(top.Status) {
			top = a
		}
	}
	return top, look.AgentRank(top.Status) > look.AgentRank("idle")
}

// Agents are the run's live agents: the lead first, then the roles in order.
func (r *Run) Agents() []Agent {
	var out []Agent
	if r.LeadPane != "" {
		out = append(out, Agent{r.Lead.Label, r.LeadPane, r.LeadStatus, r.LeadTitle})
	}
	for _, c := range r.Cards {
		if c.PaneID != "" {
			out = append(out, Agent{c.Role.Label, c.PaneID, c.Status, c.Title})
		}
	}
	return out
}

// Run is everything the ticket board shows.
type Run struct {
	Key      string
	Dir      string // .runs/<KEY>
	Worktree string
	Branch   string
	JiraURL  string
	Team     string // the run's "Team:" field
	TeamName string // the team definition it picked
	Target   string
	// Spec is the story code (E7_US_42) that names the spec in the specifications clone;
	// Prototype is its HTML prototype there, when one exists.
	Spec      string
	Prototype string

	Objective   []string
	CurrentStep []string
	Questions   []string
	Landed      bool

	// Lead is the team's lead definition; LeadStatus and LeadPane are its live
	// agent, empty when none is open.
	Lead       team.Lead
	LeadStatus string
	LeadPane   string
	LeadTitle  string
	// Workspace is the Herdr workspace open on the worktree, or "".
	Workspace string

	Cards []Card

	fields   map[string]string
	sections map[string][]string
	layout   Layout
}

// Field returns a header field of the state file ("Team", "Target", ...).
func (r *Run) Field(name string) string { return r.fields[strings.ToLower(name)] }

// StatePath is the run's state file.
func (r *Run) StatePath() string { return filepath.Join(r.Dir, r.Layout().State) }

// Layout is the layout the run was read with; DefaultLayout for a Run that
// was not read by Load.
func (r *Run) Layout() Layout {
	if r.layout.landed == nil {
		return DefaultLayout()
	}
	return r.layout
}

// Find locates the run directory for a worktree, <dir>/<KEY>/<state> in the
// layout, picking the key named in the branch when several runs share the
// worktree.
func Find(worktree string, l Layout) (dir, key string, ok bool) {
	matches, _ := filepath.Glob(filepath.Join(worktree, l.Dir, "*", l.State))
	if len(matches) == 0 {
		return "", "", false
	}
	branch := ReadBranch(worktree)
	for _, m := range matches {
		k := filepath.Base(filepath.Dir(m))
		if branch != "" && strings.Contains(strings.ToUpper(branch), strings.ToUpper(k)) {
			return filepath.Dir(m), k, true
		}
	}
	sort.Slice(matches, func(i, j int) bool { return modTime(matches[i]) > modTime(matches[j]) })
	return filepath.Dir(matches[0]), filepath.Base(filepath.Dir(matches[0])), true
}

// Load reads the run and places each role of its team on the board.
func Load(worktree string, opts Options, live Live) (*Run, error) {
	l := opts.RunLayout()
	dir, key, ok := Find(worktree, l)
	if !ok {
		return nil, os.ErrNotExist
	}
	text, err := os.ReadFile(filepath.Join(dir, l.State))
	if err != nil {
		return nil, err
	}
	r := &Run{
		Key:      key,
		Dir:      dir,
		Worktree: worktree,
		Branch:   ReadBranch(worktree),
		JiraURL:  links.Issue(key),
		layout:   l,
	}
	r.parse(string(text))
	r.Team = r.Field(l.TeamField)
	r.Target = r.Field(l.TargetField)
	r.Spec = l.normalizeSpec(r.Field(l.SpecField))
	if r.Spec == "" {
		r.Spec = l.mostMentionedSpec(dir)
	}
	r.Prototype = findPrototype(opts.SpecRoot, l.prototypeDir(r.Spec), r.Spec)
	r.Objective = firstLines(r.section(l.objective), 3)
	r.CurrentStep = firstLines(r.section(l.currentStep), 4)
	r.Questions = r.openQuestions()
	landed := strings.Join(r.section(l.landed), "\n")
	// The landing check writes its report here whether it passed or not.
	r.Landed = strings.TrimSpace(landed) != "" && !l.notLanded.MatchString(landed)

	teams := opts.Teams
	if teams == nil {
		teams, _ = team.Load("")
	}
	t, _ := team.Pick(teams, r.Team)
	r.TeamName = t.Name
	r.Lead, _ = t.Lead.With(opts.Lead)
	r.Workspace = live.WorkspaceOf(worktree)

	mine := agentsIn(worktree, r.Workspace, live.Agents)
	r.placeLead(t.Roles, mine, live.Tabs)
	r.placeRoles(t.Roles, mine, live.Tabs)
	return r, nil
}

func (r *Run) parse(text string) {
	r.fields = map[string]string{}
	r.sections = map[string][]string{}
	heading := ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "## ") {
			heading = strings.TrimSpace(line[3:])
			r.sections[heading] = nil
			continue
		}
		if heading == "" {
			// Header: either one field per line, or "A: x | B: y" on one line.
			for _, part := range strings.Split(line, " | ") {
				// Field names are short ("Start commit"); longer text with a
				// colon is prose, not a field.
				if k, v, found := strings.Cut(part, ":"); found && len(strings.TrimSpace(k)) <= 14 {
					r.fields[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
				}
			}
			continue
		}
		r.sections[heading] = append(r.sections[heading], line)
	}
}

// section returns the lines of the first section, by heading in order, whose
// heading matches.
func (r *Run) section(re *regexp.Regexp) []string {
	headings := make([]string, 0, len(r.sections))
	for h := range r.sections {
		headings = append(headings, h)
	}
	sort.Strings(headings)
	for _, h := range headings {
		if re.MatchString(h) {
			return r.sections[h]
		}
	}
	return nil
}

// openQuestions collects the bullets of every "open" section that are not
// already marked settled. It is a hint for the person, not a verdict.
func (r *Run) openQuestions() []string {
	var out []string
	headings := make([]string, 0, len(r.sections))
	for h := range r.sections {
		headings = append(headings, h)
	}
	sort.Strings(headings)
	for _, h := range headings {
		if !r.layout.questions.MatchString(h) || r.layout.notQuestion.MatchString(h) {
			continue
		}
		for _, l := range r.sections[h] {
			t := strings.TrimSpace(l)
			if !strings.HasPrefix(t, "- ") || r.layout.settled.MatchString(t) {
				continue
			}
			out = append(out, strings.TrimSpace(t[2:]))
		}
	}
	return out
}

// agentsIn are the ticket's agents: those in its workspace, wherever their
// folder is, and those working inside the worktree from another workspace.
func agentsIn(worktree, workspace string, agents []herdr.Agent) []herdr.Agent {
	var out []herdr.Agent
	for _, a := range agents {
		if a.Agent == nil {
			continue
		}
		if workspace != "" && a.WorkspaceID == workspace || a.Cwd != "" && within(a.Cwd, worktree) {
			out = append(out, a)
		}
	}
	return out
}

func cleanPath(p string) string {
	return strings.ToLower(filepath.Clean(strings.TrimPrefix(p, `\\?\`)))
}

func samePath(a, b string) bool { return cleanPath(a) == cleanPath(b) }

func within(path, root string) bool {
	p, r := cleanPath(path), cleanPath(root)
	return p == r || strings.HasPrefix(p, r+string(filepath.Separator))
}

// titleOf is the agent's terminal title, or "" when it only names the agent
// program ("Cursor Agent") and so says nothing about the work.
func titleOf(a herdr.Agent) string {
	t := strings.TrimSpace(a.Title)
	if a.Agent != nil && (strings.EqualFold(t, *a.Agent) || strings.EqualFold(t, *a.Agent+" agent")) {
		return ""
	}
	return t
}

func describe(a herdr.Agent, tabLabels map[string]string) string {
	return strings.ToLower(a.Name + " " + tabLabels[a.TabID])
}

// placeLead finds the lead among the ticket's agents: one its match names,
// else one named after the ticket, else one in the ticket's workspace that is
// no role's (a lead started by hand, in a tab nobody renamed).
func (r *Run) placeLead(roles []team.Role, agents []herdr.Agent, tabLabels map[string]string) {
	key := strings.ToLower(r.Key)
	isRole := func(a herdr.Agent) bool {
		d := describe(a, tabLabels)
		for _, role := range roles {
			if role.Matches(d) {
				return true
			}
		}
		return false
	}
	tests := []func(herdr.Agent) bool{
		func(a herdr.Agent) bool { return r.Lead.Matches(describe(a, tabLabels)) },
		func(a herdr.Agent) bool { return strings.ToLower(strings.TrimSpace(a.Name)) == key },
		func(a herdr.Agent) bool { return r.Workspace != "" && a.WorkspaceID == r.Workspace && !isRole(a) },
	}
	for _, test := range tests {
		for _, a := range agents {
			if test(a) {
				r.LeadStatus, r.LeadPane, r.LeadTitle = a.AgentStatus, a.PaneID, titleOf(a)
				return
			}
		}
	}
}

func (r *Run) placeRoles(roles []team.Role, agents []herdr.Agent, tabLabels map[string]string) {
	dispatches := r.dispatchNames()

	for _, role := range roles {
		c := Card{Role: role}
		for _, d := range dispatches {
			if role.Matches(d) {
				c.Rounds++
			}
		}
		var live *herdr.Agent
		for i := range agents {
			if !role.Matches(describe(agents[i], tabLabels)) {
				continue
			}
			// A copy that asks more of the user says more than an idle leftover.
			if live == nil || look.AgentRank(agents[i].AgentStatus) > look.AgentRank(live.AgentStatus) {
				live = &agents[i]
			}
		}
		done := r.done(role)
		if live != nil {
			c.PaneID, c.Status, c.Title = live.PaneID, live.AgentStatus, titleOf(*live)
		}
		switch {
		case live != nil && live.AgentStatus == "blocked":
			c.Column = Waiting
			c.Note = "asking you something"
		case live != nil && live.AgentStatus == "working":
			c.Column = Working
			if c.Rounds > 1 {
				c.Note = "round " + itoa(c.Rounds)
			}
		case done:
			c.Column = Done
			if c.Rounds > 1 {
				c.Note = itoa(c.Rounds) + " rounds"
			}
		case live != nil:
			c.Column = Working
			c.Note = "no result yet"
		case c.Rounds > 0:
			c.Column = Pending
			c.Stuck = true
			c.Note = "dispatched, but no agent and no result"
		default:
			c.Column = Pending
		}
		r.Cards = append(r.Cards, c)
	}
}

func (r *Run) dispatchNames() []string {
	entries, _ := os.ReadDir(r.Dir)
	var out []string
	for _, e := range entries {
		if r.layout.dispatch.MatchString(e.Name()) {
			out = append(out, strings.ToLower(e.Name()))
		}
	}
	return out
}

func leftPad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// mostMentionedSpec picks the story code the run's files mention most. Runs
// whose state file has no spec field name related stories too, but their own
// story dominates.
func (l Layout) mostMentionedSpec(dir string) string {
	if l.specCode == nil {
		return ""
	}
	counts := map[string]int{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, m := range l.specCode.FindAllString(string(data), -1) {
			if code := l.normalizeSpec(m); code != "" {
				counts[code]++
			}
		}
	}
	best, bestN := "", 0
	for code, n := range counts {
		if n > bestN || n == bestN && code < best {
			best, bestN = code, n
		}
	}
	return best
}

// findPrototype finds the story's prototype in folder (relative to specRoot):
// an HTML file named <spec>_..., preferring its _00_index page.
func findPrototype(specRoot, folder, spec string) string {
	if specRoot == "" || folder == "" || spec == "" {
		return ""
	}
	base := filepath.Join(specRoot, filepath.FromSlash(folder))
	var pages []string
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if !d.IsDir() && strings.HasPrefix(name, spec+"_") && strings.HasSuffix(strings.ToLower(name), ".html") {
			pages = append(pages, path)
		}
		return nil
	})
	if len(pages) == 0 {
		return ""
	}
	sort.Strings(pages)
	for _, p := range pages {
		if strings.Contains(filepath.Base(p), "_00_index") {
			return p
		}
	}
	return pages[0]
}

// ReadBranch reads the checked-out branch without running git: a worktree's
// .git is a file naming its git directory, whose HEAD names the branch.
func ReadBranch(worktree string) string {
	gitPath := filepath.Join(worktree, ".git")
	gitDir := gitPath
	if data, err := os.ReadFile(gitPath); err == nil {
		if d, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:"); ok {
			gitDir = strings.TrimSpace(d)
		}
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	ref, ok := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: refs/heads/")
	if !ok {
		return ""
	}
	return ref
}

func firstLines(lines []string, n int) []string {
	var out []string
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
			if len(out) == n {
				break
			}
		}
	}
	return out
}

func modTime(path string) int64 {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime().UnixNano()
	}
	return 0
}

func itoa(n int) string { return strconv.Itoa(n) }
