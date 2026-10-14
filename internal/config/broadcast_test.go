package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fast_forward moves the reader's own checkouts, so a repository file cannot
// turn it on, and says so.
func TestRepoFileCannotTurnOnFastForward(t *testing.T) {
	write(t, "")
	main, wt := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), "[pipeline]\nfast_forward = true\n")
	s := LoadFor(wt)
	if s.Pipeline.FastForward {
		t.Fatal("the repository file turned fast_forward on")
	}
	if !slices.ContainsFunc(s.Problems, func(p string) bool { return strings.Contains(p, "fast_forward is only read from config.toml") }) {
		t.Fatalf("no word about what was ignored: %q", s.Problems)
	}

	write(t, "[pipeline]\nfast_forward = true\n")
	if !LoadFor(wt).Pipeline.FastForward {
		t.Fatal("config.toml could not turn fast_forward on")
	}
}

// Moving cards by hand is the team's process: unset means on, and the
// repository file can turn it off over config.toml.
func TestManualMovesComeFromTheRepository(t *testing.T) {
	write(t, "")
	main, wt := fakeRepo(t)
	if LoadFor(wt).Pipeline.ManualMoves != nil {
		t.Fatal("manual_moves set with nothing written")
	}
	write(t, "[pipeline]\nmanual_moves = true\n")
	put(t, filepath.Join(main, RepoFileName), "[pipeline]\nmanual_moves = false\n")
	if mm := LoadFor(wt).Pipeline.ManualMoves; mm == nil || *mm {
		t.Fatalf("the repository file did not turn manual_moves off: %v", mm)
	}
}

// The broadcast's presets come from the team's repository file, or default.
func TestBroadcastPresetsComeFromTheRepositoryOrDefault(t *testing.T) {
	write(t, "")
	main, wt := fakeRepo(t)
	if got := LoadFor(wt).Broadcast; !slices.Equal(got, DefaultPresets) {
		t.Fatalf("defaults %q", got)
	}
	put(t, filepath.Join(main, RepoFileName), "[broadcast]\npresets = [\"Rebase onto {target}\"]\n")
	if got := LoadFor(wt).Broadcast; !slices.Equal(got, []string{"Rebase onto {target}"}) {
		t.Fatalf("from the repository %q", got)
	}
}
