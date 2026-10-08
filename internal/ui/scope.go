package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// The board can be scoped to one git repository: the checkout it was opened in
// and every worktree of it. Other Herdr spaces stay off the board.

// SetScope limits the board to spaces in the same git repository as dir. A dir
// outside any repository leaves the board showing every space.
func (m *Model) SetScope(dir string) {
	m.scope = gitCommonDir(dir)
	m.scopeOf = map[string]string{}
}

// Scoped reports whether the board is limited to one repository.
func (m *Model) Scoped() bool { return m.scope != "" }

func (m *Model) inScope(key string) bool {
	if m.scope == "" {
		return true
	}
	common, ok := m.scopeOf[key]
	if !ok {
		common = gitCommonDir(key)
		m.scopeOf[key] = common
	}
	return common == m.scope
}

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
