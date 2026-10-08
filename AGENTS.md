# Agent guide

This file is for a coding agent asked to install ekanban for someone, set it up
for their project, or change its code. The user-facing description is in
`README.md`.

ekanban is a [Herdr](https://herdr.dev) plugin. This fork runs on **Windows**:
every command in `herdr-plugin.toml` goes through `cmd /c`, and the manifest
declares `platforms = ["windows"]`.

## Install for a user

### 1. Check what is there

| Need | Check | Why |
|---|---|---|
| Herdr 0.7.5 or later | `herdr --version` | the plugin API it uses |
| Go (the version in `go.mod`) | `go version` | the install step compiles from source |
| GitHub CLI, signed in (optional) | `gh auth status` | pull request columns on the plain board |
| A Nerd Font in the terminal (optional) | ask the user | icons; leave `icons` off without one |

Stop and tell the user if Herdr or Go is missing. Do not install them without
asking.

### 2. Install

```powershell
herdr plugin install ezemacchi/ekanban -y
herdr plugin list
```

`herdr plugin list` must show `ekanban ... enabled` and its config folder. For
a working copy instead (someone developing the plugin):

```powershell
git clone -b windows https://github.com/ezemacchi/ekanban
cd ekanban
go build -trimpath -o bin\ekanban.exe .\cmd\ekanban
herdr plugin link .
```

The binary is `bin\ekanban.exe` under the plugin's folder. It is not on `PATH`;
call it by its full path. `herdr plugin list` prints the folder for a linked
plugin; an installed one is under Herdr's plugin folder.

### 3. A key to open the board

Add this to Herdr's config (`%APPDATA%\herdr\config.toml`), then run
`herdr server reload-config`:

```toml
[[keys.command]]
key = "prefix+d"
type = "plugin_action"
command = "ekanban.open"
description = "Space board"
```

`prefix+d` is free in Herdr's default keymap. A `keys.command` entry silently
replaces a built-in key, so check with the user before choosing another, and
run `herdr config check` after editing.

Offer the `show` action as well. It opens the board for wherever the user is:
the workspace's `board` tab if there is one, else a new `board` tab beside an
agent's pane, else the board over the current pane:

```toml
[[keys.command]]
key = "prefix+shift+k"
type = "plugin_action"
command = "ekanban.show"
description = "open the kanban"
```

### 4. Settings

There are two settings files, both optional:

| File | Holds | Shared? |
|---|---|---|
| `config.toml` in the plugin's config folder (`herdr plugin config-dir ekanban`) | personal choices: icons, the agent a board starts (`[orchestrator] kind`, `args`), keys, the specifications clone | never |
| `.ekanban.toml` at the root of a repository | that project's process: tracker and pull request links, build server, target branches, `[run]` layout, the `[orchestrator]` prompt | the team decides |

`ekanban.exe config --init` writes a commented `config.toml` with every setting.
`ekanban.exe config`, run inside a repository, prints which files are in force.

The repository file is read over `config.toml` when a board opens in that
repository or any of its worktrees. A value replaces the personal one; a list
replaces the whole list. `[orchestrator] kind` and `args` are ignored there,
because they choose a program to start. `[lead]` is the old name of
`[orchestrator]`; it still works, but write `[orchestrator]`.

Ask the user whether `.ekanban.toml` should be committed. When it holds internal
addresses (a private tracker, build server, or code host), keep it out of git
for that clone only:

```powershell
Add-Content .git\info\exclude ".ekanban.toml"
```

A file kept out of git exists only in the main checkout. Boards opened in
linked worktrees find it there.

Settings never hold credentials. The plugin reads pull requests through the
user's `gh` login and talks to the build server without signing in. There are
two credentials, both optional. The second is a Bitbucket personal access token
for `[bitbucket]`, kept in the variable `token_env` names (`BITBUCKET_TOKEN` by
default); `[bitbucket] url` and `token_env` are ignored in `.ekanban.toml`, and
the same rule applies: never ask for it, write it, or print it. The first is for
Jira (`[jira]`): an Atlassian API token and the
e-mail of its account, kept in two environment variables that `email_env` and
`token_env` name (`JIRA_EMAIL` and `JIRA_API_TOKEN` by default). The user
creates and sets them; never ask for the value in chat, write it to a file, or
print it. `[jira] url`, `email_env`, `token_env` and `handoff.command` are
ignored in `.ekanban.toml`. If any other setting seems to need a token, stop
and ask.

### 5. Teams

A team is the set of roles a run goes through. The ticket board shows one card
per role. The binary ships one example team (planner, implementer, reviewer,
QA). Put the user's own teams in a `teams` folder inside the config folder, one
TOML file per team; `internal/team/teams/example.toml` explains every field,
including the `[orchestrator]` table (the agent that runs the team). Mark the
usual one with `default = true`.

### 6. Check it works

```powershell
herdr plugin pane open --plugin ekanban --entrypoint tab --cwd <repository>
herdr plugin pane open --plugin ekanban --entrypoint ticket --placement tab --cwd <worktree>
```

The first is the global board for a repository: one card per ticket worktree,
in delivery columns. It is the `tab` entrypoint, which stays open when a card
sends you elsewhere; `board` (a popup) and `side` (a dock) close then. The
second is one run's roles; it needs a run folder in the worktree (by default `.runs/<KEY>/STATE.md`; `[run]` changes that). Read
the pane back with `herdr pane read <pane_id> --source visible` and confirm the
cards are there.

### Do not

- Run `herdr server stop` while the user has agents working. It ends their
  session.
- Close Herdr workspaces, tabs or panes you did not open.
- Edit the board's state file (`board.json`, under `%LOCALAPPDATA%\herdr\plugins\ekanban`)
  while a board is open. The board rewrites it.

## Change the code

```powershell
go build ./...
go vet ./...
go test ./...
gofmt -l cmd internal    # must print nothing
```

GitHub CI runs the same on Linux, plus `go test -race`. Tests must pass on both
systems: build paths with `filepath`, and compare against `store.Key(...)`
rather than a literal path.

Rules for this repository:

- **No internal information in code, tests or history.** No private hostnames,
  ticket keys from a real tracker, people's names or internal repository names.
  Examples use `example.com`, `ABC-1`, `PROJ-123`. Project values belong in a
  repository's `.ekanban.toml` or the user's `config.toml`.
- A project's process is configuration, not code. Where runs live, how their
  state file reads, and which teams exist come from TOML (`[run]`, `teams/`).
  Do not hard-code another layout.
- Settings fall back to their defaults when wrong, and say so. A bad value
  never stops a board.
- The UI strings are English.
- Every key is an action in `internal/ui/actions.go` or
  `internal/ticketui/model.go`, so `[keys]` can rebind it and the help screen
  lists it.
- Never write a rebindable key into a string. Ask the key map for it
  (`hintKey`, `keyMap().Key`), mark it with `screen.Key` and draw the message
  with `screen.Say`, so it follows `[keys]` and is drawn in the key colour.
  Leave the hint out when the action has no key.
- Both boards draw through `internal/screen`. Change how a board looks there,
  not in one board, or the two drift apart again.

Package map:

| Package | Job |
|---|---|
| `cmd/ekanban` | entry points: board, tab, sidebar, open (the show action), ticket, watch, startup, config |
| `internal/config` | `config.toml` and `.ekanban.toml` |
| `internal/ui` | the plain board and the global board in a repository |
| `internal/ticketui` | the ticket board |
| `internal/screen` | what both boards draw alike: columns, cards, footer buttons, click zones, keys in messages |
| `internal/keys` | actions, their default keys, and `[keys]` rebinding |
| `internal/ticket` | reading a run folder: roles, agents, questions, `[run]` layout |
| `internal/team` | team definitions |
| `internal/pipeline` | delivery columns from git and the build server |
| `internal/orchestrator` | going to, or starting, the agent that runs a ticket |
| `internal/jira` | read-only Jira client: ticket list, linked tests, comments, `[jira]` column mapping and handoff command |
| `internal/bitbucket` | read-only Bitbucket Server client: a pull request's merge checks, reviewers and Code Insights reports (SonarQube), and `Judge`, which says fit / waiting / blocked |
| `internal/herdr` | the Herdr socket API |
| `internal/gh` | pull requests through the `gh` CLI |
| `internal/store` | the board's own state file |
| `internal/watch` | the background poller and notifications |
