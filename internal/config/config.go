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

	"github.com/ezemacchi/ekanban/internal/bitbucket"
	"github.com/ezemacchi/ekanban/internal/columns"
	"github.com/ezemacchi/ekanban/internal/jira"
	"github.com/ezemacchi/ekanban/internal/orchestrator"
	"github.com/ezemacchi/ekanban/internal/reviews"
	"github.com/ezemacchi/ekanban/internal/stack"
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
	// Keys binds keys to actions by name: accept = "Z" or accept = ["Z", "z"].
	Keys map[string]any `toml:"keys"`
	// Orchestrator overrides the teams' [orchestrator]: how the agent running a ticket is
	// recognised and started.
	Orchestrator team.Orchestrator `toml:"orchestrator"`
	// Lead is [orchestrator]'s old name, still read; see foldLead.
	Lead *team.Orchestrator `toml:"lead"`
	// Run is where a team run lives and how its state file reads.
	Run ticket.LayoutConfig `toml:"run"`
	// Jira is the site the global board reads tickets from.
	Jira jira.Config `toml:"jira"`
	// Bitbucket is the server whose pull requests the board judges: whether
	// each one may merge, SonarQube's quality gate included.
	Bitbucket bitbucket.Config `toml:"bitbucket"`
	// Reviews is where the pull requests waiting on the user's review come
	// from, and where their cards go.
	Reviews reviews.Config `toml:"reviews"`
	// Broadcast is what the global board offers to say to every orchestrator.
	Broadcast BroadcastConfig `toml:"broadcast"`
	// Questions is how the boards answer a run's open questions: the message
	// Reply types into the orchestrator, and the agents Discuss starts.
	Questions orchestrator.Questions `toml:"questions"`
	// Stack is stacked cards: related tickets drawn under a card. Off unless
	// it names a source.
	Stack stack.Config `toml:"stack"`
	// Close is how a finished ticket is closed: the check run before it.
	Close CloseConfig `toml:"close"`
	// Accept is which columns a ticket may be accepted from.
	Accept AcceptConfig `toml:"accept"`
}

// CloseConfig is the [close] table.
type CloseConfig struct {
	// Check is a program run in the worktree before a ticket is closed, with
	// its arguments; {key}, {worktree}, {branch}, {target}, {pr} and {repo}
	// are filled in. Anything but exit 0 stops the close, and what it printed
	// is shown. It starts a program, so it is read from config.toml only.
	Check []string `toml:"check"`
}

// AcceptConfig is the [accept] table.
type AcceptConfig struct {
	// From are the column ids a ticket may be accepted from; empty is the
	// last one, ready_qa, alone.
	From []string `toml:"from"`
}

// BroadcastConfig is the [broadcast] table.
type BroadcastConfig struct {
	// Presets are ready-made messages; {target} is each ticket's target
	// branch and {key} its key.
	Presets []string `toml:"presets"`
}

// DefaultPresets are the broadcast's messages when [broadcast] names none.
var DefaultPresets = []string{
	"Fetch and rebase your branch onto origin/{target}, run the checks, and push.",
	"Stop where you are and tell me in two lines what is done and what is left.",
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
	// first, each with the publish job that deploys it. Empty when unset:
	// the caller fills the repository's default branch (pipeline.DefaultBranch).
	Targets []TargetConfig `toml:"target"`
	// Columns rename, recolour, reorder or re-icon the computed columns, and
	// add one for a custom rule to return.
	Columns []columns.Column `toml:"column"`
	// FastForward brings the local target branches up to their remote after
	// each fetch, never over changes or commits of their own. It moves your
	// checkouts, so it is read from config.toml only.
	FastForward bool `toml:"fast_forward"`
	// ManualMoves lets a card be moved by hand over its computed column, until
	// that column changes. Unset means on; a team can turn it off in its
	// repository file.
	ManualMoves *bool `toml:"manual_moves"`
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
	Jira          jira.Config                    // [jira], defaults filled
	Bitbucket     bitbucket.Config               // [bitbucket], defaults filled
	Reviews       reviews.Config                 // [reviews]
	Broadcast     []string                       // [broadcast] presets, defaults filled
	Questions     orchestrator.Questions         // [questions], defaults filled
	Stack         stack.Config                   // [stack]; no sources means off
	CloseCheck    []string                       // [close] check, from config.toml only
	AcceptFrom    []string                       // [accept] from; empty: the last column alone
	// TargetDefaulted is true when [[pipeline.target]] named no branch and
	// the one in Pipeline.Targets was taken from the repository instead.
	TargetDefaulted bool
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
// which program starts or where the Jira token goes: [orchestrator] kind and
// args, and [jira] url, email_env, token_env and handoff.command, are read
// from config.toml only.
func LoadFor(dir string) Settings {
	s := Settings{PollInterval: DefaultPollInterval, Notifications: true}
	var c Config
	if base, err := Dir(); err == nil {
		s.Path = filepath.Join(base, "config.toml")
		s.Problems = decodeOver(&c, s.Path, s.Problems)
		s.Problems = foldLead(&c, s.Path, s.Problems)
	}
	if s.RepoPath = RepoFile(dir); s.RepoPath != "" {
		// Snapshots of what only config.toml may set. The lists are cloned:
		// decoding over a slice rewrites its elements in place, which would
		// rewrite the snapshot and let the repository file through.
		orchestrator, account, server, ff := c.Orchestrator, c.Jira, c.Bitbucket, c.Pipeline.FastForward
		reviewKind, reviewCommand := c.Reviews.Kind, slices.Clone(c.Reviews.Command)
		closeCheck := slices.Clone(c.Close.Check)
		small, large := c.Questions.Small, c.Questions.Large
		small.Args, large.Args = slices.Clone(small.Args), slices.Clone(large.Args)
		orchestrator.Args = slices.Clone(orchestrator.Args)
		account.Handoff.Command = slices.Clone(account.Handoff.Command)
		account.Handoff.ReworkCommand = slices.Clone(account.Handoff.ReworkCommand)
		s.Problems = decodeOver(&c, s.RepoPath, s.Problems)
		s.Problems = foldLead(&c, s.RepoPath, s.Problems)
		if c.Orchestrator.Kind != orchestrator.Kind || !slices.Equal(c.Orchestrator.Args, orchestrator.Args) {
			s.Problems = append(s.Problems, fmt.Sprintf("%s: [orchestrator] kind and args are only read from config.toml — ignored", s.RepoPath))
			c.Orchestrator.Kind, c.Orchestrator.Args = orchestrator.Kind, orchestrator.Args
		}
		if c.Jira.URL != account.URL || c.Jira.EmailEnv != account.EmailEnv || c.Jira.TokenEnv != account.TokenEnv || !slices.Equal(c.Jira.Handoff.Command, account.Handoff.Command) || !slices.Equal(c.Jira.Handoff.ReworkCommand, account.Handoff.ReworkCommand) {
			s.Problems = append(s.Problems, fmt.Sprintf("%s: [jira] url, email_env, token_env and handoff.command and rework_command are only read from config.toml — ignored", s.RepoPath))
			c.Jira.URL, c.Jira.EmailEnv, c.Jira.TokenEnv = account.URL, account.EmailEnv, account.TokenEnv
			c.Jira.Handoff.Command, c.Jira.Handoff.ReworkCommand = account.Handoff.Command, account.Handoff.ReworkCommand
		}
		// It moves the reader's own checkouts, so it is theirs to turn on.
		if c.Pipeline.FastForward != ff {
			s.Problems = append(s.Problems, fmt.Sprintf("%s: [pipeline] fast_forward is only read from config.toml — ignored", s.RepoPath))
			c.Pipeline.FastForward = ff
		}
		// They choose programs to start, like [orchestrator] kind and args.
		if !sameAgent(c.Questions.Small, small) || !sameAgent(c.Questions.Large, large) {
			s.Problems = append(s.Problems, fmt.Sprintf("%s: [questions] small and large are only read from config.toml — ignored", s.RepoPath))
			c.Questions.Small, c.Questions.Large = small, large
		}
		// The token goes to this address, so a repository cannot pick it.
		if c.Bitbucket != server {
			s.Problems = append(s.Problems, fmt.Sprintf("%s: [bitbucket] url and token_env are only read from config.toml — ignored", s.RepoPath))
			c.Bitbucket = server
		}
		// The kind chooses where a token goes, and the command a program to
		// start.
		if c.Reviews.Kind != reviewKind || !slices.Equal(c.Reviews.Command, reviewCommand) {
			s.Problems = append(s.Problems, fmt.Sprintf("%s: [reviews] kind and command are only read from config.toml — ignored", s.RepoPath))
			c.Reviews.Kind, c.Reviews.Command = reviewKind, reviewCommand
		}
		// It starts a program, like [reviews] command.
		if !slices.Equal(c.Close.Check, closeCheck) {
			s.Problems = append(s.Problems, fmt.Sprintf("%s: [close] check is only read from config.toml — ignored", s.RepoPath))
			c.Close.Check = closeCheck
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
	s.Jira = c.Jira.WithDefaults()
	s.Bitbucket = c.Bitbucket.WithDefaults()
	s.Reviews = c.Reviews
	s.Questions = c.Questions.WithDefaults()
	s.Stack = c.Stack
	s.CloseCheck = c.Close.Check
	for _, id := range c.Accept.From {
		if id = strings.TrimSpace(id); id != "" {
			s.AcceptFrom = append(s.AcceptFrom, id)
		}
	}
	s.Broadcast = c.Broadcast.Presets
	if len(s.Broadcast) == 0 {
		s.Broadcast = DefaultPresets
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

# Keep the target branches up to date in your checkouts: after each fetch the
# board fast-forwards them to their remote. Only ever a fast-forward: a branch
# with commits of its own, or checked out where there are changes, is left
# alone and the board says so. U does it at once. Read from this file only.
# fast_forward = true

# Let a card be moved by hand to another column. It is marked as moved by
# hand, says where the board would put it, and follows the board again once
# the board agrees or computes another column twice in a row. false keeps
# every card where it is computed.
# manual_moves = true

# Branches pull requests merge into, most downstream first. publish is the
# build server job that deploys the branch; env is how the card names it.
# They are also the target branches H offers on a Jira card. With none, the
# board uses the remote's default branch (origin/HEAD), else main.
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
# command / relaunch_command: a program that starts the orchestrator in place of
# the board (o with none open / O), run without a shell in the worktree, with the
# same placeholders. Its last JSON line, or last line, is shown as the result.
# Once it has run its answer is final; if it cannot be started at all, or is
# unset, the board starts the orchestrator itself from kind and prompt.
# command = ["pwsh", "-NoProfile", "-File", "C:/path/Restart-Orchestrator.ps1", "-Key", "{key}", "-Worktree", "{worktree}"]
# relaunch_command = [..., "-Replace"]
# The boards save the orchestrator's conversation id in the run folder
# (orchestrator-session.json). o with none open starts a cursor or claude agent on
# it (--resume) and sends resume_prompt instead of prompt; O always starts a new
# conversation. If the old one cannot be reopened, a new one starts.
# resume_prompt = "You were closed and are resumed. Read {state} again and continue."

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
# closed = '(?i)^(decisions?|closed)\b'   # sections of answered questions
# for_you = '(?i)\bfor (you|me)\b'       # sections of questions for you, not another person
# asked = '^[^:(]*\(([^)]+)\)'          # who answers a question: "Product (Ana): ..." -> Ana
# A story code in the run's files, and the folder under spec_clone holding its
# HTML prototypes ({1}, {2}... are the code's groups). Off unless set.
# spec_code = '(?i)\b(E\d+)[_-](US|TS)[_-](\d+)'
# prototypes = "specifications/backlog/{1}"

# Jira, read only: the global board shows the tickets of jql that have no
# worktree yet, each in the column its Jira status maps to, and every card
# shows its ticket's Jira status. The account is read from two environment
# variables, never from a file: an Atlassian API token and the e-mail of the
# account it belongs to. Leave url out to turn Jira off.
# url, email_env, token_env and handoff.command are read from this file only;
# the rest can also go in a repository's .ekanban.toml.
# [jira]
# url = "https://your-site.atlassian.net"
# email_env = "JIRA_EMAIL"
# token_env = "JIRA_API_TOKEN"
# jql = "project = ABC AND assignee = currentUser() AND statusCategory != Done ORDER BY priority DESC, updated DESC"
# test_type = "Test"      # the issue type of linked test cases
# A ticket with no worktree goes by its status name, else its status category
# (new, indeterminate, done), to a column id; "" leaves it off the board.
# Defaults: new = "todo", indeterminate = "in_progress", done = "".
# [jira.columns]
# "In Quality Review" = "ready_qa"
# H on such a card asks for the target branch and the team, then runs command
# without a shell. Placeholders: {key} {summary} {target} {team} {type} {repo}
# ({type} is "fix" for the fix_types issue types, else "feat"; {repo} is the
# main checkout). A command that prints a JSON line with "code" and "message"
# has it shown on the status line.
# [jira.handoff]
# command = ["pwsh", "-NoProfile", "-File", "handoff.ps1", "-Key", "{key}", "-Summary", "{summary}", "-Target", "{target}", "-Team", "{team}", "-Type", "{type}"]
# teams = ["standalone", "small-team", "full-team"]
# fix_types = ["Bug", "Defect", "Defect Candidate"]
# With [stack] on, H on a card with a worktree and failed tests runs this, with
# the same placeholders plus {tests} (keys, comma-separated) and {evidence}.
# rework_command = ["pwsh", "-NoProfile", "-File", "handoff.ps1", "-Key", "{key}", "-Rework", "-Tests", "{tests}", "-QaComment", "{evidence}", "-Target", "{target}", "-Team", "{team}"]

# Bitbucket, read only: Jenkins turning green does not mean a pull request may
# merge, because Bitbucket also holds the reports other tools post on the
# commit (SonarQube's quality gate), the reviewers' verdicts and its own merge
# checks. With this on, each card's pull request says whether it is fit to
# merge, or what is in the way. The project and repository are taken from
# pipeline.pr_url. The token is read from an environment variable, never from
# a file, and is sent only to url; both are read from this file only.

# Review requests, read only: the open pull requests waiting on your review.
# A card whose ticket has one says so and asks for you; one with no card gets
# a card of its own in column (on_review unless set; "" only marks the cards
# already there). kind picks where they come from: "bitbucket" uses
# [bitbucket] and the repository in pipeline.pr_url; "command" runs command
# without a shell, in the repository, and reads a JSON array of objects with
# id, title, branch, target, author, status, url and updated (only id is
# required). kind and command are read from this file only.
# [reviews]
# kind = "bitbucket"
# column = "on_review"
# GitHub, through the gh CLI:
# kind = "command"
# command = ["gh", "pr", "list", "--search", "review-requested:@me", "--json", "number,title,headRefName,baseRefName,author,url,updatedAt", "--jq", "[.[] | {id: .number, title, branch: .headRefName, target: .baseRefName, author: .author.login, url, updated: .updatedAt}]"]
# GitLab, through the glab CLI and jq:
# command = ["pwsh", "-NoProfile", "-Command", "glab mr list --reviewer=@me -F json | jq '[.[] | {id: .iid, title, branch: .source_branch, target: .target_branch, author: .author.username, url: .web_url, updated: .updated_at}]'"]

# The global board's broadcast (B) says one message to every orchestrator
# open, each getting it as a message of its own. presets are ready-made ones;
# {target} is each ticket's target branch and {key} its key. A repository's
# .ekanban.toml may set them for its team.
# [broadcast]
# presets = [
#   "Fetch and rebase your branch onto origin/{target}, run the checks, and push.",
#   "Stop where you are and tell me in two lines what is done and what is left.",
# ]

# Closing a finished ticket (C, or ekanban close) checks it is committed,
# pushed and merged, then runs check, when set, in its worktree: exit 0 lets
# the close go on. {key} {worktree} {branch} {target} {pr} {repo} are filled
# in. Read from this file only, since it starts a program.
# [close]
# check = ["pwsh", "-NoProfile", "-File", "C:/tools/shipped.ps1", "{key}"]

# The columns a ticket may be accepted (a) from; the last one alone when unset.
# [accept]
# from = ["to_deploy", "ready_qa"]

# A run's open questions. r on a question (Reply) types reply, or reply_you
# for a question for you, into the ticket's orchestrator, unsent: {to} is who
# answers it, {brief} its brief's path from the worktree, {question} its words.
# D on a question with a brief (Discuss) opens a tab, starts small or large by
# the brief's needs, and tells it to read discuss ({brief} is the brief's full
# path, the rest as in [orchestrator] prompt). small and large are read from
# this file only; the messages can also go in a repository's .ekanban.toml.
# [questions]
# reply = "Answer from {to} to the open question {brief}: "
# reply_you = "My answer to the open question {brief}: "
# discuss = "Read {brief} in full. It is the context of one open question on {key}. Follow its ## Kickoff."
# tab = "question"
# small = { kind = "cursor", args = ["--model", "some-small-model"] }
# large = { kind = "cursor", args = ["--model", "some-large-model"] }

# [bitbucket]
# url = "https://bitbucket.example.com"
# token_env = "BITBUCKET_TOKEN"   # a personal access token that can read the repository

# Stacked cards: related tickets drawn as strips under a card. j/k and the
# arrow keys walk through them before leaving the card, a click selects one,
# and the detail shows the same stack. Off until sources names one; a source
# is where a stack comes from. Known: failed-tests.
# [stack]
# sources = ["failed-tests"]
# max_items = 8       # strips a card draws; the rest show as "+N more"
# collapse = true     # a card that is not selected shows "+N failed" instead of its strips
# column = "todo"     # a card with a stack moves to this column; "" leaves it where it is
# failed-tests: the tests linked to the card's ticket that QA failed or blocked.
# from = "comment" reads QA's comments (a test named by its key, or by the
# code in its summary, on a line that says failed or blocked; a later line that
# says passed takes it off); from = "linked" takes every linked test, for teams
# that only link a test once it failed. The words are regular expressions.
# [stack.source.failed-tests]
# from = "comment"
# failed = '(?i)\bfail(ed|s|ing|ure)?\b'
# blocked = '(?i)\bblock(ed|s|ing)?\b'
# passed = '(?i)\bpass(ed|es)?\b'

# Keys, by action name: one key or a list. A rebound action stops answering
# its old key, and an empty list turns it off. The help screen (?) shows the
# current keys and the names are listed by ` + "`ekanban keys`" + `. gg, gp,
# gf and 1-9 are fixed. [keys.board] and [keys.ticket] apply to one board only.
# Some actions belong to what is selected (a question, say): bound by name like
# the rest, their keys win there over the board's, and only there.
# [keys]
# accept = "Z"
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

func sameAgent(a, b orchestrator.Agent) bool { return a.Kind == b.Kind && slices.Equal(a.Args, b.Args) }
