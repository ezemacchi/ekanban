// Package archive keeps a copy of each ticket run's folder beside the board,
// so a run's notes -- its state file, its questions and its log -- outlive
// the worktree they were written in. A worktree removed by hand, or by a
// tool that knows nothing of the board, no longer takes them with it.
//
// The copy is laid out as the worktree held it (<copy>/.runs/<KEY>/...), so
// the readers of package ticket load it as they load a worktree.
package archive

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxFile is the largest file copied; a larger one is left out and counted.
// A run's notes are text: anything this big is an artifact, not a note.
const MaxFile = 8 << 20

// metaName is the file in a copy that says what it was copied from.
const metaName = ".ekanban-archive.json"

// Meta is what a copy records about itself.
type Meta struct {
	From      string    `json:"from"` // the run folder copied
	Worktree  string    `json:"worktree"`
	Signature string    `json:"signature"`
	At        time.Time `json:"at"`
	// Skipped are files left out for their size, by path in the run folder.
	Skipped []string `json:"skipped,omitempty"`
}

// Path is where key's copy is kept under the plugin's state folder, "" for a
// key that cannot be a folder name.
func Path(stateDir, key string) string {
	key = strings.TrimSpace(key)
	if key == "" || key == "." || key == ".." || strings.ContainsAny(key, `/\:`) {
		return ""
	}
	return filepath.Join(stateDir, "archive", key)
}

// Snapshot copies runDir, a folder inside worktree, to dest when anything
// in it changed since the last copy, and reports whether it copied. The old
// copy is replaced whole, never merged into.
func Snapshot(worktree, runDir, dest string) (bool, error) {
	if dest == "" {
		return false, errors.New("no archive folder for this ticket")
	}
	rel, err := filepath.Rel(worktree, runDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false, fmt.Errorf("%s is not inside %s", runDir, worktree)
	}
	files, err := listFiles(runDir)
	if err != nil {
		return false, err
	}
	sig := signature(files)
	if m, err := ReadMeta(dest); err == nil && m.Signature == sig {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), "."+filepath.Base(dest)+"-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)

	meta := Meta{From: runDir, Worktree: worktree, Signature: sig, At: time.Now().UTC()}
	for _, f := range files {
		if f.size > MaxFile {
			meta.Skipped = append(meta.Skipped, filepath.ToSlash(f.rel))
			continue
		}
		if err := copyFile(filepath.Join(runDir, f.rel), filepath.Join(tmp, rel, f.rel)); err != nil {
			return false, err
		}
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, metaName), append(data, '\n'), 0o644); err != nil {
		return false, err
	}
	if err := os.RemoveAll(dest); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return false, err
	}
	return true, nil
}

// ReadMeta reads what a copy records about itself.
func ReadMeta(dest string) (Meta, error) {
	var m Meta
	data, err := os.ReadFile(filepath.Join(dest, metaName))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}

type file struct {
	rel  string
	size int64
	mod  time.Time
}

func listFiles(root string) ([]file, error) {
	var out []file
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, file{rel: rel, size: info.Size(), mod: info.ModTime()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, err
}

// signature changes when a file is added, removed, resized or rewritten.
func signature(files []file) string {
	h := fnv.New64a()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", filepath.ToSlash(f.rel), f.size, f.mod.UnixNano())
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if info, err := in.Stat(); err == nil {
		_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	}
	return nil
}
