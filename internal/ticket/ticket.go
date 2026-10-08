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
	// PaneID is the role's live agent, when there is one.
	PaneID string
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
	// Workspace is the Herdr workspace open on the worktree, or "".
	Workspace string

	Cards []Card

	fields   map[string]string
	sections map[string][]string
}

// Field returns a header field of STATE.md ("Team", "Target", ...).
func (r *Run) Field(name string) string { return r.fields[strings.ToLower(name)] }

// Find locates the run directory for a worktree: .runs/<KEY>/STATE.md, picking
// the key named in the branch when several runs share the worktree.
func Find(worktree string) (dir, key string, ok bool) {
	matches, _ := filepath.Glob(filepath.Join(worktree, ".runs", "*", "STATE.md"))
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
	dir, key, ok := Find(worktree)
	if !ok {
		return nil, os.ErrNotExist
	}
	text, err := os.ReadFile(filepath.Join(dir, "STATE.md"))
	if err != nil {
		return nil, err
	}
	r := &Run{
		Key:      key,
		Dir:      dir,
		Worktree: worktree,
		Branch:   ReadBranch(worktree),
		JiraURL:  links.Issue(key),
	}
	r.parse(string(text))
	r.Team = r.Field("Team")
	r.Target = r.Field("Target")
	r.Spec = normalizeSpec(r.Field("Spec"))
	if r.Spec == "" {
		r.Spec = mostMentionedSpec(dir)
	}
	r.Prototype = findPrototype(opts.SpecRoot, r.Spec)
	r.Objective = firstLines(r.section(`(?i)^(objective|objetivo)`), 3)
	r.CurrentStep = firstLines(r.section(`(?i)^(current step|paso actual|pipeline)`), 4)
	r.Questions = r.openQuestions()
	landed := strings.Join(r.section(`(?i)^landed`), "\n")
	// The landing check writes its report here whether it passed or not.
	r.Landed = strings.TrimSpace(landed) != "" && !regexp.MustCompile(`(?i)not landed|exit 1|blocked|bloquead`).MatchString(landed)

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

// section returns the lines of the first section whose heading matches.
func (r *Run) section(pattern string) []string {
	re := regexp.MustCompile(pattern)
	for h, lines := range r.sections {
		if re.MatchString(h) {
			return lines
		}
	}
	return nil
}

var (
	openHeading = regexp.MustCompile(`(?i)open|abiert|pendient|pending|blocker|bloque|pregunta|question`)
	notOpen     = regexp.MustCompile(`(?i)defect`)
	settled     = regexp.MustCompile(`(?i)\b(decided|ruled|answered|resuelt|settled|deferred|diferid|closed|cerrad)\b|defects: none|^-\s*none\b`)
)

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
		if !openHeading.MatchString(h) || notOpen.MatchString(h) {
			continue
		}
		for _, l := range r.sections[h] {
			t := strings.TrimSpace(l)
			if !strings.HasPrefix(t, "- ") || settled.MatchString(t) {
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
				r.LeadStatus, r.LeadPane = a.AgentStatus, a.PaneID
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
			// A working or blocked copy says more than an idle leftover.
			if live == nil || agents[i].AgentStatus == "working" || agents[i].AgentStatus == "blocked" {
				live = &agents[i]
			}
		}
		done := r.done(role)
		if live != nil {
			c.PaneID = live.PaneID
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
			c.Note = "idle, no result yet"
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

var dispatchFile = regexp.MustCompile(`(?i)^dispatch[-_].*\.md$`)

func (r *Run) dispatchNames() []string {
	entries, _ := os.ReadDir(r.Dir)
	var out []string
	for _, e := range entries {
		if dispatchFile.MatchString(e.Name()) {
			out = append(out, strings.ToLower(e.Name()))
		}
	}
	return out
}

var specCode = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(E\d+)[_-](US|TS)[_-](\d+)`)

// normalizeSpec turns any spelling of a story code into E7_US_42.
func normalizeSpec(s string) string {
	m := specCode.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	n, _ := strconv.Atoi(m[3])
	return strings.ToUpper(m[1]) + "_" + strings.ToUpper(m[2]) + "_" + leftPad2(n)
}

func leftPad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// mostMentionedSpec picks the story code the run's files mention most. Runs
// started before STATE.md carried a Spec field name related stories too, but
// their own story dominates.
func mostMentionedSpec(dir string) string {
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
		for _, m := range specCode.FindAllString(string(data), -1) {
			if code := normalizeSpec(m); code != "" {
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

// findPrototype finds the story's prototype under
// specifications/backlog/<E1>/: its _00_index page, else its first HTML file.
func findPrototype(specRoot, spec string) string {
	if specRoot == "" || spec == "" {
		return ""
	}
	increment := spec[:strings.Index(spec, "_")]
	base := filepath.Join(specRoot, "specifications", "backlog", increment)
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
