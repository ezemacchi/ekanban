package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/ticket"
)

// commandFor is how long a start command may take: it waits for the agent to
// come up and take its prompt.
const commandFor = 3 * time.Minute

// viaCommand runs the configured command that starts run's orchestrator.
// ran is false when there is no command, or it could not be started at all,
// and the caller starts the orchestrator itself. Once the command has run its
// answer is final, whatever its exit code: it may have started the agent
// already, and a second one would be worse than an error.
func viaCommand(run *ticket.Run, command []string) (text string, ran bool, err error) {
	if len(command) == 0 {
		return "", false, nil
	}
	args := make([]string, len(command))
	for i, a := range command {
		args[i] = Expand(a, run)
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandFor)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = run.Worktree
	cmd.Env = commandEnv(os.Environ())
	gh.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) && ctx.Err() == nil {
		return "", false, nil // not started: the program is missing or cannot run
	}
	text, ok := commandResult(out, err)
	if !ok {
		return "", true, errors.New(text)
	}
	return text, true, nil
}

// commandEnv tells the command it runs inside Herdr, which it does, when the
// plugin's environment does not say so.
func commandEnv(env []string) []string {
	has := func(name string) bool {
		for _, e := range env {
			if v, ok := strings.CutPrefix(e, name+"="); ok && v != "" {
				return true
			}
		}
		return false
	}
	if !has("HERDR_ENV") {
		env = append(env, "HERDR_ENV=1")
	}
	if !has("HERDR_BIN_PATH") {
		if p, err := exec.LookPath("herdr"); err == nil {
			env = append(env, "HERDR_BIN_PATH="+p)
		}
	}
	return env
}

// commandResult reads the command's answer: the last line that is a JSON
// object with a message (and a code, 0 for done), else its last output line.
func commandResult(out []byte, err error) (string, bool) {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var r struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
		}
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "{") && json.Unmarshal([]byte(l), &r) == nil && r.Message != "" {
			return r.Message, err == nil && (r.Code == nil || *r.Code == 0)
		}
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if err != nil {
		if last == "" {
			return err.Error(), false
		}
		return last + " (" + err.Error() + ")", false
	}
	if last == "" {
		last = "done"
	}
	return last, true
}
