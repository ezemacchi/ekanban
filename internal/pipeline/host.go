package pipeline

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// CodeHost is where pull requests live, as far as git can tell: the subject
// its merge commits carry. The board finds a merged pull request in the
// target branches' history, so it needs no credentials for the host.
// Bitbucket and GitHub are built in; "any" accepts both. Another host is an
// implementation plus a RegisterHost call, chosen by pipeline.code_host.
type CodeHost interface {
	// MergeSubjects are fixed strings, any of which marks pull request pr's
	// merge commit.
	MergeSubjects(pr int) []string
	// BranchSubject is a fixed string a merge commit of branch contains, or
	// "" when the host's subjects do not name the branch usefully.
	BranchSubject(branch string) string
	// NamesBranch says whether a merge commit's subject names branch as the
	// one merged. It is how a branch rewritten before merging is still
	// matched to its merge.
	NamesBranch(subject, branch string) bool
}

type subjects struct {
	merge  []string // fmt patterns with %d for the pull request
	branch string   // fmt pattern with %s for the branch, or ""
	// owner is set for subjects that name the branch as owner/branch.
	owner bool
}

// NamesBranch: Bitbucket writes "from <branch> to <target>", GitHub
// "from <owner>/<branch>", where an owner has no slash.
func (s subjects) NamesBranch(subject, branch string) bool {
	if branch == "" {
		return false
	}
	if s.branch != "" && strings.Contains(subject, fmt.Sprintf(s.branch, branch)) {
		return true
	}
	if !s.owner {
		return false
	}
	_, from, ok := strings.Cut(subject, " from ")
	if !ok {
		return false
	}
	_, named, ok := strings.Cut(strings.TrimSpace(from), "/")
	return ok && named == branch
}

func (s subjects) MergeSubjects(pr int) []string {
	out := make([]string, len(s.merge))
	for i, p := range s.merge {
		out[i] = fmt.Sprintf(p, pr)
	}
	return out
}

func (s subjects) BranchSubject(branch string) string {
	if s.branch == "" {
		return ""
	}
	return fmt.Sprintf(s.branch, branch)
}

var (
	bitbucket = subjects{
		merge:  []string{"Pull request #%d:", "Merge pull request #%d in "},
		branch: "from %s to ",
	}
	// GitHub's subject names owner/branch, which a fixed string cannot match
	// without the owner; the branch is found from the build server instead.
	github = subjects{merge: []string{"Merge pull request #%d from "}, owner: true}
	both   = subjects{merge: append(append([]string(nil), bitbucket.merge...), github.merge...), branch: bitbucket.branch, owner: true}
)

var (
	hostMu       sync.Mutex
	hostRegistry = map[string]CodeHost{"bitbucket": bitbucket, "github": github, "any": both}
)

// RegisterHost makes a code host available to pipeline.code_host.
func RegisterHost(name string, h CodeHost) {
	hostMu.Lock()
	defer hostMu.Unlock()
	if _, dup := hostRegistry[name]; dup {
		panic("pipeline: code host " + name + " registered twice")
	}
	hostRegistry[name] = h
}

// Host is the named code host; "" is "any". An unknown name is reported and
// falls back to "any".
func Host(name string) (CodeHost, []string) {
	if name == "" {
		name = "any"
	}
	hostMu.Lock()
	defer hostMu.Unlock()
	if h, ok := hostRegistry[name]; ok {
		return h, nil
	}
	var known []string
	for k := range hostRegistry {
		known = append(known, k)
	}
	sort.Strings(known)
	return hostRegistry["any"], []string{fmt.Sprintf("unknown code_host %q (known: %v); using any", name, known)}
}

var prNumber = regexp.MustCompile(`#(\d+)`)

func numberIn(subject string) int {
	if m := prNumber.FindStringSubmatch(subject); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// DefaultTicketKey is a Jira-style key: PROJ-123.
const DefaultTicketKey = `[A-Z][A-Z0-9]+-\d+`

// TicketKey compiles pipeline's ticket_key; "" is DefaultTicketKey. A pattern
// that does not compile is reported and the default is used.
func TicketKey(pattern string) (*regexp.Regexp, []string) {
	if pattern == "" {
		return regexp.MustCompile(DefaultTicketKey), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return regexp.MustCompile(DefaultTicketKey), []string{fmt.Sprintf("ticket_key %q does not compile (%v); using %s", pattern, err, DefaultTicketKey)}
	}
	return re, nil
}
