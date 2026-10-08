# ekanban

[![ci](https://github.com/ezemacchi/ekanban/actions/workflows/ci.yml/badge.svg)](https://github.com/ezemacchi/ekanban/actions/workflows/ci.yml)

A [Herdr](https://herdr.dev) plugin: a status board over your spaces, in a popup,
on a key.

Herdr's own space list tells you what Herdr knows — label, panes, agent state.
This adds the part only you know: what you've actually started, what's finished,
and what's parked because you're waiting on a person or something outside the
machine.

```
 ▾ List                                                    🔔 1 space · 4 live · archive hidden

 ▾ Triage (0)
 ▾ Todo (0)
 ▾ In Progress (2)
   dev-stream             ~/src/github.com/phin-tech/dev-stream                      ·working
   ekanban       ~/src/github.com/ezemacchi/ekanban                   ·idle
 ▾ Waiting (2)
   🔔 docs-site           vendor SLA response, chased 2026-07-18                     ·blocked
 ❯ api-gateway            waiting on Dave re: API key                                   ·idle
 ▸ Done (0)

 1 Triage  2 Todo  3 In Progress  4 Waiting  5 Done
 K table · d detail · v move · n note · enter jump · ? help
```

Statuses are yours: rename them, reorder them, invent new ones. The dim right
column is Herdr's agent state — a hint only. It never groups, sorts, or
overrides anything you set.

`K` cycles the same board through three views. The **table** is the flat one —
every space on a line, in aligned columns, including the fields the list has no
room for:

```
 Board                                                                            5 live · archive hidden
   SPACE                ↓STATUS     NOTE                                              AGENT    CHANGED
   dev-stream           In Progress —                                                 idle     just now
   ekanban     In Progress —                                                 working  2h ago
 ❯ api-gateway          Waiting     waiting on Dave re: API key rotation — he's back  idle     10m ago
   docs-site            Waiting     vendor SLA response, chased 2026-07-18            blocked  1d ago
   billing              Done        —                                                 idle     3d ago
```

No groups, no collapse, and it's the only view you can re-sort: `o` cycles
status → name → changed, and `↓` marks the column in force. Sorting by status
lays the rows out exactly as the list groups them, so `v` still works there.

The **kanban** is columns, one per status:

```
 Board                                                                          5 live · archive hidden

 Todo 0                   In Progress 2            Waiting 2                Done 1
 ──────────────────────── ──────────────────────── ──────────────────────── ────────────────────────
 —                          dev-stream             ❯ api-gateway              billing
                            ·idle                    waiting on Dave re:      ·idle
                                                     API key rotation
                            ekanban         ·idle
                            ·working
                                                     docs-site
                                                     vendor SLA response,
                                                     chased 2026-07-18
                                                     ·blocked
```

A column *is* a status, so `v` then `h`/`l` walks a card sideways to retag it,
and `j`/`k` reorders within the column. The view you were last in is remembered.

Rows can only ever show a truncated note, so the list keeps a detail pane
alongside. It tracks the cursor with no keypress needed, and shows the note in
full along with the path, workspace, and when the status last changed. `d` hides
it if you want the room back. In kanban the columns already use the width, so
`d` opens the same detail as a modal instead — and you can keep browsing with
`j`/`k`, or edit with `n`, without closing it.

## Pull request context

If a space's directory has a pull request for its current branch, the board
shows it: number, state, review decision and CI checks. Worktrees are the case
this suits best — one branch per space, so one PR per row.

```
   SPACE                STATUS      NOTE                          PR                   AGENT
   billing              In Progress —                             #130 ● changes ·     working
   docs-site            Waiting     —                             #119 ○ ✗             blocked
 ❯ api-gateway          Waiting     waiting on Dave re: API key   #123 ● approved ✓    idle
```

`●` open · `○` draft · `◆` merged · `✕` closed, then the review decision, then
checks `✓` pass `✗` fail `·` running, then `conflict` or `behind` when the
branch cannot land as it stands. A mergeable branch says nothing — the column
is for what needs doing, and GitHub computes mergeability lazily, so an
un-computed state is left blank rather than guessed. A row is coloured by whatever most needs
attention: failing checks first, then changes-requested. `gp` opens the
selected space's PR in a browser.

**PR state is context, never control.** It never sets a status, moves a row, or
reorders anything — the same rule the agent hint follows. You drive status; this
just tells you what GitHub thinks while you decide.

`gf` takes the failing check further: it hands the agent in that space the
check's name, the end of its log, and the link — typed into its input, unsent,
like everything else here.

```
CI is failing on #123: build (and 1 other check)

--- FAIL: TestBoardKeepsOrder (0.00s)
    board_test.go:88: order = [b a], want [a b]
FAIL	github.com/o/r/internal/board	0.312s

https://github.com/o/r/actions/runs/1/job/2
```

The log comes from `gh run view --log-failed`, trimmed to the end of the failed
step, with the timestamps, group markers and colour codes taken out. Checks
outside Actions — CircleCI, Buildkite — have no log to fetch, so those send the
name and the link, which is what you would have copied anyway.

## Talking to a space's agent

`m` types a message into the agent running in the selected space and takes you
there — **without submitting it**. You read it, add a line, press enter. The
board is where you notice that something is blocked; this is how you say
something about it without hunting for the right window.

It refuses rather than guesses. Panes with no agent are skipped (shells, plugin
sidebars, this board), and a space running two agents is a refusal, not a coin
flip: typing a review comment into the wrong agent is worse than not sending it.

## How pull requests are found

The PR comes from the space's branch, and once found, from its own URL — which
still reaches it after the branch has moved on. A merged or closed one goes back
to the branch, so the row finds whatever replaced it.

It reads through the `gh` CLI, so it uses your existing login and needs no
token. Results are cached beside the board and refreshed when you open it, so
the board paints instantly and fills in as answers arrive.

Nothing is refetched on Herdr's event stream: an event says the workspace list
may have changed, which is no reason to re-ask GitHub, re-read a worktree, or
re-push a token Herdr already has. A space that has just opened is worked up at
once; everything else ages out on its own clock, or on `r`. On a session of
thirty spaces with agents running, that is the difference between a board that
idles and one that keeps Herdr busy for as long as it is open.

No repo and no PR look the same — an empty column. A **missing or logged-out
`gh` says so once**, because Herdr launches plugins with a minimal PATH: left
silent, the whole feature would vanish and look identical to having no PRs.

The short form is also pushed as a `$pr` token, if you want it in the sidebar:

```toml
[ui.sidebar.spaces]
rows = [
  ["state_icon", "workspace"],
  ["branch", "$status"],
  ["$pr"],
]
```

## Notifications and the bell

A watcher polls your spaces' pull requests in the background, every two minutes,
and raises a Herdr notification when something actually changes:

| | |
|---|---|
| checks pass → fail | the thing you were waiting on just broke |
| a review lands | approved, or changes requested |
| clean → conflict | needs a rebase before it can land |
| merged or closed | the work landed, or didn't |

**Only changes, never states.** A failing check is announced once, not on every
poll — notifying on state trains you to ignore the notifications.

Herdr toasts are transient: fired while you are away, they are gone. So every
notification is also recorded, and the space wears a 🔔 until you look at it.
Selecting the row clears it; the count in the header means a bell inside a
collapsed group or an off-screen column is still visible. The detail view spells
out what happened, and names the checks that are failing rather than just saying
some are.

The watcher starts when Herdr does, through a `[[startup]]` hook, and again
after a live handoff — so a pull request going red reaches you whether or not
you have opened the board. Opening the board starts one too, if none is
running. It holds an event subscription, which does double duty: Herdr closing
drops the connection so the watcher exits at once rather than discovering the
loss on its next tick, and a workspace appearing or closing nudges it to poll
then instead of waiting out the timer. A burst of events still costs one poll.

Lookups are both bounded and paced: a few calls run at once, and a token bucket
meters how fast they leave. Concurrency alone is not enough — four workers will
still fire fifty requests at a fifty-space board as fast as they can cycle.
Foreground and background differ on purpose: with the board open you are waiting
for an answer, so it favours latency; the watcher has two minutes and nobody
looking, so it trickles.

A lockfile means there is only ever one watcher, however many times you open the
board, and it covers every space across every repo. Run it by hand with
`ekanban watch` if you would rather.

It follows the Herdr session that started it. If you run named sessions
side by side, spaces in the other one are outside its view.

## Inside a repository

Open the board in a repository and its columns stop being statuses you set: they
are where each ticket is in delivery, read from git and the build server.

```
 To Do 1          Working 2           Reviewing 3          Shipping 0   To QA 2
 ABC-12           ABC-7               ABC-9                —            ABC-3
 workspace clo…   Sticky header       PR #184 · build ok                PR #136
                  Implementer                                           shipped to dev
```

A card is a ticket worktree, named by its ticket key. Its column comes from a
chain of rules: deployed, merged, not started, working, landed. The first rule
that decides wins. `[pipeline]` in the settings names the build server, the
branches pull requests merge into, and the publish job that deploys each one.
A card also shows the loudest agent of its run (the one that needs you, or the
one that just finished) and the board's footer notes each change.

### The ticket board

A team run keeps a folder in its worktree, `.runs/<KEY>/STATE.md` by default.
The ticket board opens as a tab of the ticket's workspace and shows one card per
role of the run's team: pending, working, waiting on you, done. Under it are the
run's objective, current step and open questions, and a list of what the agents
did. `enter` goes to a role's tab, and `o` goes to the agent running the team
(its lead), or starts one with a prompt to resume the run.

Teams are TOML files. One example team ships with the binary; yours go in the
`teams` folder beside `config.toml`. Where runs live and how their state file
reads is `[run]` in the settings, so another team's layout needs no code.

Both boards take the mouse: click a card to select it, click again to go there,
and click the buttons along the bottom.

## Install

This fork runs on Windows, and installing it compiles it, so Go must be on
`PATH`:

```powershell
herdr plugin install ezemacchi/ekanban --ref windows -y
```

For local development, point Herdr at a working tree instead. No build runs,
so you compile it yourself:

```powershell
git clone -b windows https://github.com/ezemacchi/ekanban
cd ekanban; go build -trimpath -o bin\ekanban.exe .\cmd\ekanban
herdr plugin link .
```

`AGENTS.md` walks a coding agent through installing and setting it up.

Then bind a key in `%APPDATA%\herdr\config.toml`:

```toml
[[keys.command]]
key = "prefix+d"
type = "plugin_action"
command = "ekanban.open"
description = "Space board"
```

and reload: `herdr server reload-config`.

`prefix+d` is unbound in Herdr's default keymap. A `keys.command` entry silently
shadows a built-in, so check the config reference before picking another —
`prefix+b` is `toggle_sidebar` and `prefix+k` is `focus_pane_up`, both easy to
lose by accident.

Requires Herdr 0.7.5+ and Go.

## The board beside your panes

The board also renders narrow, for a region down the right-hand side of the
window: the grouped list only, with no detail pane and no layout switching — a
narrow board has no room for the table and kanban arrangements, and switching
there would overwrite the layout the popup remembers.

One action opens it:

```powershell
herdr plugin action invoke dock --plugin ekanban
```

or bind it, the same way as `open`:

```toml
[[keys.command]]
key = "prefix+D"
type = "plugin_action"
command = "ekanban.dock"
description = "Board on the right"
```

It opens as a split to the right of the focused pane. (Upstream's
`action-dock.sh`, which also reaches hoarder's right-hand dock, needs `sh` and
is not used by this fork's manifest.)

### What the narrow rows show

The popup's row spends 22 columns on a padded name whatever the width is, and
gives the rest to a path. Neither survives down the side of a window, so the
narrow row is built the other way round — the name takes what is left after the
markers, and each thing beside it earns its room:

| column | |
|---|---|
| name | always; everything else gives way to it |
| note, else branch | what you are waiting on, or which worktree this is — only when there is enough room to read one |
| `#123 ●✓` | the pull request, dropped first when the row runs out |
| `◐ ◆ ✓ · ○` | the agent: working, blocked, done, idle, offline |

A filter has no header to live in here, so it shows along the bottom instead.

## Status in the Spaces sidebar

The board mirrors each status into the workspace's `status` metadata token, so
it can show in Herdr's native Spaces sidebar too. Add `$status` to a row:

```toml
[ui.sidebar.spaces]
rows = [
  ["state_icon", "workspace"],
  ["branch", { token = "$status", fg = "yellow", bold = true }],
]
```

Herdr 0.7.5 added per-token styling, so a row entry can be
`{ token, fg, bold, dim }` instead of a plain string.

Tokens don't survive a server restart, but the board file does — a
`workspace.created` hook re-applies the stored status whenever a space appears,
so the badge is correct even if you never open the board.

**The default status gets no badge.** Every space you have never touched sits
there, so badging it would put the same word on every sidebar row while telling
you nothing — and it could not distinguish "I filed this as Todo" from "I have
never looked at this". That is why the shipped set leads with **Triage**: it
means *not looked at yet*, which leaves Todo free to mean a decision you
actually made, badge and all.

Which status is the default is an explicit choice, not a position: press `D` on
one in the `S` manager. It is marked there, and it can sit anywhere in the
order, so rearranging the board never silently changes which status goes quiet.
A board that has never named one falls back to the first.

## Settings

Everything works without configuration. To change something, Herdr gives the
plugin its own config directory, which survives reinstalls:

```sh
ekanban config --init   # write a commented template
ekanban config          # show what is in force, and from where
```

That lands at `%APPDATA%\herdr\plugins\config\ekanban\config.toml`
(`herdr plugin config-dir ekanban` prints the folder):

```toml
# How often the background watcher asks GitHub about your pull requests.
# Minimum 30s, maximum 1h. Opening or closing a workspace polls immediately
# regardless, so this only governs noticing a review landing or CI going red.
poll_interval = "2m"

# Herdr toasts when a pull request changes. Bells on the board are recorded
# either way, so turning this off makes the board quiet rather than blind.
notifications = true
```

A value that is out of range is clamped, and one that makes no sense falls back
to its default — but either way it says so on stderr rather than ignoring your
typo. A broken file never stops the board.

This is deliberately separate from `board.json`: that is state the board writes
for itself, this is what you tell the board, and it is never overwritten.

### A repository's own settings

A repository can keep a `.ekanban.toml` at its root with any of the same
settings. It is read over `config.toml` whenever a board opens in that
repository or one of its worktrees, so the project's process lives with the
project: tracker and pull request links, build server, target branches, the
`[run]` layout, the prompt a new lead starts with. A value there replaces the
personal one, and a list replaces the whole list. `[lead] kind` and `args` are
read from `config.toml` only, since they choose a program to start.

Commit it to share it with the team, or list it in `.git/info/exclude` to keep
it to one clone. An uncommitted file is only in the main checkout, and boards in
linked worktrees look for it there.

## Keys

| Key | |
|---|---|
| `K` | cycle the view: list → table → kanban (or click the title) |
| `o` | table only: sort by status, name, or when it last changed |
| `d` | list: show or hide the detail pane · elsewhere: detail modal |
| `j` / `k` | move |
| `gg` / `G` | first row · last row |
| `gp` | open the pull request in a browser |
| `gf` | send the failing check, and the end of its log, to that space's agent |
| `h` / `l` | kanban: move between columns · list: collapse / expand a group |
| `v` | grab the row, then move it — leaving its group changes its status |
| `enter` | jump to the space (reopens archived ones at their old path) |
| `1`–`9` | send to that status; the numbers are listed along the bottom |
| `s` | status picker |
| `n` | edit the note — who or what you're waiting on |
| `R` | rename the space — renames the Herdr workspace too |
| `m` | type a message into that space's agent, then go there to send it |
| `space` | collapse / expand a group |
| `F` | show only the status under the cursor — `F` or `esc` for all |
| `O` | reorder Herdr's own Spaces sidebar to match this board |
| `a` | show or hide archived spaces |
| `/` | filter by name, path, or note |
| `S` | manage statuses: add, rename, reorder, delete, set the default |
| `x` | forget the selected space |
| `r` | refresh |
| `q` | quit |

In a repository the board adds its own: `a` accepts a ticket in the last column
and moves it to the archive, `A` opens that archive, `o` goes to the ticket's
lead (so sorting the table moves to `ctrl+o`). `ekanban keys` lists every
screen's keys, and `[keys]` in the settings rebinds any of them.

## Mouse

The board takes the mouse, so Herdr never sees a click inside it — everything a
pointer can do here, the board does itself.

| | |
|---|---|
| wheel | scroll the list, leaving the cursor where it is |
| click a group header | collapse or expand it |
| click a PR or a check | open it in a browser |
| click the title | open the view switcher (popup only) |
| click a space | **docked:** go there · **popup:** select it |
| click it again | popup: go there |

Clicking a space does different things on the two boards on purpose. The dock
is a strip you glance at on your way somewhere, so one click goes there —
asking for a second would be a second click on every jump you make all day.

The popup is where rows are worked on rather than travelled to: click a row,
then press `2`, or `n`, or `v`. A click that jumped would close the whole board
under you, so there the second click is what activates.

Either board closes once you jump. You opened it to get somewhere, and once you
are there it is a strip of screen showing you a board you have stopped reading.
A key brings it straight back.

Scrolling leaves the cursor alone either way: the cursor is what the keys act
on, and having a look further down the board should not change what `1` or
`enter` would do.

Taking the mouse also takes drag-to-select; most terminals still allow it with
shift held.

## Filtering and ordering

`F` narrows the board to whichever status the cursor is on — no picker, since
you are already standing on the group you want. `F` again, or `esc`, restores
everything. It applies to all three views, and the header says so, because an
empty board that does not explain itself just looks broken.

`O` pushes the board's order onto Herdr: the Spaces sidebar is reordered to
match, statuses first and then however you arranged them by hand.

That is a deliberate keypress rather than something the board does
continuously, because reordering somebody's sidebar every time a status changed
would fight anyone who arranges their spaces themselves. Herdr can filter its
*Agent* panel by a metadata token — the board's `$status` works there — but not
its Spaces panel, and plenty of spaces have no agent at all. Moving workspaces
is the only mechanism that reaches Spaces.

## How state works

Herdr workspace ids (`w4`, `w5`) are reassigned every session, so they're
useless as a durable key. Statuses are keyed by **canonical directory path**
instead: a status set today is still on the project when you reopen it next
week, in a new session, with a different workspace id.

Two axes are deliberately kept apart:

- **Status** is yours, and it groups the board. A live space marked Done sits in
  the Done group.
- **Liveness** is Herdr's, and it decides main list vs archive. Close a space in
  Herdr and it moves behind `a` with its status intact; `enter` reopens it.

Everything lives in one file, `$HERDR_PLUGIN_STATE_DIR/board.json` — status
definitions and per-directory entries together, written atomically. It's
hand-editable if you'd rather.

Rows sort by most-recently-touched until you arrange a column by hand with `v`.
After that the arrangement sticks: hand-ranked rows hold their positions at the
top of the group, and anything you haven't touched falls in below them by
recency. Rearranging a row doesn't count as working on it, so it won't disturb
that fallback ordering.

Several workspaces open on the same directory share one row, because a status
belongs to the project rather than the window.

```sh
ekanban sync            # re-apply stored statuses to workspace tokens
ekanban startup         # what Herdr's [[startup]] hook runs
ekanban watch           # poll PRs and notify (the board starts this for you)
ekanban ticket [dir]    # the ticket board of the run in a worktree
ekanban pipeline [repo] # where each worktree of a repository lands, as JSON
ekanban config          # show the settings in force, and the repository file
ekanban config --init   # write a commented settings template
ekanban keys            # every action and the keys bound to it
ekanban version         # which build this is
ekanban prune           # forget entries whose directory no longer exists
```

## Releasing

A push to `main` republishes the rolling `latest` prerelease. A `v*` tag cuts a
permanent one, stamps that version into the binary, and commits the matching
`version` back to `herdr-plugin.toml` — Herdr reads that file to report what is
installed, so a stale one would claim the wrong version for ever.

```sh
git tag v0.2.0 && git push origin v0.2.0
```

CI builds through `build.sh` and fails if the built binary disagrees with the
manifest, so the two cannot drift apart unnoticed.

## Development

```powershell
go test ./...
go build -trimpath -o bin\ekanban.exe .\cmd\ekanban
.\bin\ekanban.exe      # runs against the live session via $env:HERDR_SOCKET_PATH
```

`AGENTS.md` has the checks a change must pass and the rules for this
repository.

Run the binary directly from any pane inside a Herdr session — it doesn't need
to be installed as a plugin to work, which makes for a fast inner loop.
