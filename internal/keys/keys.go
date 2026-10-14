// Package keys is the board's key map: every action has a name, default keys
// and a line of help, and config.toml's [keys] table can bind other keys to it.
//
// The screens keep switching on an action's first default key. Resolve sits
// in front of them and turns whatever was pressed into that key, so binding
// "y" to accept makes y behave exactly as a did, and a stops accepting.
package keys

import (
	"fmt"
	"slices"
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
	// A scoped action's is its name unless set, so it never meets a
	// board-wide action's key in the screen's switch.
	Canon string
	// Scope is the situation the action belongs to, such as "question"; ""
	// is board-wide. [keys] binds it by name like any other.
	Scope string
}

// Map is a screen's actions with the user's bindings applied.
type Map struct {
	actions []Action
	canon   []string                  // what the screen switches on, per action
	byKey   map[string]map[string]int // scope -> effective key -> action
	moved   map[string]bool           // default keys whose action now lives elsewhere
}

// New applies bindings (action name -> keys) to actions. Bindings for actions
// this screen does not have are ignored here; Unknown reports the ones no
// screen has. A key bound to two actions of one scope keeps the first and is
// reported. A scoped action's key hides a board-wide action's in that scope;
// that is reported only when a binding leaves the board-wide one no key there.
func New(actions []Action, bindings map[string][]string) (*Map, []string) {
	m := &Map{byKey: map[string]map[string]int{}, moved: map[string]bool{}}
	var problems []string
	rebound := map[string]bool{}
	for _, a := range actions {
		canon := a.Canon
		switch {
		case canon != "":
		case a.Scope != "":
			canon = a.Name
		case len(a.Keys) > 0:
			canon = a.Keys[0]
		}
		m.canon = append(m.canon, canon)
		if keys, ok := bindings[a.Name]; ok && !a.Fixed {
			// An empty list turns the action off on this screen.
			for _, k := range a.Keys {
				m.moved[k] = true
			}
			a.Keys = keys
			rebound[a.Name] = true
		}
		m.actions = append(m.actions, a)
	}
	for i, a := range m.actions {
		if a.Fixed {
			continue
		}
		in := m.byKey[a.Scope]
		if in == nil {
			in = map[string]int{}
			m.byKey[a.Scope] = in
		}
		for _, k := range a.Keys {
			if j, taken := in[k]; taken {
				problems = append(problems, fmt.Sprintf("key %q is bound to both %s and %s; %s keeps it", k, m.actions[j].Name, a.Name, m.actions[j].Name))
				continue
			}
			in[k] = i
		}
	}
	return m, append(problems, m.hidden(rebound)...)
}

// hidden reports the board-wide actions a scope leaves with no key, when a
// binding did it: a default that hides one is the design, not a mistake.
func (m *Map) hidden(rebound map[string]bool) []string {
	var out []string
	for _, scope := range m.Scopes() {
		for _, a := range m.actions {
			if a.Scope != "" || a.Fixed || len(a.Keys) == 0 {
				continue
			}
			var by []string
			cause := rebound[a.Name]
			for _, k := range a.Keys {
				j, ok := m.byKey[scope][k]
				if !ok {
					by = nil
					break
				}
				by = append(by, fmt.Sprintf("%s is %s", k, m.actions[j].Name))
				cause = cause || rebound[m.actions[j].Name]
			}
			if len(by) > 0 && cause {
				out = append(out, fmt.Sprintf("%s has no key on a %s: %s there", a.Name, scope, strings.Join(by, ", ")))
			}
		}
	}
	return out
}

// find is the action pressed triggers with scopes in force, the first scope
// first and the board-wide actions last.
func (m *Map) find(pressed string, scopes []string) (int, bool) {
	for _, s := range scopes {
		if i, ok := m.byKey[s][pressed]; ok && s != "" {
			return i, true
		}
	}
	i, ok := m.byKey[""][pressed]
	return i, ok
}

// Resolve is what the screen should see for pressed with scopes in force: the
// first default key (or Canon) of the action pressed triggers, "" for a
// default key whose action was moved to another key, or pressed itself when
// no action claims it.
func (m *Map) Resolve(pressed string, scopes ...string) string {
	if m == nil {
		return pressed
	}
	if i, ok := m.find(pressed, scopes); ok {
		return m.canon[i]
	}
	if m.moved[pressed] {
		return ""
	}
	return pressed
}

// KeyIn is the first key of name that triggers it with scopes in force, for
// a hint drawn there; "" when there is none, such as a board-wide action
// whose keys a scoped one takes, so its hint is left out rather than naming
// a key that does something else.
func (m *Map) KeyIn(name string, scopes ...string) string {
	if m == nil {
		return ""
	}
	for i, a := range m.actions {
		if a.Name != name {
			continue
		}
		if a.Fixed {
			return m.Key(name)
		}
		for _, k := range a.Keys {
			if j, ok := m.find(k, scopes); ok && j == i {
				return k
			}
		}
		return ""
	}
	return ""
}

// Scopes are the scopes the screen's actions belong to, in order of first
// appearance, without the board-wide one.
func (m *Map) Scopes() []string {
	var out []string
	if m == nil {
		return out
	}
	for _, a := range m.actions {
		if a.Scope != "" && !slices.Contains(out, a.Scope) {
			out = append(out, a.Scope)
		}
	}
	return out
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
