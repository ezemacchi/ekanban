package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Folders are grouped by repository, and what is not a checkout -- a plain
// folder the board lists as a space -- is left out.
func TestByRepo(t *testing.T) {
	root := t.TempDir()
	mk := func(name string, checkout bool) string {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if checkout {
			if err := os.WriteFile(filepath.Join(d, ".git"), []byte("gitdir: x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}
	a1, a2, b1, plain := mk("a-1", true), mk("a-2", true), mk("b-1", true), mk("notes", false)
	repo := map[string]string{a1: "/repo/a", a2: "/repo/a", b1: "/repo/b", plain: "/repo/a"}
	groups := byRepo([]string{a2, plain, b1, a1}, func(d string) string { return repo[d] })
	if len(groups) != 2 || groups[0].repo != "/repo/a" || groups[1].repo != "/repo/b" {
		t.Fatalf("groups: %+v", groups)
	}
	if len(groups[0].dirs) != 2 || groups[0].dirs[0] != a1 || groups[0].dirs[1] != a2 {
		t.Fatalf("repo a: %v", groups[0].dirs)
	}
}
