package store

import (
	"os"
	"path/filepath"
	"strings"
)

// CommonDir returns the repository's shared .git directory for dir, which is
// the same for the main checkout and all its worktrees: what Accepted.Repo
// holds. It reads the files git leaves rather than running git, since the
// board rebuilds often. "" outside a repository.
func CommonDir(dir string) string {
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

// SameDir says whether a and b are the same folder, as the board keys them;
// letter case only counts where the file system does.
func SameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return normPath(Key(a)) == normPath(Key(b))
}

func normPath(p string) string {
	p = filepath.Clean(p)
	if filepath.Separator == '\\' {
		p = strings.ToLower(p)
	}
	return p
}
