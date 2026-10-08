package ui

import (
	"strings"

	"github.com/ezemacchi/ekanban/internal/keys"
)

// The global board's actions, one list per screen. The first key of each is
// what update.go switches on; config.toml's [keys] binds others by name.
var (
	commonActions = []keys.Action{
		{Name: "quit", Keys: []string{"q", "esc"}, Help: "quit", Short: "quit"},
		{Name: "layout", Keys: []string{"K"}, Help: "cycle the view: list → table → kanban"},
		{Name: "sort", Keys: []string{"o"}, Help: "table only: sort by status, name, or when it last changed"},
		{Name: "detail", Keys: []string{"d"}, Help: "list: show or hide the detail pane · elsewhere: detail modal"},
		{Name: "down", Keys: []string{"j", "down"}, Help: "move", Short: "move"},
		{Name: "up", Keys: []string{"k", "up"}, Help: "move", Short: "move"},
		{Name: "top", Keys: []string{"gg", "home"}, Help: "first row", Short: "first", Fixed: true},
		{Name: "bottom", Keys: []string{"G", "end"}, Help: "last row", Short: "last"},
		{Name: "page-down", Keys: []string{"ctrl+d", "pgdown"}, Help: "half a page down"},
		{Name: "page-up", Keys: []string{"ctrl+u", "pgup"}, Help: "half a page up"},
		{Name: "open-pr", Keys: []string{"gp"}, Help: "open the pull request in a browser", Short: "open the PR", Fixed: true},
		{Name: "send-failure", Keys: []string{"gf"}, Help: "send the failing check, with the end of its log, to that space's agent", Short: "send the failure", Fixed: true},
		{Name: "left", Keys: []string{"h", "left"}, Help: "kanban: move between columns · list: collapse / expand", Short: "fold · unfold"},
		{Name: "right", Keys: []string{"l", "right"}, Help: "kanban: move between columns · list: collapse / expand", Short: "fold · unfold"},
		{Name: "grab", Keys: []string{"v"}, Help: "grab a row, then move it — leaving its group changes its status", Short: "grab and move"},
		{Name: "jump", Keys: []string{"enter"}, Help: "jump to space (reopens archived ones)", Short: "jump to space"},
		{Name: "set-status", Keys: []string{"1-9"}, Help: "send to that status, numbered along the bottom", Short: "set status", Fixed: true},
		{Name: "status-picker", Keys: []string{"s"}, Help: "status picker", Short: "status picker"},
		{Name: "note", Keys: []string{"n"}, Help: "edit note — who or what you are waiting on", Short: "edit note"},
		{Name: "rename", Keys: []string{"R"}, Help: "rename the space — renames the Herdr workspace too", Short: "rename space"},
		{Name: "message", Keys: []string{"m"}, Help: "type a message into that space's agent, then go there to send it", Short: "message agent"},
		{Name: "fold", Keys: []string{" ", "tab"}, Help: "collapse / expand group", Short: "fold group"},
		{Name: "status-only", Keys: []string{"F"}, Help: "show only the status under the cursor — F or esc for all", Short: "this status only"},
		{Name: "reorder-spaces", Keys: []string{"O"}, Help: "reorder Herdr's own Spaces sidebar to match this board", Short: "reorder Spaces"},
		{Name: "filter", Keys: []string{"/"}, Help: "filter by name, path or note", Short: "filter"},
		{Name: "statuses", Keys: []string{"S"}, Help: "manage statuses (add, rename, reorder, delete)", Short: "statuses"},
		{Name: "forget", Keys: []string{"x"}, Help: "forget the selected space", Short: "forget space"},
		{Name: "refresh", Keys: []string{"r"}, Help: "refresh", Short: "refresh"},
		{Name: "help", Keys: []string{"?"}, Help: "this help", Short: "help"},
	}

	// BoardActions are the manual board's: the common ones plus archived.
	BoardActions = append(append([]keys.Action(nil), commonActions...),
		keys.Action{Name: "archived", Keys: []string{"a"}, Help: "show or hide archived spaces", Short: "archived"})

	// PipelineActions are the computed board's: a accepts instead of showing
	// archived spaces, A opens the archive of accepted tickets, and o goes to
	// a ticket's orchestrator, so sorting the table moves to ctrl+o.
	PipelineActions = append(withKeys(commonActions, "sort", "ctrl+o"),
		keys.Action{Name: "accept", Keys: []string{"a"}, Help: "accept a ticket in the last column: it moves to the Archive", Short: "accept"},
		keys.Action{Name: "archive", Keys: []string{"A"}, Help: "Archive of accepted tickets", Short: "archive"},
		keys.Action{Name: "lead", Keys: []string{"o"}, Canon: "lead", Help: "go to the ticket's orchestrator (the team's lead), or open one", Short: "orchestrator"})

	// ArchiveActions are the archive list's.
	ArchiveActions = []keys.Action{
		{Name: "archive", Keys: []string{"A", "q", "esc"}, Help: "back to the board", Short: "back"},
		{Name: "down", Keys: []string{"j", "down"}, Help: "move", Short: "move"},
		{Name: "up", Keys: []string{"k", "up"}, Help: "move", Short: "move"},
		{Name: "bottom", Keys: []string{"G"}, Help: "last", Short: "last"},
		{Name: "filter", Keys: []string{"/"}, Help: "search", Short: "search"},
		{Name: "open-issue", Keys: []string{"o", "enter"}, Help: "open the ticket in the tracker", Short: "tracker"},
		{Name: "open-pull-request", Keys: []string{"p"}, Help: "open the pull request", Short: "pull request"},
		{Name: "restore", Keys: []string{"u"}, Help: "back to the board", Short: "restore"},
	}
)

// withKeys is a copy of actions with name's keys replaced, keeping the code
// path it had.
func withKeys(actions []keys.Action, name string, ks ...string) []keys.Action {
	out := append([]keys.Action(nil), actions...)
	for i, a := range out {
		if a.Name == name {
			if a.Canon == "" && len(a.Keys) > 0 {
				out[i].Canon = a.Keys[0]
			}
			out[i].Keys = ks
		}
	}
	return out
}

// keyMaps are the screens' maps with the user's bindings.
type keyMaps struct {
	board, pipeline, archive *keys.Map
}

// SetKeys applies config.toml's [keys]; conflicts go to the status line.
func (m *Model) SetKeys(bindings map[string][]string) {
	var problems []string
	add := func(actions []keys.Action) *keys.Map {
		km, p := keys.New(actions, bindings)
		problems = append(problems, p...)
		return km
	}
	m.keys = keyMaps{board: add(BoardActions), pipeline: add(PipelineActions), archive: add(ArchiveActions)}
	for name, ks := range bindings {
		for _, k := range ks {
			if k == "g" {
				problems = append(problems, name+": g starts the gg / gp / gf chords and cannot be bound")
			}
		}
	}
	if len(problems) > 0 {
		m.status = strings.Join(uniq(problems), " · ")
	}
}

// keyMap is the map of the screen in front.
func (m *Model) keyMap() *keys.Map {
	switch {
	case m.pipelineOn() && m.archiveView:
		return m.keys.archive
	case m.pipelineOn():
		return m.keys.pipeline
	}
	return m.keys.board
}

// hintKey is the current key of an action on the screen in front.
func (m *Model) hintKey(name string) string {
	km := m.keyMap()
	if km == nil {
		km, _ = keys.New(m.defaultActions(), nil)
	}
	return km.Key(name)
}

func (m *Model) defaultActions() []keys.Action {
	switch {
	case m.pipelineOn() && m.archiveView:
		return ArchiveActions
	case m.pipelineOn():
		return PipelineActions
	}
	return BoardActions
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
