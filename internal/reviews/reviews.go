// Package reviews is where the board learns which pull requests (merge
// requests, on some hosts) are waiting on the user's review. The board knows
// only Source and Request; which code host answers is chosen once, by
// [reviews] kind, from the builders cmd/ekanban hands to New.
//
// Two kinds are built in: bitbucket (Bitbucket Server, through
// internal/bitbucket) and command (any program that prints the list as JSON,
// such as gh or glab). Another host is a Source and one more entry in the map
// of builders.
package reviews

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Request is one pull request asking the user for a review.
type Request struct {
	// ID is the host's number for it. A string, because not every host's
	// identifier is a pull request number.
	ID     string
	Title  string
	Branch string // the branch it comes from
	Target string // the branch it goes into
	Author string
	// Status is where the user stands on it, in the host's words: "not
	// reviewed yet", "you asked for changes".
	Status  string
	URL     string // its page; only an https address is offered as a link
	Updated time.Time
}

// Source lists the requests waiting on the user. It only reads.
type Source interface {
	// Name is how messages name it: "Bitbucket", "gh".
	Name() string
	Awaiting(ctx context.Context) ([]Request, error)
}

// Config is the [reviews] table.
type Config struct {
	// Kind picks the Source; "" turns review requests off.
	Kind string `toml:"kind"`
	// Column is the board column a request with no card of its own goes
	// in. Unset is the Reviewing column; "" adds no cards and only marks the
	// cards already on the board.
	Column *string `toml:"column"`
	// Command is the program the command kind runs, without a shell.
	Command []string `toml:"command"`
}

// CardColumn is the column review cards go in, fallback when Column is
// unset, and "" when cards are turned off.
func (c Config) CardColumn(fallback string) string {
	if c.Column == nil {
		return fallback
	}
	return strings.TrimSpace(*c.Column)
}

// Builder makes the Source of one kind from the [reviews] table.
type Builder func(Config) (Source, error)

// New builds the configured kind. A nil Source with no reason means off by
// choice; with a reason, off because the configuration is wrong, and the
// reason is for the status line.
func New(cfg Config, kinds map[string]Builder) (Source, string) {
	kind := strings.TrimSpace(cfg.Kind)
	if kind == "" || kind == "none" {
		return nil, ""
	}
	build, ok := kinds[kind]
	if !ok {
		return nil, fmt.Sprintf("[reviews] kind %q is not known (known: %s) — review requests are off", kind, strings.Join(Kinds(kinds), ", "))
	}
	src, err := build(cfg)
	if err != nil {
		return nil, fmt.Sprintf("review requests are off: %v", err)
	}
	return src, ""
}

// Kinds are the names in kinds, sorted.
func Kinds(kinds map[string]Builder) []string {
	out := make([]string, 0, len(kinds))
	for k := range kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
