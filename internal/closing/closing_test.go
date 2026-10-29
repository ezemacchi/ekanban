package closing

import (
	"errors"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/state"
)

// fake is an Env that records what was done, with git answers by command.
type fake struct {
	git       map[string]string // command -> output; a missing one fails
	check     error
	output    string
	did       []string
	removeErr error
}

func (f *fake) env() Env {
	return Env{
		Git: func(dir string, args ...string) (string, error) {
			out, ok := f.git[strings.Join(args, " ")]
			if !ok {
				return "", errors.New("exit status 1")
			}
			return out, nil
		},
		RunCheck: func(dir string, args []string) (string, error) {
			f.did = append(f.did, "check "+strings.Join(args, " "))
			return f.output, f.check
		},
		Archive: func(t state.Ticket) (string, error) {
			f.did = append(f.did, "archive "+t.Key)
			return "/state/archive/" + t.Key, nil
		},
		Accept: func(t state.Ticket, archive string) error {
			f.did = append(f.did, "accept "+t.Key+" "+archive)
			return nil
		},
		CloseWorkspace: func(id string) error {
			f.did = append(f.did, "close "+id)
			return nil
		},
		RemoveWorktree: func(repo, worktree string) error {
			f.did = append(f.did, "remove "+worktree)
			return f.removeErr
		},
	}
}

func clean() map[string]string {
	return map[string]string{
		"status --porcelain":                        "",
		"rev-list --count HEAD --not --remotes":     "0",
		"merge-base --is-ancestor HEAD origin/main": "",
	}
}

func plan() Plan {
	return Plan{
		Ticket:     state.Ticket{Key: "ABC-1", Worktree: "/w/abc-1", Branch: "feature/abc-1", Git: state.Branch{Target: "main"}, PR: &state.PR{Number: 42}},
		Repo:       "/r",
		Workspaces: []string{"w7"},
		Self:       "w1",
		Check:      []string{"is-merged", "{key}", "{pr}", "{target}"},
	}
}

// Everything checked first, then accept and archive, the workspace, the
// worktree: in that order.
func TestCloseInOrder(t *testing.T) {
	f := &fake{git: clean(), output: "ABC-1 is in main\n"}
	var said []string
	if err := Close(f.env(), plan(), func(s Step) { said = append(said, s.Name) }); err != nil {
		t.Fatal(err)
	}
	want := "check is-merged ABC-1 42 main|archive ABC-1|accept ABC-1 /state/archive/ABC-1|close w7|remove /w/abc-1"
	if got := strings.Join(f.did, "|"); got != want {
		t.Fatalf("did:\n%s\nwant:\n%s", got, want)
	}
	if got := strings.Join(said, ","); got != "uncommitted changes,pushed,merged,close check,accepted,workspace closed,worktree removed" {
		t.Fatalf("said %s", got)
	}
}

// Each failed check refuses with its reason and changes nothing.
func TestCloseRefuses(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(f *fake, p *Plan)
		check string
		why   string
	}{
		{"from inside", func(f *fake, p *Plan) { p.Self = "w7" }, "where it runs", "another workspace"},
		{"uncommitted", func(f *fake, p *Plan) { f.git["status --porcelain"] = " M a.go\n?? b.go" }, "uncommitted changes", "2 path(s)"},
		{"unpushed", func(f *fake, p *Plan) { f.git["rev-list --count HEAD --not --remotes"] = "3" }, "pushed", "3 commit(s)"},
		{"not merged", func(f *fake, p *Plan) { delete(f.git, "merge-base --is-ancestor HEAD origin/main") }, "merged", "PR #42 is not merged"},
		{"no target", func(f *fake, p *Plan) { p.Ticket.Git.Target, p.Ticket.PR = "", nil }, "merged", "no target"},
		{"the check says no", func(f *fake, p *Plan) { f.check, f.output = errors.New("exit status 1"), "ABC-1 is not on main yet\n" }, "close check", "not on main yet"},
	}
	for _, c := range cases {
		f := &fake{git: clean()}
		p := plan()
		c.edit(f, &p)
		err := Close(f.env(), p, func(Step) {})
		var r *Refused
		if !errors.As(err, &r) || r.Check != c.check || !strings.Contains(r.Why, c.why) {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		for _, d := range f.did {
			if !strings.HasPrefix(d, "check ") {
				t.Errorf("%s: %q was done after a refusal", c.name, d)
			}
		}
	}
}

// A pull request seen merged is enough; the branch need not be an ancestor
// (a squash merge leaves it not one).
func TestCloseMergedPullRequest(t *testing.T) {
	f := &fake{git: clean()}
	delete(f.git, "merge-base --is-ancestor HEAD origin/main")
	p := plan()
	p.Ticket.PR.MergedTo = "main"
	p.Check = nil
	if err := Close(f.env(), p, func(Step) {}); err != nil {
		t.Fatal(err)
	}
}

// A worktree that will not go says what was already done.
func TestCloseSaysWhatWasDoneWhenTheWorktreeStays(t *testing.T) {
	f := &fake{git: clean(), removeErr: errors.New("Permission denied")}
	p := plan()
	p.Check = nil
	err := Close(f.env(), p, func(Step) {})
	if err == nil || IsRefused(err) || !strings.Contains(err.Error(), "accepted and its workspaces closed") {
		t.Fatalf("err %v", err)
	}
}

func TestFill(t *testing.T) {
	p := plan()
	p.Ticket.PR = nil
	got := Fill([]string{"{repo}", "{worktree}", "{branch}", "x{pr}x"}, p)
	if strings.Join(got, " ") != "/r /w/abc-1 feature/abc-1 xx" {
		t.Fatalf("fill: %q", got)
	}
}
