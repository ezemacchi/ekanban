# ekanban

[![ci](https://github.com/ezemacchi/ekanban/actions/workflows/ci.yml/badge.svg)](https://github.com/ezemacchi/ekanban/actions/workflows/ci.yml)

A [Herdr](https://herdr.dev) plugin: a status board over your spaces, in a popup,
on a key.

ekanban started as a fork of [herdr-phin-board](https://github.com/phin-tech/herdr-phin-board)
by Phin Technologies, under the MIT license (see `LICENSE`).

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
there — **without submitting it**. On the board of tickets that agent is the
ticket's orchestrator, found as the broadcast finds it, so role agents open
beside it do not get in the way. You read it, add a line, press enter. The
board is where you notice that something is blocked; this is how you say
something about it without hunting for the right window.

It asks rather than guesses. Panes with no agent are skipped (shells, plugin
sidebars, this board). When a ticket has no orchestrator open, or a space runs
several agents, a chooser lists the agents there, each by its tab, with the
last lines of the highlighted one's pane beside it, read live, so you can see
which is which; the one you choose gets the message, this once. Nothing is
renamed or remembered: typing a review comment into the wrong agent is worse
than asking.
An agent waiting at a prompt is refused too: what you type would land in its
dialog. Answer the prompt first.

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

Open the board in a repository and its columns are where each ticket is in
delivery, worked out from git and the build server rather than set by you. You
can still move a card by hand when you know better; the board marks it and
takes over again as soon as it learns something new.

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

A pull request counts as merged when its merge commit holds the worktree's
commit, or when the merge's subject names the worktree's branch. The second
covers a branch rebased on the server before it merged: the card then says
*worktree differs from what merged*, the cue to check that nothing left only
in the worktree was lost. Both hosts' merge commits are read: Bitbucket Server
("Merge pull request #12 in P/repo from feature/x to main") and GitHub ("Merge
pull request #12 from owner/feature/x"). A squash merge, or Bitbucket Cloud's
"Merged in feature/x (pull request #12)", leaves no such commit, so those pull
requests are not seen as merged.

When you know better than the board, move the card by hand. With the mouse,
**Move** in the footer or in the card's `⋯` menu opens a list of the columns;
click one. With the keys, `s` opens the same list, `1`–`9` sends the card
straight to a column by its position, and `v` then `h`/`l` carries it
sideways. The list marks the column the board would choose with *· board*.

A card moved by hand carries a hand in its border (✋ without Nerd Font icons)
and a line saying where the board would put it, *board: Reviewing*. The mark
goes on its own as soon as the board computes the column you chose. If the
board computes some other column, the card follows it once that column is read
twice in a row; a single read is held back, since a git command that failed or
a run file caught mid-write can change the column too. To put it back
yourself, use **Undo** on the card (`b`), or move it to the column the board
says. A card moved into the last column can be accepted like
any other. Every key named here can be rebound in `[keys]`, and the board's
buttons and messages show your binding.

A team that wants columns strictly computed turns this off in its
repository's `.ekanban.toml`:

```toml
[pipeline]
manual_moves = false
```
A card reads in three steps. Its key sits in the top border with the ticket's
type, and its title under it. A card that asks for you (an agent waiting on you,
a failed build, a pull request Bitbucket blocks) has a red border and a red dot,
and the top bar counts them: click the count, or press `!`, to go to the next.
Under the title is the one fact that matters most, such as the loudest agent of
the run or the pull request and its build. The selected card opens up to show
every fact, with buttons for what it can do (go there, the ticket, the pull
request) and `⋯` (or `.`) for a menu of everything else. The board's footer
notes each agent change.

### The ticket board

A team run keeps a folder in its worktree, `.runs/<KEY>/STATE.md` by default.
The ticket board opens as a tab of the ticket's workspace and shows one card per
role of the run's team: pending, working, waiting on you, done. Under it are the
run's objective, current step and open questions, and a list of what the agents
did. `enter` goes to a role's tab, and `o` goes to the agent running the team
(its orchestrator), or starts one with a prompt to resume the run. `o` only goes
to an orchestrator that is open, so for one that is dead, hung at an empty
prompt, or on the wrong model there is `O` (the orchestrator line's Relaunch
button). It asks (Y)es/(N)o first, then quits the running agent with two
`ctrl+c`, keeps its tab, and starts a new agent there with the same resume
prompt `o` uses. The new agent is a new conversation, not a resumed one: it reads
the run's files to see where the run stopped. An agent that does not quit is
reported, never killed, and nothing starts; with no orchestrator open it reuses
the empty `orchestrator` tab a dead one left, or opens a new tab. A prompt whose
Enter the CLI dropped just after starting is submitted with a later Enter. Which
model it runs on comes from `args` in `[orchestrator]` of `config.toml`, such as
`args = ["--model", "<model>", "--trust"]` for the Cursor CLI.

The ticket and the prototype are buttons under the objective, and the
orchestrator's line has a button that goes to it. A finished role takes one
line; a role waiting on you has a red border. The columns and the Now, Activity
and Open questions sections fold with a click on their arrow, and the run's
`**bold**` and `` `code` `` are drawn rather than shown as marks.

A run asks two kinds of question. **Questions for you** -- which model runs a
role, approving a scope change -- go under a `## Questions for you` section and
are drawn first, in the attention colour. The others, for the people who answer
for the product or the architecture, go under `## Open decisions`, grouped by
who answers each: the name in the label's brackets (`Product (Ana): ...`), or
the brief's `to:`.

Each question is one line, and may link a **brief**: a file in the run folder
with the question's detail, so the state file every agent reads stays short.

```markdown
## Questions for you
- Which model runs the implementer? [brief](questions/implementer-model.md)

## Open decisions
- Product (Ana): which caption on the empty list? [brief](questions/empty-caption.md)
```

The brief starts with a few `key: value` lines between `---` lines, which the
board reads; the rest is the question's own sections (why it blocks, options,
sources, the message sent, a kickoff for a discussion):

```markdown
---
to: Ana
role: Product
status: open        # draft, open or closed
asked: 2026-01-02
chat: https://chat.example.com/new?to=ana&message=...
needs: small        # small: the brief is enough to discuss it; large: the code and specs too
---
```

A question's chat -- a link on its line, `[label](url)` or a bare URL, as older
runs write it, or the brief's `chat:` -- is a button after it; a click opens it.
`a` opens it too, from the keyboard: it asks first when there is one, and offers
a menu when there are several.

`j` goes on from a column's last role to the questions. On a selected question:

- `enter` opens its chat once you say yes; on a question for you, it replies.
- `r` replies: it types into the orchestrator the opening of an answer that
  names the question's brief, `Answer from Ana to the open question
  .runs/ABC-1/questions/empty-caption.md: `, unsent, and takes you there. You
  add the answer and send it, and the orchestrator knows which question it is
  whatever it was doing. On a role, `r` refreshes as everywhere else.
- `D` discusses it: a new tab with an agent of its own, started from the brief,
  for when the person asked answers with a question back. The brief's `needs`
  picks the agent, `[questions] small` or `large` in config.toml.
- `x` closes it without an answer, when nobody needs it any more (the ticket
  moved on, say). It asks why, "no longer needed" already written, and once
  you say yes the board closes it itself: the line moves to `## Decisions` as
  `... Closed <date> by the user (board), not answered: <reason>.` and its
  brief is closed with the same outcome. No agent is asked, even with the
  orchestrator running: moving a line is not worth an agent's tokens, and the
  orchestrator reads it closed the next time it reads the state file.
- `y` copies it.

The same from a terminal, with no board and no agent:

```
ekanban.exe questions [worktree]                     # the open questions, numbered
ekanban.exe questions close 2 --reason "in QA"       # close one without an answer
ekanban.exe questions close which-caption            # ... by its brief's name
```

The board says when a question and its brief disagree: a brief that is missing,
still a draft or already closed, an open brief with no chat for someone other
than you, or a line that reads as answered only because of a word in its link.
The question gets a `!` (the warning glyph with icons on), the selected one says
what is wrong, and the footer counts them.

Answered questions are listed under Closed questions, folded at first, with
their count in the title; a click on its arrow or `c` unfolds it. They are the
bullets of a `## Decisions` (or `## Closed ...`) section, then the bullets of
the open sections marked settled. Writing who answered and how in each keeps
the history readable, and a closed question's links are left out:

```markdown
## Decisions
- Product: which caption? Closed 2026-01-02 by Ana (chat): "No items yet".
```

The title line and the buttons stay put; everything between them scrolls with
the wheel when the run is taller than the pane, and moving the selection keeps
the selected card in view.

Teams are TOML files. One example team ships with the binary; yours go in the
`teams` folder beside `config.toml`. A team's `[orchestrator]` table says how
its orchestrator is recognised and started. Where runs live and how their
state file reads is `[run]` in the settings, so another team's layout needs no
code.

Both boards are drawn from the same pieces — columns, cards, buttons, menus,
fields — and take the mouse: click a card to select it, click again to go
there, and click any button; each one shows the key that does the same.

### Jira

With `[jira]` set, the global board reads your tickets from Jira. Read only: it
never writes to Jira.

- A ticket of your query that has no worktree yet is a card of its own, in the
  column its Jira status maps to. By default a status in the category *new*
  goes to To Do and one in *indeterminate* (in progress, in review) to Working;
  a done one is left off. `[jira.columns]` maps a status name or category to any
  column, so a ticket waiting on QA can sit in To QA. A ticket in
  implementation is therefore never shown as not started.
- The selected card and the detail show the ticket's Jira status, and in the
  detail the status line opens the ticket on click.
- The detail (`d`) adds the ticket's linked test cases, with their states, and
  the latest comment, which is where QA writes what failed.
- `H` on a card with no worktree asks for the target branch and the team, shows
  the command it will run, and runs `[jira.handoff] command` on `enter`: the
  board does not know how work is started, your command does. It is run
  without a shell, so a ticket's summary is one argument whatever it contains.
  The target branches are the `[[pipeline.target]]` ones. With none, the board
  uses the remote's default branch (`origin/HEAD`), else `main`, and the
  handoff box says it is a default rather than a choice.
- `y` on any card, or in its detail, copies what another session needs to pick
  it up: the ticket and its status, the spec and prototype the run names, the
  worktree and branch, the pull request and its build, the run's current step
  and open questions, the linked tests, and the latest comment in full.
- `t` opens the card's Jira ticket in the browser, from `issue_url`, on a card
  with no worktree too. `p` opens the ticket's HTML prototype (the one its
  spec names; a card with no worktree has none). Both work in the card's
  detail (`d`) as well, and the selected card, its menu and the detail name
  them.
- `P` opens the card's pull request in the browser, from `pipeline.pr_url`;
  `gp` does the same. It used to be `p`, which is now the prototype.

The account is two environment variables, named by `email_env` and `token_env`
(an Atlassian API token); the token is never in a file, and a repository's
`.ekanban.toml` cannot change the site, the variables or the handoff command.
Without them the board says so on its status line and works as before.

### Stacked cards

A card can carry related tickets under it as strips, like cards laid one over
another with only their bottom edge showing: the ticket key and one word. It
is off until `[stack]` names a source, and nothing else on the board changes
while it is off.

```toml
[stack]
sources   = ["failed-tests"]   # what builds a card's stack; empty = off
max_items = 8                  # strips drawn; the rest show as "+N more"
collapse  = true               # a card that is not selected shows "+2 failed"
column    = "todo"             # optional: bring a card with a stack back to this column
[stack.source.failed-tests]
from = "comment"               # or "linked": every linked test counts
```

The one built-in source, `failed-tests`, reads the Jira ticket's linked tests.
With `from = "comment"` (the default) a test counts as failed when a comment
names it, by its key or by the `[code]` in its title, on a line that says it
failed; a later line saying it passed clears it. With `from = "linked"` every
linked test counts, for teams that only link a test when it fails. The words
that mean failed, blocked or passed are settings of the source too (`failed`,
`blocked` and `passed`, regular expressions). Another source is a Go type
registered by name in `internal/stack` (`stack.Register`); the board only
knows the `Source` interface.

With a stack on the selected card:

- `j` / `k` or the arrows walk it: down goes from the card to each strip and
  then on to the next card; up goes back, and from a card above's last strip
  returns into it. A line under the strips says where each direction leads
  (`↑ ABC-1 · ↓ ABC-11`, `↓ next card` at the end).
- `t` opens the strip under the cursor in the tracker (the card's own ticket
  when the cursor is on the card). The three actions are `stack-up`,
  `stack-down` and `stack-open` in `[keys]`, and only apply to a card with a
  stack.
- The detail (`d`) follows the same cursor: `Stack 2 of 3`, where each key
  leads, the cards of the stack, and the strip's own details under them.
  `j` / `k` move through it without closing it.
- With the mouse: click a strip to select it, click it again for its detail;
  the card's `↑` and `↓` buttons move up and down.
- `H` hands the card's ticket off together with the tests that failed, in one
  run, when `[jira.handoff] rework_command` is set. It has the placeholders of
  `command` plus `{tests}` (the keys, comma-separated) and `{evidence}` (what
  was written about them, a line each). Like `command`, only `config.toml`
  can set it.

### Talking to every orchestrator, and keeping the target branches current

`B` (or the Broadcast button) says one message to the orchestrators of the
tickets on the board -- "rebase onto master", say. The box lists the open
orchestrators, all ticked, with what each is doing. Quick choices above the
list tick exactly some of them -- All, None, the ones with a pull request
open, idle or working ones, or those of one column -- and each says how many
it would tick. With the keys, `tab` goes to the list, where `h`/`l` (or the
arrows) go through the quick choices and each badge's own key picks it. Untick
any with a click or `space`, write the message (`↑`/`↓` there go through the
ready-made ones) or pick a ready-made one, and `enter` asks
(Y)es/(N)o, naming who gets it, before each orchestrator gets it as a message
of its own. A busy one reads it when it is done. It goes through Herdr's own
submission (`agent.prompt`), never typed keys, so an orchestrator waiting at a
prompt is listed but cannot be ticked: the message's enter would answer its
dialog. Answer it there first. `{target}` and `{key}` in the
message become each ticket's target branch and key. A search or a column
filter narrows who is listed. The ready-made messages are `[broadcast]
presets`, which a repository's `.ekanban.toml` may set for its team.

A ticket with no orchestrator open is listed too, unticked: `space` or a click
opens the same chooser of the agents in its workspace, and the one you choose
gets this broadcast.

The board fetches the target branches to know what is merged; that does not
move your local `master`. `U` (or Update branches) does: it fetches them and
fast-forwards each local one that is behind. Only ever a fast-forward: a
branch with commits of its own, or checked out where there are changes, is
left alone, and the footer says so (`master ↓4 · local changes`). With
`[pipeline] fast_forward = true` in config.toml it happens after every fetch.
It moves your checkouts, so a repository file cannot turn it on.

### The same from a terminal, for scripts and agents

An agent working for you -- or a script -- can read the board and act on it
without opening it. Each command takes the repository as its last argument,
the current folder by default.

```powershell
ekanban.exe state                  # one line per ticket worktree
ekanban.exe state --json           # the same as data
ekanban.exe broadcast --message "rebase onto origin/{target}" --filter column=Working
ekanban.exe broadcast --message "push your branch" --filter key=ABC-1,ABC-2 --yes
ekanban.exe update-branches        # what U does
```

`state` gives, for each ticket worktree: its branch against its target
(commits ahead and behind `origin/<target>`, uncommitted paths, commits on no
remote), its pull request and column as the board computes them, the
orchestrator and what it is doing, the other live agents, and the open
questions split into those for you and those for others. Herdr is optional:
outside a session the rows have no agents and the report says why.

`broadcast` follows the box's rules: each orchestrator gets the message as
its own, `{target}` and `{key}` filled in, and one waiting at a prompt never
gets it -- it is named instead, as is a ticket with no orchestrator open. The
filters are `all` (the default), `pr`, `idle`, `working`, `done`,
`column=<id or name>` and `key=<KEY,...>`; several narrow together. It lists
who gets what and asks before sending; `--yes` skips the question, for a
caller that has already asked you.

### Open questions

A ticket whose run has open questions says how many in its card's top border
(`?2`, or the question glyph with icons on), so you need not open each ticket to
find out. A question for you turns the card red, like an agent asking, and `!`
finds it. On such a ticket the footer offers `Q Questions`, and `Q` (or
Questions in the card's menu) lists them in a chooser, those
for you first, with the whole of the highlighted one beside the list, its brief
and the chats it carries. There `enter` opens its chat once you say yes, or
replies to a question for you; `r` replies, `D` discusses it and `x` closes
it without an answer, as on the ticket board. With no orchestrator open, Reply asks which agent gets it, as `m`
does.

### Bitbucket

A green build is not a pull request that may merge. Bitbucket also holds the
reports other tools post on the commit (SonarQube's quality gate is one), the
reviewers' verdicts and its own merge checks, and any of them can stop it. With
`[bitbucket]` set, a card's pull request gets one more line saying which it is:

- `ready to merge`: no report failed, nobody asked for changes, and Bitbucket
  would merge it.
- `waiting: ...`: a report is not finished, or the build server says a build is
  still running (Bitbucket refuses while required builds are unfinished, which
  is a wait and not a block).
- `blocked: SonarQube failed`, `blocked: has merge conflicts`,
  `blocked: changes requested by <name>`, or Bitbucket's own reason for
  refusing. A second reason shows as `+1`.

The detail (`d`) lists every reason and each report in full: SonarQube's own
words ("2 New Critical Issues (required ≤ 0)"), its figures such as coverage and
new issues, and the reviewers. The report's line opens its page, SonarQube's
dashboard for that pull request, on click. `y` copies the verdict and the
reports with the rest of the card's context. Merged pull requests get no verdict
and are not asked about; each open one is read at most once a minute (`r` reads
now), and a server that cannot be reached is said on the card instead of
showing nothing, which would read as "no problem".

```toml
[bitbucket]
url = "https://bitbucket.example.com"
token_env = "BITBUCKET_TOKEN"   # the default
```

The token is a personal access token that can read the repository, read from the
environment variable and never from a file. The project and repository come from
`pipeline.pr_url`. The token is sent only to `url`, so `url` and `token_env` are
read from `config.toml` only: a repository's `.ekanban.toml` cannot change them.
Read only; it never writes to Bitbucket.

### Pull requests waiting on your review

`[reviews]` puts the open pull requests where you are a reviewer, and have not
approved, on the board. A card whose ticket has one (by the ticket key in the
pull request's title, else its branch, using `ticket_key`) says
`review #212 by Ana` and asks for you. A pull request whose ticket has no card
gets one of its own in `column`, which is Reviewing unless set; `column = ""`
only marks the cards already there. **Review** on such a card opens the pull
request; it has no worktree, so it cannot be handed off or moved by hand. The
list is read at most once a minute (`r` reads now), and a failed read keeps the
last one.

Where the list comes from is a kind; the board itself knows no code host:

```toml
[reviews]
kind = "bitbucket"   # [bitbucket]'s server and token, pipeline.pr_url's repository
```

`kind = "command"` runs any program, without a shell, in the repository, and
reads a JSON array of objects with `id`, `title`, `branch`, `target`, `author`,
`status`, `url` and `updated` (RFC 3339); only `id` is required. GitHub through
the `gh` CLI:

```toml
[reviews]
kind = "command"
command = ["gh", "pr", "list", "--search", "review-requested:@me",
  "--json", "number,title,headRefName,baseRefName,author,url,updatedAt",
  "--jq", "[.[] | {id: .number, title, branch: .headRefName, target: .baseRefName, author: .author.login, url, updated: .updatedAt}]"]
```

GitLab is the same with `glab mr list --reviewer=@me -F json` mapped through
`jq`. Another host can also be a kind in Go: implement `reviews.Source` and add
its builder to the map `cmd/ekanban` hands to `reviews.New`. `kind` and
`command` are read from `config.toml` only, since they choose where a token
goes and what runs; a repository's `.ekanban.toml` may set `column`.

## Install

ekanban runs on Windows, and installing it compiles it, so Go must be on
`PATH`:

```powershell
herdr plugin install ezemacchi/ekanban -y
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
lose by accident. `herdr config check` validates the file.

`ekanban.show` is the other action worth a key. It shows the board for
wherever you are: the workspace's `board` tab when it has one, else a new
`board` tab (first in the workspace) when you are in an agent's pane, else the
board over the current pane until you quit or jump. The board in a tab stays
open when a card sends you elsewhere.

```toml
[[keys.command]]
key = "prefix+shift+k"
type = "plugin_action"
command = "ekanban.show"
description = "open the kanban"
```

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
`[run]` layout, the prompt a new orchestrator starts with, the `[questions]`
messages Reply types and the prompt a discussion starts from. A value there
replaces the personal one, and a list replaces the whole list.
`[orchestrator] kind` and `args`, `[questions] small` and `large`, and `[jira]`
`url`, `email_env`, `token_env` and `handoff.command`, are read from
`config.toml` only, since they choose a program to start or where a credential
comes from.

`[orchestrator]` used to be called `[lead]`, and the action `o` runs used to be
`lead` in `[keys]`. The old names still work, and the board asks you to rename
them.

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
| `y` | copy the card's context (ticket, spec, branch, pull request, QA's comment) to the clipboard |
| `gf` | send the failing check, and the end of its log, to that space's agent |
| `h` / `l` | kanban: move between columns · list: collapse / expand a group |
| `v` | grab the row, then move it — leaving its group changes its status |
| `enter` | jump to the space (reopens archived ones at their old path) |
| `1`–`9` | send to that status; the numbers are listed along the bottom |
| `s` | status picker |
| `n` | edit the note — who or what you're waiting on |
| `R` | rename the space — renames the Herdr workspace too |
| `m` | type a message into the ticket's orchestrator (or the space's one agent), then go there to send it |
| `space` | fold or unfold a group, or the column on the kanban |
| `.` | the selected card's menu: everything you can do with it (its `⋯` button) |
| `!` | go to the next card that needs you |
| `F` | show only the status under the cursor — `F` or `esc` for all |
| `O` | reorder Herdr's own Spaces sidebar to match this board |
| `a` | show or hide archived spaces |
| `/` | search by name, path, or note (or click the search field) |
| `S` | manage statuses: add, rename, reorder, delete, set the default |
| `x` | forget the selected space: its note, order and status — it asks (Y)es/(N)o first |
| `r` | refresh |
| `q` | quit — `esc` only backs out (a box, a search, a filter) and never closes the board |

In a repository the board adds its own: `a` accepts a ticket in the last column
and moves it to the archive, `A` opens that archive, `o` goes to the ticket's
orchestrator (so sorting the table moves to `ctrl+o`), `t` opens its Jira
ticket, `p` its prototype, `P` its pull request, `Q` reads its open questions
(where `r` replies, `D` discusses one and `x` closes one without an answer),
and `H` hands off a Jira ticket that has no worktree.
There `s`, `1`–`9` and `v` move a card to another column by hand, and `b` puts
a card moved by hand back where the board says. `B` broadcasts
a message to the orchestrators and `U` brings the target branches up to date (see below). `y` copies a
card's context on both boards. `ekanban keys` lists every
screen's keys, and `[keys]` in the settings rebinds any of them:

```toml
[keys]
note = "N"          # one key
archive = ["A", "z"] # or several
[keys.board]
orchestrator = []   # an empty list turns the action off on that board
```

Everything that names a key follows the binding: the buttons along the bottom,
the detail modal's, and messages such as "press n to add one", where the key
is drawn in the same colour as the buttons. An action with no key drops out of
them rather than naming a key that does something else.

Some actions belong to what is selected: on the ticket board, a role or a
question. Their keys win there over the board's and do nothing elsewhere, so a
key can mean one thing on a question and another on the rest of the board.
They are bound by name like any other, and `ekanban keys` marks them
"on a question only". While one hides a board action's key, that action's
button leaves the bottom row; a binding that leaves a board action no key at
all in some situation is reported.

## Mouse

The board takes the mouse, so Herdr never sees a click inside it — everything a
pointer can do here, the board does itself.

| | |
|---|---|
| wheel | scroll the list, leaving the cursor where it is |
| click a button | what its key does; every button shows its key |
| click the search field | search; its `✕` clears the search |
| click a column's name | show that column alone; the chip it leaves in the top bar, or its `✕`, brings the rest back |
| click a column's `▾` | fold it to its header; `▸` opens it again |
| click a group header | collapse or expand it |
| click a table heading | sort by it (name, status, or when it last changed) |
| click `⋯` on the selected card | its menu |
| click a strip under a card | stacked cards: select it · again: its detail |
| click "● N need you" | go to the next card that needs you |
| click a PR or a check | open it in a browser |
| click the title | open the view switcher (popup only) |
| click a space | **docked:** go there · **popup:** select it |
| click it again | popup: go there |

There is no right click: Herdr's pane owns the terminal, so every menu opens
from a button you can see.

Clicking a space does different things on the two boards on purpose. The dock
is a strip you glance at on your way somewhere, so one click goes there —
asking for a second would be a second click on every jump you make all day.

The popup is where rows are worked on rather than travelled to: click a row,
then press `2`, or `n`, or `v`. A click that jumped would close the whole board
under you, so there the second click is what activates.

The popup and the dock close once you jump. You opened them to get somewhere,
and once you are there they are a strip of screen showing you a board you have
stopped reading. A key brings them straight back. A board in a tab of its own
(the `tab` entrypoint, or what `ekanban.show` opens beside an agent) is out of
the way already, so it stays open.

Scrolling leaves the cursor alone either way: the cursor is what the keys act
on, and having a look further down the board should not change what `1` or
`enter` would do.

Taking the mouse also takes drag-to-select; most terminals still allow it with
shift held.

## Filtering and ordering

`F` narrows the board to whichever status the cursor is on — no picker, since
you are already standing on the group you want. `F` again, or `esc`, restores
everything. On the kanban, a click on a column's name does the same. It
applies to all three views, and the top bar says so with a chip whose `✕`
lifts it, because an empty board that does not explain itself just looks
broken.

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
ekanban tab             # the board in a tab of its own, which stays open on a jump
ekanban sidebar         # the narrow board, for a split down the side
ekanban open            # the show action: the board for wherever you are
ekanban ticket [dir]    # the ticket board of the run in a worktree
ekanban pipeline [repo] # where each worktree of a repository lands, as JSON
ekanban state [--json] [repo]       # every ticket: branch, PR, column, agents, questions
ekanban broadcast --message "..."   # one message to the chosen orchestrators
ekanban update-branches [repo]      # fast-forward the local target branches
ekanban config          # show the settings in force, and the repository file
ekanban config --init   # write a commented settings template
ekanban keys            # every action and the keys bound to it
ekanban version         # which build this is
ekanban prune           # forget entries whose directory no longer exists
```

## Releasing

There are no published binaries: `herdr plugin install` compiles from `main`.
Raise `version` in `herdr-plugin.toml` when a change is worth telling apart, since
that is what `herdr plugin list` reports.

CI runs on Windows: gofmt, vet, the tests and the manifest's build command. It
also compiles for Linux and macOS, so the code keeps building there even though
the manifest only declares Windows.

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
