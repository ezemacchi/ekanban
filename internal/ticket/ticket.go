// Package ticket reads one Big Team or Full Team run from
// its worktree, so the board can show which role is doing what.
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

	"github.com/phin-tech/herdr-phin-board/internal/herdr"
	"github.com/phin-tech/herdr-phin-board/internal/links"
)

// Column is a kanban column.
type Column int

const (
	Pending Column = iota
	Working
	Waiting
	Done
)

// Columns in display order, with the label shown above each.
var Columns = []struct {
	Col   Column
	Label string
}{
	{Pending, "Pendiente"},
	{Working, "Trabajando"},
	{Waiting, "Esperándote"},
	{Done, "Terminado"},
}

// Role is one role of a team and how to recognise it on disk and in Herdr.
type Role struct {
	ID    string
	Label string
	match *regexp.Regexp
	// done reports whether the role left its result in the run directory.
	done func(r *Run) bool
}

func fileDone(name string) func(*Run) bool {
	return func(r *Run) bool {
		_, err := os.Stat(filepath.Join(r.Dir, name))
		return err == nil
	}
}

var missionControl = []Role{
	{"technical-lead", "Technical Lead", regexp.MustCompile(`tech(nical)?-?lead`), fileDone("ENVELOPE.md")},
	{"evidence", "Evidence Researcher", regexp.MustCompile(`evidence|researcher`), fileDone("EVIDENCE.md")},
	{"implementer", "Implementer", regexp.MustCompile(`implementer`), fileDone("IMPL.md")},
	{"reviewer", "Reviewer", regexp.MustCompile(`reviewer|parity`), fileDone("PARITY.md")},
	{"qa", "Adversarial QA", regexp.MustCompile(`(^|[^a-z])qa([^a-z]|$)|adversarial`), fileDone("QA.md")},
	{"curator", "Context Curator", regexp.MustCompile(`curator`), func(r *Run) bool { return r.Landed }},
}

var pitCrew = []Role{
	{"mechanic", "Mechanic", regexp.MustCompile(`mechanic`), func(r *Run) bool { return r.Field("Pushed") != "" }},
	{"inspector", "Inspector", regexp.MustCompile(`inspector`), fileDone("REVIEW.md")},
}

// Card is one role on the board.
type Card struct {
	Role   Role
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
	Team     string
	Target   string

	Objective   []string
	CurrentStep []string
	Questions   []string
	Landed      bool

	OrchestratorStatus string
	OrchestratorPane   string

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
	branch := readBranch(worktree)
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
func Load(worktree string, agents []herdr.Agent, tabLabels map[string]string) (*Run, error) {
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
		Branch:   readBranch(worktree),
		JiraURL:  links.Issue(key),
	}
	r.parse(string(text))
	r.Team = r.Field("Team")
	r.Target = r.Field("Target")
	r.Objective = firstLines(r.section(`(?i)^(objective|objetivo)`), 3)
	r.CurrentStep = firstLines(r.section(`(?i)^(current step|paso actual|pipeline)`), 4)
	r.Questions = r.openQuestions()
	landed := strings.Join(r.section(`(?i)^landed`), "\n")
	// The landing check writes its report here whether it passed or not.
	r.Landed = strings.TrimSpace(landed) != "" && !regexp.MustCompile(`(?i)not landed|exit 1|blocked|bloquead`).MatchString(landed)

	mine := agentsIn(worktree, agents)
	r.placeOrchestrator(mine, tabLabels)
	r.placeRoles(mine, tabLabels)
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

func agentsIn(worktree string, agents []herdr.Agent) []herdr.Agent {
	root := strings.ToLower(filepath.Clean(worktree))
	var out []herdr.Agent
	for _, a := range agents {
		if a.Agent == nil || a.Cwd == "" {
			continue
		}
		cwd := strings.ToLower(filepath.Clean(a.Cwd))
		if cwd == root || strings.HasPrefix(cwd, root+string(filepath.Separator)) {
			out = append(out, a)
		}
	}
	return out
}

func describe(a herdr.Agent, tabLabels map[string]string) string {
	return strings.ToLower(a.Name + " " + tabLabels[a.TabID])
}

func (r *Run) placeOrchestrator(agents []herdr.Agent, tabLabels map[string]string) {
	key := strings.ToLower(r.Key)
	for _, a := range agents {
		d := describe(a, tabLabels)
		if strings.Contains(d, "orchestrator") || strings.TrimSpace(a.Name) == key {
			r.OrchestratorStatus = a.AgentStatus
			r.OrchestratorPane = a.PaneID
			return
		}
	}
}

func (r *Run) placeRoles(agents []herdr.Agent, tabLabels map[string]string) {
	roles := missionControl
	if regexp.MustCompile(`(?i)full`).MatchString(r.Team) {
		roles = pitCrew
	}
	dispatches := r.dispatchNames()

	for _, role := range roles {
		c := Card{Role: role}
		for _, d := range dispatches {
			if role.match.MatchString(d) {
				c.Rounds++
			}
		}
		var live *herdr.Agent
		for i := range agents {
			if !role.match.MatchString(describe(agents[i], tabLabels)) {
				continue
			}
			// A working or blocked copy says more than an idle leftover.
			if live == nil || agents[i].AgentStatus == "working" || agents[i].AgentStatus == "blocked" {
				live = &agents[i]
			}
		}
		done := role.done(r)
		if live != nil {
			c.PaneID = live.PaneID
		}
		switch {
		case live != nil && live.AgentStatus == "blocked":
			c.Column = Waiting
			c.Note = "te está preguntando algo"
		case live != nil && live.AgentStatus == "working":
			c.Column = Working
			if c.Rounds > 1 {
				c.Note = "vuelta " + itoa(c.Rounds)
			}
		case done:
			c.Column = Done
			if c.Rounds > 1 {
				c.Note = itoa(c.Rounds) + " vueltas"
			}
		case live != nil:
			c.Column = Working
			c.Note = "quieto, sin resultado todavía"
		case c.Rounds > 0:
			c.Column = Pending
			c.Stuck = true
			c.Note = "se despachó, pero no hay agente ni resultado"
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

// readBranch reads the checked-out branch without running git: a worktree's
// .git is a file naming its git directory, whose HEAD names the branch.
func readBranch(worktree string) string {
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
