// Package team holds the definitions of the teams a run can belong to: their
// roles, how each role is recognised, and when it is done. Definitions are
// TOML files, so a team is described, not coded. The built-in ones are
// embedded; a file of the same name in the user's teams folder replaces one,
// and any other file there adds a team.
package team

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed teams/*.toml
var builtin embed.FS

// Role is one role of a team.
type Role struct {
	ID    string `toml:"id"`
	Label string `toml:"label"`
	Icon  string `toml:"icon"`
	// Match is a regex on dispatch file names, agent names and tab labels.
	Match string `toml:"match"`
	// A role is done when any of these holds.
	DoneFile       string `toml:"done_file"`
	DoneField      string `toml:"done_field"`
	DoneWhenLanded bool   `toml:"done_when_landed"`

	re *regexp.Regexp
}

// Matches reports whether text (lower case) names this role.
func (r Role) Matches(text string) bool { return r.re != nil && r.re.MatchString(text) }

// Lead is the agent that runs the team. The board finds it among the ticket's
// agents, and opens one when there is none.
type Lead struct {
	Label string `toml:"label"` // shown on the board: "Orchestrator"
	// Match is a regex on agent names and tab labels.
	Match string `toml:"match"`
	// Tab is the label of the tab a new lead opens in.
	Tab string `toml:"tab"`
	// Kind is the Herdr agent kind a new lead starts as: cursor, claude, ...
	// Empty means the board cannot start one.
	Kind string   `toml:"kind"`
	Args []string `toml:"args"` // passed to the agent
	// Prompt is sent to a new lead, written to a file in the run folder and
	// pointed at. Placeholders: {label} {key} {team} {target} {branch}
	// {worktree} {run} {state} {workspace} {issue}.
	Prompt string `toml:"prompt"`

	re *regexp.Regexp
}

// DefaultPrompt is a new lead's prompt when the team and config.toml give none.
const DefaultPrompt = "You are the {label} of ticket {key}, team {team}. The run's folder is {run}. " +
	"Read {state} in full, then continue the run from where it stopped. Work only in {worktree}."

// Matches reports whether text (lower case) names the lead.
func (l Lead) Matches(text string) bool { return l.re != nil && l.re.MatchString(text) }

// With is l with the fields o sets replacing its own: config.toml's [lead]
// over the team's.
func (l Lead) With(o Lead) (Lead, error) {
	if o.Label != "" {
		l.Label = o.Label
	}
	if o.Match != "" {
		l.Match = o.Match
	}
	if o.Tab != "" {
		l.Tab = o.Tab
	}
	if o.Kind != "" {
		l.Kind = o.Kind
	}
	if o.Args != nil {
		l.Args = o.Args
	}
	if o.Prompt != "" {
		l.Prompt = o.Prompt
	}
	return l, l.compile()
}

func (l *Lead) compile() error {
	if l.Label == "" {
		l.Label = "Lead"
	}
	if l.Tab == "" {
		l.Tab = strings.ToLower(l.Label)
	}
	if l.Prompt == "" {
		l.Prompt = DefaultPrompt
	}
	l.re = nil
	if l.Match == "" {
		return nil
	}
	re, err := regexp.Compile(l.Match)
	if err != nil {
		return fmt.Errorf("lead match: %v", err)
	}
	l.re = re
	return nil
}

// Team is a named set of roles.
type Team struct {
	Name    string `toml:"name"`
	Match   string `toml:"match"`
	Default bool   `toml:"default"`
	Lead    Lead   `toml:"lead"`
	Roles   []Role `toml:"role"`

	file    string
	builtin bool
	re      *regexp.Regexp
}

// Load reads the built-in teams and those in dir (which may be "" or
// missing). A broken file is skipped and reported; the rest still load.
func Load(dir string) ([]Team, []string) {
	files := map[string][]byte{}
	own := map[string]bool{}
	var problems []string
	entries, _ := fs.ReadDir(builtin, "teams")
	for _, e := range entries {
		data, _ := builtin.ReadFile("teams/" + e.Name())
		files[e.Name()] = data
	}
	if dir != "" {
		paths, _ := filepath.Glob(filepath.Join(dir, "*.toml"))
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				problems = append(problems, fmt.Sprintf("team %s: %v", path, err))
				continue
			}
			files[filepath.Base(path)] = data
			own[filepath.Base(path)] = true
		}
	}

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var teams []Team
	for _, n := range names {
		t, err := parse(n, files[n])
		if err != nil {
			problems = append(problems, fmt.Sprintf("team %s: %v", n, err))
			continue
		}
		t.builtin = !own[n]
		teams = append(teams, t)
	}
	return teams, problems
}

func parse(file string, data []byte) (Team, error) {
	var t Team
	if err := toml.Unmarshal(data, &t); err != nil {
		return t, err
	}
	t.file = file
	if t.Name == "" {
		t.Name = strings.TrimSuffix(file, ".toml")
	}
	if len(t.Roles) == 0 {
		return t, fmt.Errorf("no [[role]]")
	}
	if t.Match != "" {
		re, err := regexp.Compile(t.Match)
		if err != nil {
			return t, fmt.Errorf("match: %v", err)
		}
		t.re = re
	}
	if err := t.Lead.compile(); err != nil {
		return t, err
	}
	for i := range t.Roles {
		r := &t.Roles[i]
		if r.ID == "" || r.Match == "" {
			return t, fmt.Errorf("role %d needs an id and a match", i+1)
		}
		if r.Label == "" {
			r.Label = r.ID
		}
		re, err := regexp.Compile(r.Match)
		if err != nil {
			return t, fmt.Errorf("role %s match: %v", r.ID, err)
		}
		r.re = re
	}
	return t, nil
}

// Pick is the team a run's team field names: the first whose match fits,
// else the default one (a user's before a built-in), else the first.
func Pick(teams []Team, field string) (Team, bool) {
	for _, t := range teams {
		if t.re != nil && t.re.MatchString(field) {
			return t, true
		}
	}
	for _, builtin := range []bool{false, true} {
		for _, t := range teams {
			if t.Default && t.builtin == builtin {
				return t, true
			}
		}
	}
	if len(teams) > 0 {
		return teams[0], true
	}
	return Team{}, false
}
