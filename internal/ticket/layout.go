package ticket

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LayoutConfig is the [run] table: where a team run lives and how its state
// file reads. Every field is optional; an empty one keeps its default.
type LayoutConfig struct {
	// Dir is the folder of runs inside a worktree; each run is Dir/<KEY>/.
	Dir string `toml:"dir"`
	// State is the run's state file inside its folder.
	State string `toml:"state"`
	// Dispatch matches the names of the files that send work to a role.
	Dispatch string `toml:"dispatch"`
	// TeamField, TargetField and SpecField name header fields of State.
	TeamField   string `toml:"team_field"`
	TargetField string `toml:"target_field"`
	SpecField   string `toml:"spec_field"`
	// The rest match "## " section headings of State, or lines in them.
	Objective   string `toml:"objective"`    // the run's goal; its first lines are shown
	CurrentStep string `toml:"current_step"` // what is happening now
	Landed      string `toml:"landed"`       // the landing report
	NotLanded   string `toml:"not_landed"`   // text in it that means it did not land
	Questions   string `toml:"questions"`    // sections whose bullets are open questions
	NotQuestion string `toml:"not_question"` // ... except these sections
	Settled     string `toml:"settled"`      // a bullet that is already answered
	// SpecCode matches a story code in the run's files; its groups, joined by
	// "_" with numbers padded to two digits, are the code. Empty: no specs.
	SpecCode string `toml:"spec_code"`
	// Prototypes is the folder under spec_clone holding a story's HTML
	// prototypes; {1}, {2}... are SpecCode's groups.
	Prototypes string `toml:"prototypes"`
}

// DefaultLayoutConfig is the layout a run has unless [run] says otherwise.
var DefaultLayoutConfig = LayoutConfig{
	Dir:         ".runs",
	State:       "STATE.md",
	Dispatch:    `(?i)^dispatch[-_].*\.md$`,
	TeamField:   "Team",
	TargetField: "Target",
	SpecField:   "Spec",
	Objective:   `(?i)^objective`,
	CurrentStep: `(?i)^(current step|pipeline)`,
	Landed:      `(?i)^landed`,
	NotLanded:   `(?i)not landed|exit 1|blocked`,
	Questions:   `(?i)open|pending|blocker|question`,
	NotQuestion: `(?i)defect`,
	Settled:     `(?i)\b(decided|ruled|answered|settled|deferred|closed)\b|defects: none|^-\s*none\b`,
}

// Layout is a LayoutConfig with its patterns compiled.
type Layout struct {
	Dir, State                        string
	TeamField, TargetField, SpecField string
	Prototypes                        string

	dispatch, objective, currentStep, landed, notLanded *regexp.Regexp
	questions, notQuestion, settled, specCode           *regexp.Regexp
}

// DefaultLayout is DefaultLayoutConfig compiled.
func DefaultLayout() Layout {
	l, _ := NewLayout(LayoutConfig{})
	return l
}

// NewLayout compiles c over the defaults. A pattern that does not compile
// keeps its default and is reported.
func NewLayout(c LayoutConfig) (Layout, []string) {
	d := DefaultLayoutConfig
	pick := func(v, def string) string {
		if strings.TrimSpace(v) == "" {
			return def
		}
		return v
	}
	l := Layout{
		Dir:         pick(c.Dir, d.Dir),
		State:       pick(c.State, d.State),
		TeamField:   pick(c.TeamField, d.TeamField),
		TargetField: pick(c.TargetField, d.TargetField),
		SpecField:   pick(c.SpecField, d.SpecField),
		Prototypes:  c.Prototypes,
	}
	var problems []string
	compile := func(name, v, def string) *regexp.Regexp {
		if v == "" {
			v = def
		}
		if v == "" {
			return nil
		}
		re, err := regexp.Compile(v)
		if err == nil {
			return re
		}
		problems = append(problems, fmt.Sprintf("run.%s %q does not compile (%v); using the default", name, v, err))
		if def == "" {
			return nil
		}
		return regexp.MustCompile(def)
	}
	l.dispatch = compile("dispatch", c.Dispatch, d.Dispatch)
	l.objective = compile("objective", c.Objective, d.Objective)
	l.currentStep = compile("current_step", c.CurrentStep, d.CurrentStep)
	l.landed = compile("landed", c.Landed, d.Landed)
	l.notLanded = compile("not_landed", c.NotLanded, d.NotLanded)
	l.questions = compile("questions", c.Questions, d.Questions)
	l.notQuestion = compile("not_question", c.NotQuestion, d.NotQuestion)
	l.settled = compile("settled", c.Settled, d.Settled)
	l.specCode = compile("spec_code", c.SpecCode, d.SpecCode)
	return l, problems
}

// IsLandedHeading reports whether a section heading is the landing report's.
func (l Layout) IsLandedHeading(heading string) bool { return l.landed.MatchString(heading) }

// normalizeSpec turns any spelling of a story code into its canonical form:
// the pattern's groups, upper-cased, joined by "_", numbers padded to two
// digits ("e7-us-5" -> "E7_US_05" for the groups E7, US, 5).
func (l Layout) normalizeSpec(s string) string {
	if l.specCode == nil {
		return ""
	}
	m := l.specCode.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	groups := m[1:]
	if len(groups) == 0 {
		groups = m[:1]
	}
	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		if g == "" {
			continue
		}
		if n, err := strconv.Atoi(g); err == nil {
			g = leftPad2(n)
		}
		parts = append(parts, strings.ToUpper(g))
	}
	return strings.Join(parts, "_")
}

// prototypeDir is Prototypes with the story code's parts put in.
func (l Layout) prototypeDir(spec string) string {
	if l.Prototypes == "" || spec == "" {
		return ""
	}
	out := l.Prototypes
	for i, part := range strings.Split(spec, "_") {
		out = strings.ReplaceAll(out, "{"+strconv.Itoa(i+1)+"}", part)
	}
	return out
}
