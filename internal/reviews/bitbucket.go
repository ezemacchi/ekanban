package reviews

import (
	"context"
	"errors"
	"strconv"

	"github.com/ezemacchi/ekanban/internal/bitbucket"
)

// BitbucketReader is what the bitbucket kind needs from a Bitbucket client:
// one call. *bitbucket.Client is one; a test passes its own.
type BitbucketReader interface {
	AwaitingReview(ctx context.Context, project, repo string) ([]bitbucket.Awaiting, error)
}

type bitbucketSource struct {
	reader BitbucketReader
	loc    bitbucket.Location
	link   func(number int) string
}

// Bitbucket is the builder of the bitbucket kind: the pull requests into
// loc's repository on the server reader talks to. link builds a pull
// request's page when the server gave none. reader is nil when Bitbucket is
// off, and why then says what is missing.
func Bitbucket(reader BitbucketReader, loc bitbucket.Location, link func(int) string, why string) Builder {
	return func(Config) (Source, error) {
		if reader == nil {
			if why == "" {
				why = "set [bitbucket] url in config.toml"
			}
			return nil, errors.New(why)
		}
		if link == nil {
			link = func(int) string { return "" }
		}
		return &bitbucketSource{reader: reader, loc: loc, link: link}, nil
	}
}

func (b *bitbucketSource) Name() string { return "Bitbucket" }

func (b *bitbucketSource) Awaiting(ctx context.Context) ([]Request, error) {
	list, err := b.reader.AwaitingReview(ctx, b.loc.Project, b.loc.Repo)
	if err != nil {
		return nil, err
	}
	out := make([]Request, 0, len(list))
	for _, a := range list {
		r := Request{ID: strconv.Itoa(a.Number), Title: a.Title, Branch: a.Branch, Target: a.Target,
			Author: a.Author, Status: statusWord(a.MyStatus), URL: a.Link, Updated: a.Updated}
		if r.URL == "" {
			r.URL = b.link(a.Number)
		}
		out = append(out, r)
	}
	return out, nil
}

func statusWord(status string) string {
	if status == "NEEDS_WORK" {
		return "you asked for changes"
	}
	return "not reviewed yet"
}
