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

// Team is a named set of roles.
type Team struct {
	Name    string `toml:"name"`
	Match   string `toml:"match"`
	Default bool   `toml:"default"`
	Roles   []Role `toml:"role"`

	file string
	re   *regexp.Regexp
}

// Load reads the built-in teams and those in dir (which may be "" or
// missing). A broken file is skipped and reported; the rest still load.
func Load(dir string) ([]Team, []string) {
	files := map[string][]byte{}
	var problems []string
	entries, _ := fs.ReadDir(builtin, "teams")
	for _, e := range entries {
		data, _ := builtin.ReadFile("teams/" + e.Name())
		files[e.Name()] = data
	}
	if dir != "" {
		own, _ := filepath.Glob(filepath.Join(dir, "*.toml"))
		for _, path := range own {
			data, err := os.ReadFile(path)
			if err != nil {
				problems = append(problems, fmt.Sprintf("team %s: %v", path, err))
				continue
			}
			files[filepath.Base(path)] = data
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

// Pick is the team a run's "Team:" field names: the first whose match fits,
// else the default one, else the first.
func Pick(teams []Team, field string) (Team, bool) {
	for _, t := range teams {
		if t.re != nil && t.re.MatchString(field) {
			return t, true
		}
	}
	for _, t := range teams {
		if t.Default {
			return t, true
		}
	}
	if len(teams) > 0 {
		return teams[0], true
	}
	return Team{}, false
}
