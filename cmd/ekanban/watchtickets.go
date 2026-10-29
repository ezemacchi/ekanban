package main

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/pipeline"
	"github.com/ezemacchi/ekanban/internal/store"
	"github.com/ezemacchi/ekanban/internal/ticket"
	"github.com/ezemacchi/ekanban/internal/watch"
)

// ticketReader is the watcher's watch.Tickets: the folders it watches,
// grouped by repository and classified with that repository's settings, the
// way `ekanban state` does. One pipeline source per repository is kept
// between polls, so a poll that does not refresh answers from the last
// reading of the build server.
type ticketReader struct {
	sources map[string]*pipeline.Source
}

func newTicketReader() *ticketReader {
	return &ticketReader{sources: map[string]*pipeline.Source{}}
}

func (r *ticketReader) read(ctx context.Context, dirs []string, refresh bool) []watch.Row {
	live := ticket.Live{}
	if client, err := herdr.New(); err == nil {
		live, _ = ticket.ReadLive(client)
	}
	known := knownPRs()
	var out []watch.Row
	for _, g := range byRepo(dirs, repoOf) {
		src, ok := r.sources[g.repo]
		if !ok {
			src = repoSource(g.repo)
			r.sources[g.repo] = src
		}
		if refresh || !ok {
			src.Refresh(ctx)
		}
		for _, t := range ticketRows(ctx, src, live, g.dirs, known) {
			out = append(out, watch.Row{Dir: store.Key(t.Worktree), Ticket: t})
		}
	}
	return out
}

// repoGroup is the watched folders of one repository.
type repoGroup struct {
	repo string
	dirs []string
}

// byRepo groups dirs by the repository repoOf names, leaving out what is
// not a checkout. Groups and folders are sorted, so a poll reads them in the
// same order every time.
func byRepo(dirs []string, repoOf func(string) string) []repoGroup {
	groups := map[string][]string{}
	for _, d := range dirs {
		if !store.Checkout(d) {
			continue
		}
		if repo := repoOf(d); repo != "" {
			groups[repo] = append(groups[repo], d)
		}
	}
	var out []repoGroup
	for repo, ds := range groups {
		sort.Strings(ds)
		out = append(out, repoGroup{repo, ds})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].repo < out[j].repo })
	return out
}

// repoOf is the main checkout of the repository dir belongs to: the folder
// holding its shared .git, "" when git cannot say.
func repoOf(dir string) string {
	common, err := runGit(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || common == "" {
		return ""
	}
	common = filepath.Clean(common)
	if !strings.EqualFold(filepath.Base(common), ".git") {
		return "" // a bare repository has no checkout of its own to read
	}
	return store.Key(filepath.Dir(common))
}
