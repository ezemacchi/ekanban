package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinTeamsLoad(t *testing.T) {
	teams, problems := Load("")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	mc, _ := Pick(teams, "Big Team")
	if mc.Name != "Big Team" || len(mc.Roles) != 6 {
		t.Fatalf("big team: %+v", mc)
	}
	pit, _ := Pick(teams, "Full Team")
	if pit.Name != "Full Team" || len(pit.Roles) != 2 {
		t.Fatalf("full team: %+v", pit)
	}
	if d, _ := Pick(teams, "something else"); d.Name != "Big Team" {
		t.Fatalf("default team is %q", d.Name)
	}
	if !mc.Roles[0].Matches("dispatch-technical-lead.md") {
		t.Fatal("technical lead does not match its dispatch file")
	}
}

func TestUserFileReplacesAndAdds(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("full-team.toml", "name = 'Full Team'\nmatch = '(?i)full'\n[[role]]\nid = 'solo'\nmatch = 'solo'\ndone_file = 'DONE.md'\n")
	write("design.toml", "name = 'Design'\nmatch = '(?i)design'\n[[role]]\nid = 'designer'\nlabel = 'Designer'\nmatch = 'designer'\n")
	write("broken.toml", "name = 'Broken'\n[[role]]\nid = 'x'\nmatch = '('\n")

	teams, problems := Load(dir)
	if len(problems) != 1 || !strings.Contains(problems[0], "broken.toml") {
		t.Fatalf("problems: %v", problems)
	}
	pit, _ := Pick(teams, "Full Team")
	if len(pit.Roles) != 1 || pit.Roles[0].ID != "solo" {
		t.Fatalf("user file did not replace the built-in: %+v", pit.Roles)
	}
	design, _ := Pick(teams, "Design team")
	if design.Name != "Design" {
		t.Fatalf("user team not added: %q", design.Name)
	}
}
