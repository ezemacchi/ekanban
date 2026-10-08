package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The board can be scoped to one git repository: the checkout it was opened in
// and every worktree of it. Other Herdr spaces stay off the board.

// SetScope limits the board to spaces in the same git repository as dir. A dir
// outside any repository leaves the board showing every space.
func (m *Model) SetScope(dir string) {
	m.scope = gitCommonDir(dir)
	m.scopeOf = map[string]scopeAnswer{}
}

// Scoped reports whether the board is limited to one repository.
func (m *Model) Scoped() bool { return m.scope != "" }

func (m *Model) inScope(key string) bool {
	if m.scope == "" {
		return true
	}
	if _, ok := jiraCardKey(key); ok {
		return true // a ticket, not a folder
	}
	ans, ok := m.scopeOf[key]
	// A "yes" is kept: a worktree does not move to another repository. A "no"
	// is only kept for a while, because it may be a read made while git was
	// still writing the worktree's files, or before the folder existed, and a
	// permanent "no" hides that worktree until the board is restarted.
	if !ok || (ans.common != m.scope && time.Since(ans.at) > scopeRecheck) {
		ans = scopeAnswer{common: gitCommonDir(key), at: time.Now()}
		m.scopeOf[key] = ans
	}
	return ans.common == m.scope
}

// scopeAnswer is what the board last read from a folder's git files.
type scopeAnswer struct {
	common string
	at     time.Time
}

// scopeRecheck is how long a folder that is not in the repository stays
// dismissed before its git files are read again. Reading two small files is
// cheap; this only keeps a rebuild from doing it for every foreign space.
const scopeRecheck = 10 * time.Second

// gitCommonDir returns the repository's shared .git directory for dir, which
// is the same for the main checkout and all its worktrees. It reads the files
// git leaves rather than running git, since the board rebuilds often.
func gitCommonDir(dir string) string {
	if dir == "" {
		return ""
	}
	for d := filepath.Clean(dir); ; {
		gitPath := filepath.Join(d, ".git")
		if st, err := os.Stat(gitPath); err == nil {
			if st.IsDir() {
				return normPath(gitPath)
			}
			// A worktree: ".git" is a file naming its private git directory,
			// whose "commondir" points back at the shared one.
			data, err := os.ReadFile(gitPath)
			if err != nil {
				return ""
			}
			private, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
			if !ok {
				return ""
			}
			private = strings.TrimSpace(private)
			if !filepath.IsAbs(private) {
				private = filepath.Join(d, private)
			}
			common := private
			if c, err := os.ReadFile(filepath.Join(private, "commondir")); err == nil {
				common = strings.TrimSpace(string(c))
				if !filepath.IsAbs(common) {
					common = filepath.Join(private, common)
				}
			}
			return normPath(common)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

func normPath(p string) string {
	p = filepath.Clean(p)
	if filepath.Separator == '\\' {
		p = strings.ToLower(p)
	}
	return p
}
