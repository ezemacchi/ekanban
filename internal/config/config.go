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
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/team"
	"github.com/ezemacchi/ekanban/internal/ticket"
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
	// Orchestrator overrides the teams' [orchestrator]: how the agent running a ticket is
	// recognised and started.
	Orchestrator team.Orchestrator `toml:"orchestrator"`
	// Lead is [orchestrator]'s old name, still read; see foldLead.
	Lead *team.Orchestrator `toml:"lead"`
	// Run is where a team run lives and how its state file reads.
	Run ticket.LayoutConfig `toml:"run"`
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
	// CI is the build server pull request builds and publishes are read from.
	CI CIConfig `toml:"ci"`
	// JenkinsPRJobs is the older spelling of ci = {kind = "jenkins", url = ...},
	// still read when [pipeline.ci] is absent.
	JenkinsPRJobs string `toml:"jenkins_pr_jobs"`
	// CodeHost is how merge commits are worded: bitbucket, github or any.
	CodeHost string `toml:"code_host"`
	// TicketKey is the regular expression a ticket key matches in a branch.
	TicketKey string `toml:"ticket_key"`
	// Rules names the column rules in the order they are tried.
	Rules []string `toml:"rules"`
	// Targets are the branches pull requests merge into, most downstream
	// first, each with the publish job that deploys it.
	Targets []TargetConfig `toml:"target"`
	// Columns rename, recolour, reorder or re-icon the computed columns, and
	// add one for a custom rule to return.
	Columns []columns.Column `toml:"column"`
}

// CIConfig is the [pipeline.ci] table.
type CIConfig struct {
	Kind string `toml:"kind"`
	URL  string `toml:"url"`
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
	Keys          map[string][]string            // action name -> keys
	ScreenKeys    map[string]map[string][]string // [keys.board], [keys.ticket]
	Orchestrator  team.Orchestrator              // [orchestrator], over each team's
	Layout        ticket.Layout                  // [run]
	// Path is where config.toml was read from, whether or not it existed.
	Path string
	// RepoPath is the repository's .ekanban.toml read over it, "" for none.
	RepoPath string
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

// Load reads config.toml alone; LoadFor also reads a repository's file.
func Load() Settings { return LoadFor("") }

// LoadFor reads config.toml, then the .ekanban.toml of the repository dir is
// in over it: a value the repository file sets replaces config.toml's, a list
// replaces the whole list, and a table merges field by field. Anything absent
// or unusable falls back to its default; a missing file is the normal case,
// not an error.
//
// The repository file describes the project's process, so it cannot choose
// which program starts: [orchestrator] kind and args are read from config.toml only.
func LoadFor(dir string) Settings {
	s := Settings{PollInterval: DefaultPollInterval, Notifications: true}
	var c Config
	if base, err := Dir(); err == nil {
		s.Path = filepath.Join(base, "config.toml")
		s.Problems = decodeOver(&c, s.Path, s.Problems)
		s.Problems = foldLead(&c, s.Path, s.Problems)
	}
	if s.RepoPath = RepoFile(dir); s.RepoPath != "" {
		orchestrator := c.Orchestrator
		s.Problems = decodeOver(&c, s.RepoPath, s.Problems)
		s.Problems = foldLead(&c, s.RepoPath, s.Problems)
		if c.Orchestrator.Kind != orchestrator.Kind || !slices.Equal(c.Orchestrator.Args, orchestrator.Args) {
			s.Problems = append(s.Problems, fmt.Sprintf("%s: [orchestrator] kind and args are only read from config.toml — ignored", s.RepoPath))
			c.Orchestrator.Kind, c.Orchestrator.Args = orchestrator.Kind, orchestrator.Args
		}
	}
	return resolve(c, s)
}

// decodeOver decodes the file at path over c. An unreadable or invalid file
// is reported and leaves c as it was.
func decodeOver(c *Config, path string, problems []string) []string {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return problems
	}
	if err != nil {
		return append(problems, fmt.Sprintf("could not read %s: %v", path, err))
	}
	// Checked on its own first: a decode that fails halfway would leave c
	// partly overwritten.
	var probe Config
	if err := toml.Unmarshal(data, &probe); err != nil {
		return append(problems, fmt.Sprintf("%s is not valid TOML: %v", path, err))
	}
	_ = toml.Unmarshal(data, c)
	return problems
}

// foldLead moves a [lead] table, [orchestrator]'s old name, into
// [orchestrator], whose own fields win, and asks for the rename.
func foldLead(c *Config, path string, problems []string) []string {
	if c.Lead == nil {
		return problems
	}
	c.Orchestrator = c.Lead.Over(c.Orchestrator)
	c.Lead = nil
	return append(problems, fmt.Sprintf("%s: [lead] is now [orchestrator]; it still works, rename it", path))
}

// resolve validates c into s.
func resolve(c Config, s Settings) Settings {
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
	s.Keys, s.ScreenKeys, s.Problems = bindings(c.Keys, s.Problems)
	if _, err := (team.Orchestrator{}).With(c.Orchestrator); err != nil {
		s.Problems = append(s.Problems, fmt.Sprintf("[orchestrator]: %v — ignored", err))
	} else {
		s.Orchestrator = c.Orchestrator
	}
	if s.Pipeline.CI.Kind == "" && s.Pipeline.CI.URL == "" && s.Pipeline.JenkinsPRJobs != "" {
		s.Pipeline.CI = CIConfig{Kind: "jenkins", URL: s.Pipeline.JenkinsPRJobs}
	}
	if s.Pipeline.CI.Kind == "" && s.Pipeline.CI.URL != "" {
		s.Problems = append(s.Problems, "pipeline.ci has a url but no kind — no build server is read")
	}
	if len(s.Pipeline.Targets) == 0 {
		s.Pipeline.Targets = []TargetConfig{{Branch: "main"}}
	}
	var layoutProblems []string
	s.Layout, layoutProblems = ticket.NewLayout(c.Run)
	s.Problems = append(s.Problems, layoutProblems...)
	return s
}

// Screens are the boards [keys.<screen>] can address.
var Screens = []string{"board", "ticket"}

// KeysFor is [keys] with [keys.<screen>] over it.
func (s Settings) KeysFor(screen string) map[string][]string {
	out := map[string][]string{}
	for name, ks := range s.Keys {
		out[name] = ks
	}
	for name, ks := range s.ScreenKeys[screen] {
		out[name] = ks
	}
	return out
}

// bindings reads [keys]: each action takes one key or a list of them (an
// empty list turns it off), and [keys.board] / [keys.ticket] do the same for
// one board only.
func bindings(raw map[string]any, problems []string) (map[string][]string, map[string]map[string][]string, []string) {
	flat := map[string]any{}
	screens := map[string]map[string][]string{}
	for name, v := range raw {
		table, ok := v.(map[string]any)
		if !ok {
			flat[name] = v
			continue
		}
		if !slices.Contains(Screens, name) {
			problems = append(problems, fmt.Sprintf("[keys.%s]: no such board (known: %s)", name, strings.Join(Screens, ", ")))
			continue
		}
		screens[name], problems = bindingsOf(table, "keys."+name, problems)
	}
	out, problems := bindingsOf(flat, "keys", problems)
	return out, screens, problems
}

// renamedAction is an action [keys] still accepts under its old name.
var renamedAction = struct{ from, to string }{"lead", "orchestrator"}

func bindingsOf(raw map[string]any, where string, problems []string) (map[string][]string, []string) {
	out := map[string][]string{}
	for name, v := range raw {
		switch v := v.(type) {
		case string:
			out[name] = []string{v}
		case []any:
			out[name] = []string{}
			for _, k := range v {
				s, ok := k.(string)
				if !ok || s == "" {
					problems = append(problems, fmt.Sprintf("%s.%s: every key must be a non-empty string", where, name))
					continue
				}
				out[name] = append(out[name], s)
			}
		default:
			problems = append(problems, fmt.Sprintf("%s.%s: use a key in quotes or a list of them", where, name))
		}
	}
	if ks, ok := out[renamedAction.from]; ok {
		if _, both := out[renamedAction.to]; !both {
			out[renamedAction.to] = ks
		}
		delete(out, renamedAction.from)
		problems = append(problems, fmt.Sprintf("%s.%s is now %s.%s; it still works, rename it", where, renamedAction.from, where, renamedAction.to))
	}
	return out, problems
}

// Example is the commented template written by `ekanban config --init`.
const Example = `# ekanban settings.
#
# Every value is optional; delete a line to go back to its default.
#
# A repository can add its own .ekanban.toml at its root, with any of the
# settings below: it is read over this file whenever a board opens inside that
# repository (or one of its worktrees). Put the project's process there --
# tracker, pull request and build server links, targets, [run], the [orchestrator]
# prompt -- and keep personal choices here. [orchestrator] kind and args, which choose
# the program a board starts, are read from this file only.

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
# a build server, pull requests are found from the run and merge commits only.
[pipeline]
# pr_url = "https://git.example.com/projects/P/repos/r/pull-requests/{pr}"

# How merge commits are worded, to find a merged pull request in git:
# bitbucket, github or any (both; the default).
# code_host = "any"

# The regular expression a ticket key matches in a branch name. The default
# is a Jira-style key, PROJ-123.
# ticket_key = '[A-Z][A-Z0-9]+-\d+'

# The build server. kind "jenkins" is built in; url is the multibranch job
# whose children are the pull request builds (PR-<n>). Leave it out for
# none. (The older jenkins_pr_jobs = "<url>" still works.)
# ci = { kind = "jenkins", url = "https://jenkins.example.com/job/Folder/job/Repo" }

# Rules decide a card's column, tried in order; the first that decides wins.
# Known: deployed, merged, not-started, working, landed.
# rules = ["deployed", "merged", "not-started", "working", "landed"]

# Branches pull requests merge into, most downstream first. publish is the
# build server job that deploys the branch; env is how the card names it.
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

# The agent that runs a ticket: the team's [orchestrator], with these fields
# over it. o on the boards goes to it, or opens one in a new tab of the
# ticket's workspace when none is open. kind is the Herdr agent kind to start
# (cursor, claude, ...); without it the boards only go to an open one. prompt
# is written to the run folder and the new agent is told to read it.
# Placeholders: {label} {key} {team} {target} {branch} {worktree} {run}
# {state} {workspace} {issue}. [lead], the table's old name, is still read.
# [orchestrator]
# kind = "cursor"
# args = ["--model", "some-model"]
# prompt = "You are the {label} of {key}. Read {state} in full and continue the run."

# Where a team run lives in a worktree and how its state file reads. A run is
# <dir>/<KEY>/<state>; the boards read its header fields and "## " sections.
# The patterns are regular expressions. The defaults are shown.
# [run]
# dir = ".runs"
# state = "STATE.md"
# dispatch = '(?i)^dispatch[-_].*\.md$'   # files that send work to a role
# team_field = "Team"
# target_field = "Target"
# spec_field = "Spec"
# objective = '(?i)^objective'            # section headings ...
# current_step = '(?i)^(current step|pipeline)'
# landed = '(?i)^landed'
# not_landed = '(?i)not landed|exit 1|blocked'  # ... and text in the landed one
# questions = '(?i)open|pending|blocker|question'
# not_question = '(?i)defect'
# settled = '(?i)\b(decided|ruled|answered|settled|deferred|closed)\b|defects: none|^-\s*none\b'
# A story code in the run's files, and the folder under spec_clone holding its
# HTML prototypes ({1}, {2}... are the code's groups). Off unless set.
# spec_code = '(?i)\b(E\d+)[_-](US|TS)[_-](\d+)'
# prototypes = "specifications/backlog/{1}"

# Keys, by action name: one key or a list. A rebound action stops answering
# its old key, and an empty list turns it off. The help screen (?) shows the
# current keys and the names are listed by ` + "`ekanban keys`" + `. gg, gp,
# gf and 1-9 are fixed. [keys.board] and [keys.ticket] apply to one board only.
# [keys]
# accept = "y"
# archive = ["A", "z"]
# [keys.board]
# orchestrator = []
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
