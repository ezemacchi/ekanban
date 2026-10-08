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
herdr plugin install ezemacchi/ekanban --ref windows -y
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
replaces a built-in key, so check with the user before choosing another.

### 4. Settings

There are two settings files, both optional:

| File | Holds | Shared? |
|---|---|---|
| `config.toml` in the plugin's config folder (`herdr plugin config-dir ekanban`) | personal choices: icons, the agent a board starts (`[lead] kind`, `args`), keys, the specifications clone | never |
| `.ekanban.toml` at the root of a repository | that project's process: tracker and pull request links, build server, target branches, `[run]` layout, the `[lead]` prompt | the team decides |

`ekanban.exe config --init` writes a commented `config.toml` with every setting.
`ekanban.exe config`, run inside a repository, prints which files are in force.

The repository file is read over `config.toml` when a board opens in that
repository or any of its worktrees. A value replaces the personal one; a list
replaces the whole list. `[lead] kind` and `args` are ignored there, because
they choose a program to start.

Ask the user whether `.ekanban.toml` should be committed. When it holds internal
addresses (a private tracker, build server, or code host), keep it out of git
for that clone only:

```powershell
Add-Content .git\info\exclude ".ekanban.toml"
```

A file kept out of git exists only in the main checkout. Boards opened in
linked worktrees find it there.

Settings never hold credentials. The plugin reads pull requests through the
user's `gh` login and talks to the build server without signing in. If a
setting seems to need a token, stop and ask.

### 5. Teams

A team is the set of roles a run goes through. The ticket board shows one card
per role. The binary ships one example team (planner, implementer, reviewer,
QA). Put the user's own teams in a `teams` folder inside the config folder, one
TOML file per team; `internal/team/teams/example.toml` explains every field.
Mark the usual one with `default = true`.

### 6. Check it works

```powershell
herdr plugin pane open --plugin ekanban --entrypoint board --cwd <repository>
herdr plugin pane open --plugin ekanban --entrypoint ticket --placement tab --cwd <worktree>
```

The first is the global board for a repository: one card per ticket worktree,
in delivery columns. The second is one run's roles; it needs a run folder in
the worktree (by default `.runs/<KEY>/STATE.md`; `[run]` changes that). Read
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

Package map:

| Package | Job |
|---|---|
| `cmd/ekanban` | entry points: board, sidebar, ticket, watch, startup, config |
| `internal/config` | `config.toml` and `.ekanban.toml` |
| `internal/ui` | the plain board and the global board in a repository |
| `internal/ticketui` | the ticket board |
| `internal/ticket` | reading a run folder: roles, agents, questions, `[run]` layout |
| `internal/team` | team definitions |
| `internal/pipeline` | delivery columns from git and the build server |
| `internal/lead` | going to, or starting, the agent that runs a ticket |
| `internal/herdr` | the Herdr socket API |
| `internal/gh` | pull requests through the `gh` CLI |
| `internal/store` | the board's own state file |
| `internal/watch` | the background poller and notifications |
