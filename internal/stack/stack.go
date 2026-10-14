// Package stack is stacked cards: related tickets drawn under a card, one
// strip each, that the keys and the mouse walk through before leaving the
// card. A test that failed on a story is the first use, but nothing here knows
// about tests, trackers or the board.
//
// What goes on a stack comes from a Source, registered by name the same way
// the pipeline's column rules are. [stack] sources in the settings picks which
// run, in order; without it there are no stacks and the board is unchanged.
package stack

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Item is one related ticket on a card's stack.
type Item struct {
	Key     string // drawn in the strip
	Word    string // what it is to the card: "failed", "blocked"
	Summary string
	Status  string // the ticket's own status, for the detail
	URL     string // opened by the open action; empty when there is none
	// Evidence is why the source put it here, for the detail: the line of a
	// comment that named it, say.
	Evidence  string
	Attention bool // drawn in the attention colour
}

// Card is what a source may know about a card, and nothing more.
type Card struct {
	Key string // the card's ticket key, "" when it has none
}

// Linked is a ticket linked to another, as the tracker lists it.
type Linked struct {
	Key, Summary, Status, Type, URL string
}

// Note is one comment on a ticket.
type Note struct {
	Author string
	At     time.Time
	Body   string
}

// Ticket is what the board holds about one ticket: what it links to and what
// was said on it. Tests are the links of the tracker's test type.
type Ticket struct {
	Key   string
	Tests []Linked
	Links []Linked
	Notes []Note // oldest first
}

// Lookup is what the board lends a source: the tickets it has already read.
// A source never reads the network itself, since it is asked on every draw.
type Lookup interface {
	Ticket(key string) (Ticket, bool)
}

// Source finds the stack of one card.
type Source interface {
	Name() string
	// Needs reports whether Items reads the card's ticket through the
	// Lookup, so the board reads it ahead of time.
	Needs(c Card) bool
	Items(c Card, l Lookup) []Item
}

// Factory builds a source from its [stack.source.<name>] table. Problems are
// bad values it fell back from; the source still works.
type Factory func(opts map[string]string) (Source, []string)

var registry = map[string]Factory{}

// Register makes a source available by name. Registering a name twice panics:
// two sources answering to one name would make the settings ambiguous.
func Register(name string, f Factory) {
	if _, dup := registry[name]; dup {
		panic("stack: source registered twice: " + name)
	}
	registry[name] = f
}

// Names lists the registered sources, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Config is the [stack] table.
type Config struct {
	// Sources name the sources that run, in order. Empty turns stacks off.
	Sources []string `toml:"sources"`
	// MaxItems is how many strips a card draws; the rest show as "+N more".
	MaxItems int `toml:"max_items"`
	// Collapse draws a card that is not selected with one "+N" line instead
	// of its strips. Unset means on.
	Collapse *bool `toml:"collapse"`
	// Column is a column id a card with a stack moves to, "todo" say, so the
	// work it stands for is picked up again. Empty leaves cards where they are.
	Column string `toml:"column"`
	// Source holds each source's own settings: [stack.source.<name>].
	Source map[string]map[string]string `toml:"source"`
}

// DefaultMaxItems is MaxItems when it is not set.
const DefaultMaxItems = 8

// Stacks are the sources in force and the position of each card's cursor.
// A nil *Stacks is off: every method answers as if no card had a stack.
type Stacks struct {
	sources  []Source
	max      int
	collapse bool
	column   string
	at       map[string]int
	length   map[string]int
}

// New builds the stacks of cfg: nil when it names no source. Unknown names
// and bad values are reported and skipped, so a typo costs one source, not
// the board.
func New(cfg Config) (*Stacks, []string) {
	var problems []string
	s := &Stacks{max: cfg.MaxItems, collapse: cfg.Collapse == nil || *cfg.Collapse, column: strings.TrimSpace(cfg.Column),
		at: map[string]int{}, length: map[string]int{}}
	if s.max <= 0 {
		if cfg.MaxItems < 0 {
			problems = append(problems, fmt.Sprintf("stack.max_items %d is not a count — using %d", cfg.MaxItems, DefaultMaxItems))
		}
		s.max = DefaultMaxItems
	}
	seen := map[string]bool{}
	for _, name := range cfg.Sources {
		f, ok := registry[name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("stack source %q does not exist (known: %v)", name, Names()))
			continue
		case seen[name]:
			continue
		}
		seen[name] = true
		src, p := f(cfg.Source[name])
		for _, why := range p {
			problems = append(problems, fmt.Sprintf("stack.source.%s: %s", name, why))
		}
		s.sources = append(s.sources, src)
	}
	for name := range cfg.Source {
		if !seen[name] {
			if _, ok := registry[name]; !ok {
				problems = append(problems, fmt.Sprintf("[stack.source.%s]: no such source (known: %v)", name, Names()))
			}
		}
	}
	if len(s.sources) == 0 {
		return nil, problems
	}
	return s, problems
}

// On reports whether any source runs.
func (s *Stacks) On() bool { return s != nil && len(s.sources) > 0 }

// Collapse is whether a card that is not selected shows one line for its stack.
func (s *Stacks) Collapse() bool { return s.On() && s.collapse }

// Max is how many strips a card draws.
func (s *Stacks) Max() int {
	if !s.On() {
		return 0
	}
	return s.max
}

// Column is the column a card with a stack moves to, "" for none.
func (s *Stacks) Column() string {
	if !s.On() {
		return ""
	}
	return s.column
}

// Needs reports whether any source reads c's ticket.
func (s *Stacks) Needs(c Card) bool {
	if !s.On() {
		return false
	}
	for _, src := range s.sources {
		if src.Needs(c) {
			return true
		}
	}
	return false
}

// Of is c's stack: every source's items in order, the first one to name a
// key keeping it.
func (s *Stacks) Of(c Card, l Lookup) []Item {
	if !s.On() {
		return nil
	}
	var out []Item
	seen := map[string]bool{}
	for _, src := range s.sources {
		for _, it := range src.Items(c, l) {
			k := strings.ToUpper(it.Key)
			if it.Key == "" || seen[k] || strings.EqualFold(it.Key, c.Key) {
				continue
			}
			seen[k] = true
			out = append(out, it)
		}
	}
	return out
}

// At is the cursor of the card named card on a stack of n: -1 is the card
// itself, 0 the first strip. A stack that changed length starts over on the
// card, since the strip it was on may be gone.
func (s *Stacks) At(card string, n int) int {
	if !s.On() || n == 0 {
		return -1
	}
	if s.length[card] != n {
		s.length[card] = n
		s.at[card] = -1
	}
	at, ok := s.at[card]
	if !ok || at < -1 || at >= n {
		return -1
	}
	return at
}

// Step moves the cursor of card, on a stack of n, by delta. It reports false
// when the move leaves the stack -- past the last strip, or up from the card
// -- and then leaves the cursor where it was, so the caller moves to the next
// or previous card instead.
func (s *Stacks) Step(card string, n, delta int) bool {
	if !s.On() || n == 0 {
		return false
	}
	next := s.At(card, n) + delta
	if next < -1 || next >= n {
		return false
	}
	s.at[card] = next
	return true
}

// Set puts the cursor of card, on a stack of n, on strip i (-1 the card).
func (s *Stacks) Set(card string, n, i int) {
	if !s.On() || i < -1 || i >= n {
		return
	}
	s.At(card, n)
	s.at[card] = i
}

// Neighbours are what moving up and down reaches from at, on the card named
// key with stack items: a ticket key, or "" when the move leaves the card.
func Neighbours(key string, items []Item, at int) (up, down string) {
	switch {
	case at == 0:
		up = key
	case at > 0 && at <= len(items):
		up = items[at-1].Key
	}
	if at+1 < len(items) {
		down = items[at+1].Key
	}
	return up, down
}

// Summary is a stack in a few words, for a card with no room for its strips:
// "+2 failed", or "+3 related" when the items differ. more counts the items
// past the ones given, which a card cut short still has.
func Summary(items []Item, more int) string {
	if len(items) == 0 {
		return ""
	}
	word := items[0].Word
	for _, it := range items[1:] {
		if it.Word != word {
			word = ""
			break
		}
	}
	if word == "" {
		word = "related"
	}
	return fmt.Sprintf("+%d %s", len(items)+more, word)
}
