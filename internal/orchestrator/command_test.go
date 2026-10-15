package orchestrator

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestHelperProcess is the program a command test runs: it answers as a
// start command does and exits with the code in HELPER_CODE.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("HELPER_CODE") == "" {
		return
	}
	code := os.Getenv("HELPER_CODE")
	fmt.Printf("noise\n{\"code\":%s,\"message\":\"started %s\"}\n", code, os.Getenv("HELPER_ARG"))
	if code != "0" {
		os.Exit(1)
	}
	os.Exit(0)
}

func helper(t *testing.T, code string) []string {
	t.Setenv("HELPER_CODE", code)
	return []string{os.Args[0], "-test.run=TestHelperProcess", "{key}"}
}

func TestViaCommand(t *testing.T) {
	r := run(t)
	r.Worktree = t.TempDir()

	if _, ran, _ := viaCommand(r, nil); ran {
		t.Fatal("no command ran")
	}
	if _, ran, _ := viaCommand(r, []string{"definitely-not-a-program-xyz"}); ran {
		t.Fatal("a program that cannot start counts as run")
	}

	t.Setenv("HELPER_ARG", "x")
	text, ran, err := viaCommand(r, helper(t, "0"))
	if !ran || err != nil || text != "started x" {
		t.Fatalf("text %q ran %v err %v", text, ran, err)
	}

	// A command that ran and failed is final: the board does not start a second one.
	_, ran, err = viaCommand(r, helper(t, "9"))
	if !ran || err == nil || !strings.Contains(err.Error(), "started") {
		t.Fatalf("ran %v err %v", ran, err)
	}
}

func TestGoUsesTheStartCommandAndFallsBack(t *testing.T) {
	r := run(t)
	r.Workspace, r.Worktree = "w", t.TempDir()
	r.Orchestrator.Kind = "cursor"

	r.Orchestrator.Command = helper(t, "0")
	f := &fake{}
	text, err := Go(f, r)
	if err != nil || !strings.HasPrefix(text, "started") || len(f.started) != 0 {
		t.Fatalf("text %q err %v started %q", text, err, f.started)
	}

	r.Orchestrator.Command = []string{"definitely-not-a-program-xyz"}
	f = &fake{}
	if _, err := Go(f, r); err != nil || len(f.started) != 1 {
		t.Fatalf("no fall back: err %v started %q", err, f.started)
	}
}
