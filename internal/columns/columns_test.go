package columns

import (
	"strings"
	"testing"
)

var defaults = Set{
	{ID: "todo", Label: "To Do", Color: "244", Icon: "a"},
	{ID: "doing", Label: "Working", Color: "39", Icon: "b"},
	{ID: "done", Label: "Done", Color: "78", Icon: "c"},
}

func ids(s Set) string {
	var out []string
	for _, c := range s {
		out = append(out, c.ID)
	}
	return strings.Join(out, ",")
}

func TestNoUserColumnsKeepsTheDefaults(t *testing.T) {
	got, problems := Merge(defaults, nil, false)
	if ids(got) != "todo,doing,done" || len(problems) != 0 {
		t.Fatalf("got %s %v", ids(got), problems)
	}
}

func TestRenameChangesOnlyWhatIsFilledIn(t *testing.T) {
	got, _ := Merge(defaults, []Column{{ID: "doing", Label: "Busy"}}, false)
	c, _ := got.Find("doing")
	if c.Label != "Busy" || c.Color != "39" || c.Icon != "b" {
		t.Fatalf("got %+v", c)
	}
}

func TestUserOrderWinsAndLeftOutDefaultsFollow(t *testing.T) {
	got, _ := Merge(defaults, []Column{{ID: "done"}, {ID: "todo"}}, false)
	if ids(got) != "done,todo,doing" {
		t.Fatalf("order %s", ids(got))
	}
}

func TestUnknownColumnDependsOnAllowNew(t *testing.T) {
	got, problems := Merge(defaults, []Column{{ID: "blocked", Label: "Blocked"}}, false)
	if ids(got) != "todo,doing,done" || len(problems) != 1 {
		t.Fatalf("closed set: %s %v", ids(got), problems)
	}
	got, problems = Merge(defaults, []Column{{ID: "todo"}, {ID: "blocked", Label: "Blocked"}}, true)
	if ids(got) != "todo,blocked,doing,done" || len(problems) != 0 {
		t.Fatalf("open set: %s %v", ids(got), problems)
	}
	if got.Label("blocked") != "Blocked" {
		t.Fatalf("label %q", got.Label("blocked"))
	}
}

func TestDuplicatesAndMissingIDsAreReported(t *testing.T) {
	_, problems := Merge(defaults, []Column{{Label: "x"}, {ID: "todo"}, {ID: "todo"}}, true)
	if len(problems) != 2 {
		t.Fatalf("problems %v", problems)
	}
}

func TestLabelFallsBackToTheID(t *testing.T) {
	if defaults.Label("nope") != "nope" {
		t.Fatal("missing column should show its id")
	}
}
