// Package relay hands the watcher's notices to an agent as a message in its
// conversation. A toast reaches the person; an agent working for them, one
// that keeps an eye on the runs, hears nothing unless someone tells it. With a
// relay it learns that an orchestrator finished or a pull request merged
// without being asked "has it finished yet?".
package relay

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/alert"
	"github.com/ezemacchi/ekanban/internal/herdr"
)

// Prefix opens every relayed message, so the agent can tell it from the
// person typing.
const Prefix = "[ekanban]"

// Notice is one alert waiting to be relayed.
type Notice struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// Relay is where notices go and what has not reached it yet.
type Relay struct {
	// Pane hosts the agent notices go to; "" when nobody asked for them.
	Pane    string    `json:"pane,omitempty"`
	Since   time.Time `json:"since,omitempty"`
	Pending []Notice  `json:"pending,omitempty"`

	path   string
	loaded string // Pane as read, so a save does not undo a change made meanwhile
}

// maxPending bounds what waits for an agent that never becomes free.
const maxPending = 50

// Load reads the relay, an empty one when there is none.
func Load(dir string) *Relay {
	r := &Relay{path: filepath.Join(dir, "relay.json")}
	if data, err := os.ReadFile(r.path); err == nil {
		var loaded Relay
		if json.Unmarshal(data, &loaded) == nil {
			r.Pane, r.Since, r.Pending = loaded.Pane, loaded.Since, loaded.Pending
		}
	}
	r.loaded = r.Pane
	return r
}

// On is whether notices go anywhere.
func (r *Relay) On() bool { return r.Pane != "" }

// To points the relay at pane, dropping what waited for an earlier one.
func (r *Relay) To(pane string, now time.Time) {
	r.Pane, r.Since, r.Pending = pane, now.UTC(), nil
}

// Off stops relaying.
func (r *Relay) Off() { r.Pane, r.Since, r.Pending = "", time.Time{}, nil }

// Queue keeps the alerts for the agent, reporting whether any were added. With
// no relay they are not kept: an agent pointed at it later starts from then.
func (r *Relay) Queue(alerts []alert.Alert) bool {
	if !r.On() || len(alerts) == 0 {
		return false
	}
	for _, a := range alerts {
		r.Pending = append(r.Pending, Notice{Text: a.Text, At: a.At})
	}
	if len(r.Pending) > maxPending {
		r.Pending = r.Pending[len(r.Pending)-maxPending:]
	}
	return true
}

// Message is the pending notices as one line, which an agent's input takes
// as it is.
func Message(pending []Notice) string {
	texts := make([]string, 0, len(pending))
	for _, n := range pending {
		texts = append(texts, n.Text)
	}
	return Prefix + " " + strings.Join(texts, "; ")
}

// Deliver sends what is pending as one message once the agent can take it,
// reporting whether the relay changed. An agent idle or done gets it; one
// working, or asking the person something, is left alone and told on a later
// call. A pane with no agent in it is waited for, since an agent restarting
// comes back there; a pane that is gone ends the relay. agents and panes must
// be a successful reading: an empty one would end every relay.
func (r *Relay) Deliver(agents []herdr.Agent, panes []herdr.Pane, submit func(pane, text string) error) (bool, error) {
	if !r.On() || len(r.Pending) == 0 {
		return false, nil
	}
	var to *herdr.Agent
	for i := range agents {
		if agents[i].PaneID == r.Pane && agents[i].Agent != nil {
			to = &agents[i]
		}
	}
	if to == nil {
		for _, p := range panes {
			if p.PaneID == r.Pane {
				return false, nil
			}
		}
		r.Off()
		return true, nil
	}
	if to.AgentStatus != "idle" && to.AgentStatus != "done" {
		return false, nil
	}
	if err := submit(r.Pane, Message(r.Pending)); err != nil {
		if herdr.IsBlocked(err) {
			return false, nil
		}
		return false, err
	}
	r.Pending = nil
	return true, nil
}

// Save writes the relay, unless someone pointed it elsewhere or turned it off
// since it was loaded: their choice wins over a delivery's bookkeeping.
func (r *Relay) Save() error {
	if r.path == "" {
		return errors.New("relay has no path")
	}
	if current := Load(filepath.Dir(r.path)); current.Pane != r.loaded {
		return nil
	}
	return r.write()
}

// Replace writes the relay whatever is on disk: what the person's own
// command does.
func (r *Relay) Replace() error {
	if r.path == "" {
		return errors.New("relay has no path")
	}
	return r.write()
}

func (r *Relay) write() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return err
	}
	r.loaded = r.Pane
	return nil
}
