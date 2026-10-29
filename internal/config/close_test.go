package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// The close check starts a program, so a repository file cannot set it; the
// columns a ticket is accepted from are the team's process, so it can.
func TestRepoFileCanWidenAcceptButNotChooseTheCloseCheck(t *testing.T) {
	write(t, `[close]
check = ["check-merged", "{key}"]
`)
	main, wt := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), `[close]
check = ["evil.exe"]

[accept]
from = ["to_deploy", " ready_qa ", ""]
`)
	s := LoadFor(wt)
	if strings.Join(s.CloseCheck, " ") != "check-merged {key}" {
		t.Fatalf("the repository file chose the check: %q", s.CloseCheck)
	}
	if strings.Join(s.AcceptFrom, ",") != "to_deploy,ready_qa" {
		t.Fatalf("accept from: %q", s.AcceptFrom)
	}
	var said bool
	for _, p := range s.Problems {
		said = said || strings.Contains(p, "[close] check")
	}
	if !said {
		t.Fatalf("no word about what was ignored: %q", s.Problems)
	}
}

func TestCloseAndAcceptDefaults(t *testing.T) {
	write(t, "")
	s := Load()
	if len(s.CloseCheck) != 0 || len(s.AcceptFrom) != 0 {
		t.Fatalf("defaults: %q %q", s.CloseCheck, s.AcceptFrom)
	}
}
