package ui

import (
	"time"

	"github.com/ezemacchi/ekanban/internal/store"
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
	if noWorktree(key) {
		return true // a ticket or a pull request, not a folder
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

// gitCommonDir is store.CommonDir: the repository's shared .git directory.
func gitCommonDir(dir string) string { return store.CommonDir(dir) }
