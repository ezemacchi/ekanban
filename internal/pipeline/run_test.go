package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ezemacchi/ekanban/internal/ticket"
)

// The global board finds a run's prototype in spec_clone, as the ticket
// board does: without SpecRoot it never could.
func TestRunFindsThePrototypeInTheSpecsClone(t *testing.T) {
	layout, problems := ticket.NewLayout(ticket.LayoutConfig{
		SpecCode:   `(?i)(?:^|[^a-z0-9])(E\d+)[_-](US|TS)[_-](\d+)`,
		Prototypes: "specifications/backlog/{1}",
	})
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	specs := t.TempDir()
	page := filepath.Join(specs, "specifications", "backlog", "E7", "prototypes", "E7_US_42_batch_00_index.html")
	wt := t.TempDir()
	state := filepath.Join(wt, ".runs", "ABC-1", "STATE.md")
	for path, text := range map[string]string{page: "x", state: "Team: Example\nSpec: e7-us-42\n"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run, err := New("", Settings{Layout: &layout, SpecRoot: specs}, nil).Run(wt, ticket.Live{})
	if err != nil {
		t.Fatal(err)
	}
	if run.Prototype != page {
		t.Fatalf("prototype %q, want %q", run.Prototype, page)
	}
}
