// Package columns describes a board's columns as data, so their names,
// colours, icons and order come from config.toml rather than from code.
//
// A column's ID is what the code decides with (a rule returns "on_review",
// the ticket board puts a role in "waiting"); everything else is how it is
// shown. Renaming a column therefore never touches saved state or rules.
package columns

import "fmt"

// Column is one column of a board.
type Column struct {
	ID    string `toml:"id"`
	Label string `toml:"label"`
	Color string `toml:"color"` // lipgloss colour: ANSI index or hex
	Icon  string `toml:"icon"`  // Nerd Font glyph
}

// Set is a board's columns in display order.
type Set []Column

// Find returns the column with id.
func (s Set) Find(id string) (Column, bool) {
	for _, c := range s {
		if c.ID == id {
			return c, true
		}
	}
	return Column{}, false
}

// Label is the column's name, or the id itself when the set has no such
// column, so a message never shows an empty name.
func (s Set) Label(id string) string {
	if c, ok := s.Find(id); ok && c.Label != "" {
		return c.Label
	}
	return id
}

// Merge applies the user's columns to the defaults.
//
// The user's list sets the order. An entry with a default's id changes only
// the fields it fills in. Defaults the user leaves out keep their place after
// the user's, because the code can still put a card there and a card must
// never vanish. An unknown id is added when allowNew is true (the global
// board, where a custom rule can return it) and reported otherwise.
func Merge(defaults Set, user []Column, allowNew bool) (Set, []string) {
	var out Set
	var problems []string
	used := map[string]bool{}
	for _, u := range user {
		if u.ID == "" {
			problems = append(problems, "a column without an id was ignored")
			continue
		}
		if used[u.ID] {
			problems = append(problems, fmt.Sprintf("column %q is listed twice; the second was ignored", u.ID))
			continue
		}
		base, known := defaults.Find(u.ID)
		if !known && !allowNew {
			problems = append(problems, fmt.Sprintf("unknown column %q was ignored", u.ID))
			continue
		}
		if !known {
			base = Column{ID: u.ID, Label: u.ID}
		}
		used[u.ID] = true
		out = append(out, overlay(base, u))
	}
	for _, d := range defaults {
		if !used[d.ID] {
			out = append(out, d)
		}
	}
	return out, problems
}

func overlay(base, u Column) Column {
	if u.Label != "" {
		base.Label = u.Label
	}
	if u.Color != "" {
		base.Color = u.Color
	}
	if u.Icon != "" {
		base.Icon = u.Icon
	}
	return base
}
