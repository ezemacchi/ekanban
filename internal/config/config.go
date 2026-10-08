// Package config holds the settings a user edits by hand.
//
// This is separate from board.json, which is state the board writes for itself
// -- statuses, notes, arrangement. Settings are the other direction: things you
// tell the board, which it never overwrites.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/ezemacchi/ekanban/internal/columns"
)

// PluginID must match herdr-plugin.toml, since Herdr keys the config directory
// by it.
const PluginID = "ekanban"

// Config is the whole settings file.
type Config struct {
	// PollInterval is how often the background watcher asks GitHub. Written as
	// a duration string: "30s", "2m", "10m".
	PollInterval string `toml:"poll_interval"`
	// Notifications turns Herdr toasts on or off. Bells are recorded either
	// way, so turning this off makes the board quiet rather than blind.
	Notifications *bool `toml:"notifications"`
	// Icons draws Nerd Font glyphs. Off by default: without a Nerd Font in the
	// terminal they render as empty boxes.
	Icons *bool `toml:"icons"`
	// SpecClone is the clone of the specifications repository the ticket
	// board finds prototypes in. Empty turns prototype links off.
	SpecClone string `toml:"spec_clone"`
	// IssueURL is the tracker link of a ticket, with {key} for its key.
	IssueURL string `toml:"issue_url"`
	// Pipeline drives the global board's computed columns.
	Pipeline PipelineConfig `toml:"pipeline"`
	// Ticket is how the ticket board shows a run.
	Ticket TicketConfig `toml:"ticket"`
	// Keys binds keys to actions by name: accept = "y" or accept = ["y", "Y"].
	Keys map[string]any `toml:"keys"`
}

// TicketConfig is the [ticket] table.
type TicketConfig struct {
	// Columns rename, reorder or re-icon the ticket board's four columns.
	Columns []columns.Column `toml:"column"`
}

// PipelineConfig is the [pipeline] table.
type PipelineConfig struct {
	// PRURL is a pull request's link, with {pr} for its number.
	PRURL string `toml:"pr_url"`
	// JenkinsPRJobs is the Jenkins multibranch job whose children are PR-<n>.
	JenkinsPRJobs string `toml:"jenkins_pr_jobs"`
	// Rules names the column rules in the order they are tried.
	Rules []string `toml:"rules"`
	// Targets are the branches pull requests merge into, most downstream
	// first, each with the publish job that deploys it.
	Targets []TargetConfig `toml:"target"`
	// Columns rename, recolour, reorder or re-icon the computed columns, and
	// add one for a custom rule to return.
	Columns []columns.Column `toml:"column"`
}

// TargetConfig is one [[pipeline.target]].
type TargetConfig struct {
	Branch  string `toml:"branch"`
	Env     string `toml:"env"`
	Publish string `toml:"publish"`
}

const (
	DefaultPollInterval = 2 * time.Minute
	// Below this the watcher would ask GitHub more often than anything it
	// watches realistically changes.
	MinPollInterval = 30 * time.Second
	MaxPollInterval = time.Hour
)

// Settings is the resolved, validated configuration.
type Settings struct {
	PollInterval  time.Duration
	Notifications bool
	Icons         bool
	SpecClone     string
	IssueURL      string
	Pipeline      PipelineConfig
	Ticket        TicketConfig
	Keys          map[string][]string // action name -> keys
	// Path is where the file was read from, whether or not it existed.
	Path string
	// Problems are complaints about the file's contents. A bad value falls back
	// to its default rather than stopping the board, but it is reported so a
	// typo is not silently ignored.
	Problems []string
}

// Dir is the plugin's config directory: the one Herdr injects when it launches
// us, or the same path reconstructed when run by hand.
func Dir() (string, error) {
	if dir := os.Getenv("HERDR_PLUGIN_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	if roaming := os.Getenv("APPDATA"); runtime.GOOS == "windows" && roaming != "" {
		return filepath.Join(roaming, "herdr", "plugins", "config", PluginID), nil
	}
	base, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ".config", "herdr", "plugins", "config", PluginID), nil
}

// Load reads the settings, falling back to defaults for anything absent or
// unusable. A missing file is the normal case, not an error.
func Load() Settings {
	s := Settings{PollInterval: DefaultPollInterval, Notifications: true}

	dir, err := Dir()
	if err != nil {
		return s
	}
	s.Path = filepath.Join(dir, "config.toml")

	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err != nil {
		s.Problems = append(s.Problems, fmt.Sprintf("could not read %s: %v", s.Path, err))
		return s
	}

	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		s.Problems = append(s.Problems, fmt.Sprintf("%s is not valid TOML: %v", s.Path, err))
		return s
	}

	if c.PollInterval != "" {
		d, err := time.ParseDuration(c.PollInterval)
		switch {
		case err != nil:
			s.Problems = append(s.Problems, fmt.Sprintf("poll_interval %q is not a duration — using %s", c.PollInterval, s.PollInterval))
		case d < MinPollInterval:
			s.Problems = append(s.Problems, fmt.Sprintf("poll_interval %s is below the %s minimum — using %s", d, MinPollInterval, MinPollInterval))
			s.PollInterval = MinPollInterval
		case d > MaxPollInterval:
			s.Problems = append(s.Problems, fmt.Sprintf("poll_interval %s is above the %s maximum — using %s", d, MaxPollInterval, MaxPollInterval))
			s.PollInterval = MaxPollInterval
		default:
			s.PollInterval = d
		}
	}

	if c.Notifications != nil {
		s.Notifications = *c.Notifications
	}
	if c.Icons != nil {
		s.Icons = *c.Icons
	}
	s.SpecClone = c.SpecClone
	s.IssueURL = c.IssueURL
	s.Pipeline = c.Pipeline
	s.Ticket = c.Ticket
	s.Keys, s.Problems = bindings(c.Keys, s.Problems)
	if len(s.Pipeline.Targets) == 0 {
		s.Pipeline.Targets = []TargetConfig{{Branch: "main"}}
	}
	return s
}

// bindings reads [keys]: each action takes one key or a list of them.
func bindings(raw map[string]any, problems []string) (map[string][]string, []string) {
	out := map[string][]string{}
	for name, v := range raw {
		switch v := v.(type) {
		case string:
			out[name] = []string{v}
		case []any:
			for _, k := range v {
				s, ok := k.(string)
				if !ok || s == "" {
					problems = append(problems, fmt.Sprintf("keys.%s: every key must be a non-empty string", name))
					continue
				}
				out[name] = append(out[name], s)
			}
		default:
			problems = append(problems, fmt.Sprintf("keys.%s: use a key in quotes or a list of them", name))
		}
	}
	return out, problems
}

// Example is the commented template written by `ekanban config --init`.
const Example = `# ekanban settings.
#
# Every value is optional; delete a line to go back to its default.

# How often the background watcher asks GitHub about your pull requests.
# Minimum 30s, maximum 1h. Opening or closing a workspace polls immediately
# regardless, so this only governs noticing a review landing or CI going red.
poll_interval = "2m"

# Herdr toasts when a pull request changes. Bells on the board are recorded
# either way, so turning this off makes the board quiet rather than blind.
notifications = true

# Nerd Font icons on the ticket board. Needs a Nerd Font in the terminal
# (e.g. Cascadia Code NF); without one they show as empty boxes.
icons = false

# The clone of your specifications repository, where the ticket board finds
# prototypes. Leave it out to turn prototype links off.
# spec_clone = 'C:\repos\specs'

# Tracker link of a ticket; {key} is replaced by its key.
# issue_url = "https://your-site.atlassian.net/browse/{key}"

# The global board's computed columns. Everything here is optional: without
# Jenkins, pull requests are found from the run and merge commits only.
[pipeline]
# pr_url = "https://git.example.com/projects/P/repos/r/pull-requests/{pr}"
# jenkins_pr_jobs = "https://jenkins.example.com/job/Folder/job/Repo"

# Rules decide a card's column, tried in order; the first that decides wins.
# Known: deployed, merged, not-started, working, landed.
# rules = ["deployed", "merged", "not-started", "working", "landed"]

# Branches pull requests merge into, most downstream first. publish is the
# Jenkins job that deploys the branch; env is how the card names it.
# [[pipeline.target]]
# branch = "main"
# env = "dev"
# publish = "https://jenkins.example.com/job/Folder/job/Repo%20Publish"

# The global board's columns. The id is what the rules return; label, color
# (ANSI index or hex) and icon (a Nerd Font glyph) are how it looks. Listed
# columns come first, in this order; any left out follow with their defaults.
# Ids: todo, in_progress, on_review, to_deploy, ready_qa. A new id adds a
# column for a custom rule to return.
# [[pipeline.column]]
# id = "in_progress"
# label = "Working"
# color = "39"

# The ticket board's columns, same fields. Ids: pending, working, waiting,
# done; these four are fixed, so a new id is ignored.
# [[ticket.column]]
# id = "waiting"
# label = "Waiting on you"

# Keys, by action name: one key or a list. A rebound action stops answering
# its old key. The help screen (?) shows the current keys and the names are
# listed by ` + "`ekanban keys`" + `. gg, gp, gf and 1-9 are fixed.
# [keys]
# accept = "y"
# archive = ["A", "z"]
`

// WriteExample creates the template, refusing to overwrite an existing file.
func WriteExample() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "config.toml")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return path, fmt.Errorf("%s already exists — edit it instead", path)
		}
		return "", err
	}
	defer f.Close()

	_, err = f.WriteString(Example)
	return path, err
}
