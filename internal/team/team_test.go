package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinTeamLoads(t *testing.T) {
	teams, problems := Load("")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	if len(teams) != 1 {
		t.Fatalf("built-in teams: %d", len(teams))
	}
	ex, _ := Pick(teams, "anything")
	if ex.Name != "Example" || len(ex.Roles) != 4 || ex.Lead.Label != "Lead" {
		t.Fatalf("example: %+v", ex)
	}
	if !ex.Roles[0].Matches("dispatch-01-planner.md") || !ex.Lead.Matches("abc-1-lead") || ex.Lead.Matches("leader") {
		t.Fatal("example matches")
	}
}

func TestUsersDefaultTeamWinsOverTheExample(t *testing.T) {
	dir := t.TempDir()
	body := "name = 'Mine'\nmatch = '(?i)mine'\ndefault = true\n[[role]]\nid = 'solo'\nmatch = 'solo'\n"
	if err := os.WriteFile(filepath.Join(dir, "a-mine.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	teams, _ := Load(dir)
	if d, _ := Pick(teams, "no such team"); d.Name != "Mine" {
		t.Fatalf("default team is %q", d.Name)
	}
}
func TestUserFileReplacesAndAdds(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("example.toml", "name = 'Example'\nmatch = '(?i)example'\n[[role]]\nid = 'solo'\nmatch = 'solo'\ndone_file = 'DONE.md'\n")
	write("design.toml", "name = 'Design'\nmatch = '(?i)design'\n[[role]]\nid = 'designer'\nlabel = 'Designer'\nmatch = 'designer'\n")
	write("broken.toml", "name = 'Broken'\n[[role]]\nid = 'x'\nmatch = '('\n")

	teams, problems := Load(dir)
	if len(problems) != 1 || !strings.Contains(problems[0], "broken.toml") {
		t.Fatalf("problems: %v", problems)
	}
	ex, _ := Pick(teams, "Example")
	if len(ex.Roles) != 1 || ex.Roles[0].ID != "solo" {
		t.Fatalf("user file did not replace the built-in: %+v", ex.Roles)
	}
	design, _ := Pick(teams, "Design team")
	if design.Name != "Design" {
		t.Fatalf("user team not added: %q", design.Name)
	}
}
