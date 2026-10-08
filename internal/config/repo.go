package config

import (
	"os"
	"path/filepath"
	"strings"
)

// RepoFileName is the per-repository settings file, read over config.toml.
const RepoFileName = ".ekanban.toml"

// RepoFile finds the settings file of the repository dir is in: in the root
// of dir's checkout, else in the main checkout when dir is a linked worktree,
// since a file kept out of git exists only where it was created. It is ""
// when dir is in no repository or neither place has one.
//
// It reads .git itself rather than running git, so opening a board starts no
// process.
func RepoFile(dir string) string {
	if dir == "" {
		return ""
	}
	root, gitPath := checkoutRoot(dir)
	if root == "" {
		return ""
	}
	candidates := []string{filepath.Join(root, RepoFileName)}
	if main := mainCheckout(gitPath); main != "" && main != root {
		candidates = append(candidates, filepath.Join(main, RepoFileName))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

// checkoutRoot walks up from dir to the folder holding .git.
func checkoutRoot(dir string) (root, gitPath string) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", ""
	}
	for {
		p := filepath.Join(d, ".git")
		if _, err := os.Stat(p); err == nil {
			return d, p
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", ""
		}
		d = parent
	}
}

// mainCheckout is the main checkout of a linked worktree, whose .git is a
// file "gitdir: <main>/.git/worktrees/<name>"; "" for a main checkout.
func mainCheckout(gitPath string) string {
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "" // a directory: this is the main checkout
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return ""
	}
	gitDir = strings.TrimSpace(gitDir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(filepath.Dir(gitPath), gitDir)
	}
	// commondir names the shared git directory, relative to the worktree's.
	common := filepath.Join(gitDir, "..", "..")
	if c, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common = strings.TrimSpace(string(c))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
	}
	common = filepath.Clean(common)
	if !strings.EqualFold(filepath.Base(common), ".git") {
		return "" // a bare repository has no checkout to look in
	}
	return filepath.Dir(common)
}
