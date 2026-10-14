package reviews

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// commandTimeout bounds one run of the command, so a CLI waiting for a login
// prompt cannot hold the board's read open.
const commandTimeout = time.Minute

// commandSource runs a program that prints the requests as a JSON array of
// objects with id, title, branch, target, author, status, url and updated
// (RFC 3339). Only id is required.
type commandSource struct {
	args []string
	dir  string
	hide func(*exec.Cmd)
}

// Command is the builder of the command kind. dir is where the program runs
// (the board's checkout, so gh and glab find the repository); hide keeps a
// console window from opening on Windows.
func Command(dir string, hide func(*exec.Cmd)) Builder {
	if hide == nil {
		hide = func(*exec.Cmd) {}
	}
	return func(cfg Config) (Source, error) {
		if len(cfg.Command) == 0 || strings.TrimSpace(cfg.Command[0]) == "" {
			return nil, errors.New("[reviews] kind \"command\" needs command = [\"program\", \"arg\", ...]")
		}
		return &commandSource{args: append([]string(nil), cfg.Command...), dir: dir, hide: hide}, nil
	}
}

func (c *commandSource) Name() string { return filepath.Base(c.args[0]) }

func (c *commandSource) Awaiting(ctx context.Context) ([]Request, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.args[0], c.args[1:]...)
	cmd.Dir = c.dir
	c.hide(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s did not answer within %s", c.Name(), commandTimeout)
		}
		if last := lastLine(stderr.String()); last != "" {
			return nil, fmt.Errorf("%s: %s", c.Name(), last)
		}
		return nil, fmt.Errorf("%s: %v", c.Name(), err)
	}
	return parseRequests(stdout.Bytes())
}

// parseRequests reads the command's answer. An entry without an id is
// dropped, and an id given twice is kept once.
func parseRequests(out []byte) ([]Request, error) {
	var raw []struct {
		ID      json.RawMessage `json:"id"`
		Title   string          `json:"title"`
		Branch  string          `json:"branch"`
		Target  string          `json:"target"`
		Author  string          `json:"author"`
		Status  string          `json:"status"`
		URL     string          `json:"url"`
		Updated string          `json:"updated"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &raw); err != nil {
		return nil, fmt.Errorf("the review command did not print a JSON array: %v", err)
	}
	var list []Request
	seen := map[string]bool{}
	for _, r := range raw {
		id := strings.Trim(strings.TrimSpace(string(r.ID)), `"`)
		if id == "" || id == "null" || seen[id] {
			continue
		}
		seen[id] = true
		req := Request{ID: id, Title: r.Title, Branch: r.Branch, Target: r.Target, Author: r.Author, Status: r.Status, URL: r.URL}
		if t, err := time.Parse(time.RFC3339, r.Updated); err == nil {
			req.Updated = t
		}
		list = append(list, req)
	}
	return list, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
