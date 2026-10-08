// Package keys is the board's key map: every action has a name, default keys
// and a line of help, and config.toml's [keys] table can bind other keys to it.
//
// The screens keep switching on an action's first default key. Resolve sits
// in front of them and turns whatever was pressed into that key, so binding
// "y" to accept makes y behave exactly as a did, and a stops accepting.
package keys

import (
	"fmt"
	"sort"
	"strings"
)

// Action is one thing a key does on a screen.
type Action struct {
	Name  string   // what config.toml binds, e.g. "accept"
	Keys  []string // defaults; the first is what the screen switches on
	Help  string   // the help screen's description
	Short string   // a narrower description, for a docked board
	// Fixed actions are shown in help but cannot be rebound: chords such as
	// gp, or the digits that pick a status by position.
	Fixed bool
	// Canon is what the screen switches on when it is not the first key: a
	// screen that gives a shared action another key keeps its code path.
	Canon string
}

// Map is a screen's actions with the user's bindings applied.
type Map struct {
	actions []Action
	canon   []string        // each action's first default key
	byKey   map[string]int  // effective key -> action
	moved   map[string]bool // default keys whose action now lives elsewhere
}

// New applies bindings (action name -> keys) to actions. Bindings for actions
// this screen does not have are ignored here; Unknown reports the ones no
// screen has. A key bound to two actions keeps the first and is reported.
func New(actions []Action, bindings map[string][]string) (*Map, []string) {
	m := &Map{byKey: map[string]int{}, moved: map[string]bool{}}
	var problems []string
	for _, a := range actions {
		canon := a.Canon
		if canon == "" && len(a.Keys) > 0 {
			canon = a.Keys[0]
		}
		m.canon = append(m.canon, canon)
		if keys, ok := bindings[a.Name]; ok && !a.Fixed {
			// An empty list turns the action off on this screen.
			for _, k := range a.Keys {
				m.moved[k] = true
			}
			a.Keys = keys
		}
		m.actions = append(m.actions, a)
	}
	for i, a := range m.actions {
		if a.Fixed {
			continue
		}
		for _, k := range a.Keys {
			if j, taken := m.byKey[k]; taken {
				problems = append(problems, fmt.Sprintf("key %q is bound to both %s and %s; %s keeps it", k, m.actions[j].Name, a.Name, m.actions[j].Name))
				continue
			}
			m.byKey[k] = i
		}
	}
	return m, problems
}

// Resolve is the key the screen should see for pressed: the first default key
// of the action pressed is bound to, "" for a default key whose action was
// moved to another key, or pressed itself when no action claims it.
func (m *Map) Resolve(pressed string) string {
	if m == nil {
		return pressed
	}
	if i, ok := m.byKey[pressed]; ok {
		return m.canon[i]
	}
	if m.moved[pressed] {
		return ""
	}
	return pressed
}

// Keys are the keys that trigger name now.
func (m *Map) Keys(name string) []string {
	if m != nil {
		for _, a := range m.actions {
			if a.Name == name {
				return a.Keys
			}
		}
	}
	return nil
}

// Key is the first key of name, for a hint line; "" when the action has no
// key, so a hint or message for it is left out rather than naming a key that
// does something else.
func (m *Map) Key(name string) string {
	if k := m.Keys(name); len(k) > 0 {
		return k[0]
	}
	return ""
}

// Actions are the screen's actions with their current keys, in order.
func (m *Map) Actions() []Action {
	if m == nil {
		return nil
	}
	return m.actions
}

// Unknown lists bound names that no screen has, sorted.
func Unknown(bindings map[string][]string, screens ...[]Action) []string {
	known := map[string]bool{}
	for _, s := range screens {
		for _, a := range s {
			known[a.Name] = true
		}
	}
	var out []string
	for name := range bindings {
		if !known[name] {
			out = append(out, fmt.Sprintf("unknown action %q in [keys]", name))
		}
	}
	sort.Strings(out)
	return out
}

// Display is how a key reads in help: " " is space.
func Display(keys []string) string {
	out := make([]string, len(keys))
	for i, k := range keys {
		if k == " " {
			k = "space"
		}
		out[i] = k
	}
	return strings.Join(out, " / ")
}
