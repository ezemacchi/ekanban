package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeCI is a build server that answers from memory, or not at all.
type fakeCI struct {
	builds []PRBuild
	down   bool
}

func (f *fakeCI) Name() string { return "Buildy" }

func (f *fakeCI) PRBuilds(context.Context) ([]PRBuild, error) {
	if f.down {
		return nil, errors.New("down")
	}
	return f.builds, nil
}

func (f *fakeCI) LastPublish(_ context.Context, t Target) (Publish, error) {
	return Publish{Number: 7, SHA: "abc"}, nil
}

var registeredFake = &fakeCI{builds: []PRBuild{{Number: 12, Open: true, Result: "SUCCESS"}}}

func init() {
	RegisterCI("fake", func(CIConfig) CI { return registeredFake })
}

// A server registered by name is what [pipeline.ci] kind picks.
func TestRegisteredCIIsBuiltByKind(t *testing.T) {
	ci, problems := NewCI(CIConfig{Kind: "fake"})
	if len(problems) != 0 || ci != registeredFake {
		t.Fatalf("NewCI(fake) = %v, %v", ci, problems)
	}
	if !contains(CIKinds(), "fake") || !contains(CIKinds(), "jenkins") {
		t.Fatalf("CIKinds() = %v", CIKinds())
	}
}

func TestNoCIAndUnknownCI(t *testing.T) {
	for _, kind := range []string{"", "none"} {
		if ci, problems := NewCI(CIConfig{Kind: kind}); ci != nil || len(problems) != 0 {
			t.Fatalf("kind %q: %v, %v", kind, ci, problems)
		}
	}
	ci, problems := NewCI(CIConfig{Kind: "travis"})
	if ci != nil || len(problems) != 1 || !strings.Contains(problems[0], "travis") {
		t.Fatalf("unknown kind: %v, %v", ci, problems)
	}
}

// The board names the server it reads, and says which one is not answering.
func TestSourceNamesItsCI(t *testing.T) {
	if got := New(t.TempDir(), Settings{}, nil).CIName(); got != "CI" {
		t.Fatalf("no server: CIName() = %q", got)
	}
	f := &fakeCI{down: true}
	s := New(t.TempDir(), Settings{CI: f}, nil)
	if s.CIName() != "Buildy" {
		t.Fatalf("CIName() = %q", s.CIName())
	}
	s.Refresh(context.Background())
	if !strings.HasPrefix(s.Offline(), "Buildy ") {
		t.Fatalf("Offline() = %q", s.Offline())
	}
	f.down = false
	s.Refresh(context.Background())
	if s.Offline() != "" {
		t.Fatalf("still offline: %q", s.Offline())
	}
}

func TestHostSubjects(t *testing.T) {
	bb, _ := Host("bitbucket")
	if got := bb.MergeSubjects(5); !contains(got, "Pull request #5:") || !contains(got, "Merge pull request #5 in ") {
		t.Fatalf("bitbucket merge subjects = %q", got)
	}
	if got := bb.BranchSubject("feature/x"); got != "from feature/x to " {
		t.Fatalf("bitbucket branch subject = %q", got)
	}
	gh, _ := Host("github")
	if got := gh.MergeSubjects(5); len(got) != 1 || got[0] != "Merge pull request #5 from " {
		t.Fatalf("github merge subjects = %q", got)
	}
	if gh.BranchSubject("x") != "" {
		t.Fatal("github cannot match a branch with a fixed string")
	}
	anyHost, problems := Host("")
	if len(problems) != 0 || len(anyHost.MergeSubjects(1)) != 3 {
		t.Fatalf("default host: %v, %v", anyHost.MergeSubjects(1), problems)
	}
	if _, problems := Host("gitlab"); len(problems) != 1 {
		t.Fatalf("unknown host not reported: %v", problems)
	}
	if numberIn("Merge pull request #42 from me/x") != 42 || numberIn("no number") != 0 {
		t.Fatal("numberIn")
	}
}

func TestTicketKey(t *testing.T) {
	s := New(t.TempDir(), Settings{}, nil)
	if got := s.keyIn("feature/abc-12-thing"); got != "ABC-12" {
		t.Fatalf("default key in a lower-case branch = %q", got)
	}
	re, problems := TicketKey(`#\d+`)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	s = New(t.TempDir(), Settings{TicketKey: re}, nil)
	if got := s.keyIn("fix/#381-login"); got != "#381" {
		t.Fatalf("custom key = %q", got)
	}
	if got := s.keyIn("feature/ABC-12"); got != "" {
		t.Fatalf("custom key matched a Jira key: %q", got)
	}
	if re, problems := TicketKey("("); len(problems) != 1 || re.String() != DefaultTicketKey {
		t.Fatalf("bad pattern: %v, %v", re, problems)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
