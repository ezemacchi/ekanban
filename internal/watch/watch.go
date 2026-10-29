// Package watch polls pull request state while the board is closed.
//
// Herdr has no timer in its API -- only events -- so periodic work needs a
// process of our own. This one is deliberately small: poll, diff, notify,
// record, and get out of the way when Herdr goes.
package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/alert"
	"github.com/ezemacchi/ekanban/internal/config"
	"github.com/ezemacchi/ekanban/internal/gh"
	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/relay"
	"github.com/ezemacchi/ekanban/internal/store"
)

// Interval is the default cadence, overridden by poll_interval in the config
// file. Slow enough to be inconsequential against GitHub's rate limit, fast
// enough that a failing check reaches you while you still care.
const Interval = config.DefaultPollInterval

// Lock is a pidfile ensuring one watcher per machine. The board spawns a
// watcher whenever it opens, so without this every open would add another.
type Lock struct{ path string }

// Acquire takes the lock, or reports that somebody else holds it.
func Acquire(dir string) (*Lock, bool) {
	path := filepath.Join(dir, "watch.pid")

	if data, err := os.ReadFile(path); err == nil {
		if pid, err := strconv.Atoi(string(data)); err == nil && alive(pid) {
			return nil, false
		}
		// A pidfile left by a killed watcher, or one whose pid has been
		// reused by something else. Either way it is not a live watcher.
		_ = os.Remove(path)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, false
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		// Lost the race to another watcher starting at the same moment.
		return nil, false
	}
	fmt.Fprintf(f, "%d", os.Getpid())
	f.Close()
	return &Lock{path: path}, true
}

// Release drops the lock.
func (l *Lock) Release() {
	if l != nil {
		_ = os.Remove(l.path)
	}
}

// refreshEvery is how often the tickets' targets are fetched and the build
// server reread; the polls between answer from the last reading, which is
// enough for what the agents and the run's files say.
const refreshEvery = 5 * time.Minute

// Run polls until the context is cancelled or Herdr goes away. tickets reads
// the ticket worktrees for their notices; nil watches pull requests only.
func Run(ctx context.Context, stateDir string, interval time.Duration, tickets Tickets) error {
	lock, ok := Acquire(stateDir)
	if !ok {
		// Another watcher is already doing this work.
		return nil
	}
	defer lock.Release()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// The event subscription doubles as a liveness signal: Herdr closing
	// ends the watcher at once rather than on its next tick, up to a whole
	// interval later.
	//
	// It is also the nudge. A workspace appearing or closing changes what
	// there is to watch, so everything is polled then; an agent changing
	// status, finishing a turn or stopping at a prompt, polls the tickets
	// then rather than minutes later.
	spaces, agents := subscribe(ctx, cancel)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	settings := config.Load()
	client := gh.NewBackground()

	var refreshed, polled time.Time
	full := true
	for {
		prs := client
		if !full {
			prs = nil // an agent's status says nothing about pull requests
		}
		refresh := full && time.Since(refreshed) >= refreshEvery
		if refresh {
			refreshed = time.Now()
		}
		polled = time.Now()
		if err := poll(ctx, stateDir, settings.Notifications, prs, tickets, refresh); err != nil {
			// Herdr has stopped, so there is nothing to watch and nowhere to
			// deliver a notification. Exiting beats spinning.
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			full = true
		case <-spaces:
			// Coalesce a burst of events into one poll: Herdr emits freely,
			// and gh calls are the expensive part.
			settleBurst(settle, spaces, agents)
			full = true
		case <-agents:
			// Busy agents change status all the time; reading the tickets on
			// every change would keep the watcher reading for nothing.
			wait := max(settle, agentGap-time.Since(polled))
			full = settleBurst(wait, spaces, agents)
		}
	}
}

// settle is how long to wait for an event burst to finish before polling.
const settle = 2 * time.Second

// agentGap is the least time between two polls brought on by agents alone.
const agentGap = 10 * time.Second

// Waits before subscribing again after Herdr dropped a subscription.
const (
	retryMin = time.Second
	retryMax = 30 * time.Second
)

// subscribe streams Herdr's events for as long as Herdr runs: workspace events
// nudge spaces, and agents coming, going or changing status nudge agents.
//
// Herdr ends a stream without going away: it drops a subscriber that fell
// behind (events_lost) and rejects a subscription naming a pane that just
// closed. Those are subscribed again, with a full poll to catch up on what was
// missed. Only Herdr itself not answering cancels ctx.
func subscribe(ctx context.Context, cancel context.CancelFunc) (spaces, agents <-chan struct{}) {
	sp, ag := make(chan struct{}, 1), make(chan struct{}, 1)
	client, err := herdr.New()
	if err != nil {
		cancel()
		return sp, ag
	}
	go func() {
		defer cancel()
		wait := retryMin
		for ctx.Err() == nil {
			started := time.Now()
			if panes, err := agentPanes(client); err == nil {
				stream(ctx, client, panes, sp, ag)
			}
			if ctx.Err() != nil || client.Ping() != nil {
				return
			}
			poke(sp)
			if time.Since(started) > time.Minute {
				wait = retryMin
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = min(2*wait, retryMax)
		}
	}()
	return sp, ag
}

// stream holds one subscription until it ends, or until the agent panes
// change: status events are per pane, so a new agent needs a new subscription.
func stream(ctx context.Context, client *herdr.Client, panes []string, spaces, agents chan struct{}) {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	events := make(chan herdr.Event, 64)
	go func() {
		_ = client.SubscribeTo(ctx, herdr.AgentSubscriptions(panes), events)
		close(events)
	}()
	key := strings.Join(panes, ",")
	for ev := range events {
		if strings.HasPrefix(ev.Event, "workspace.") {
			poke(spaces)
			continue
		}
		poke(agents)
		if ev.Event == herdr.StatusChanged {
			continue
		}
		if now, err := agentPanes(client); err == nil && strings.Join(now, ",") != key {
			stop()
		}
	}
}

// agentPanes are the panes running an agent, sorted.
func agentPanes(client *herdr.Client) ([]string, error) {
	list, err := client.Agents()
	if err != nil {
		return nil, err
	}
	var panes []string
	for _, a := range list {
		if a.Agent != nil {
			panes = append(panes, a.PaneID)
		}
	}
	sort.Strings(panes)
	return panes, nil
}

// poke nudges ch, unless a nudge is already pending.
func poke(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// settleBurst swallows further nudges for window, so a burst of events costs
// one poll rather than one per event, and reports whether spaces had any.
func settleBurst(window time.Duration, spaces, agents <-chan struct{}) (sawSpaces bool) {
	timer := time.NewTimer(window)
	defer timer.Stop()
	for {
		select {
		case <-spaces:
			sawSpaces = true
		case <-agents:
		case <-timer.C:
			return sawSpaces
		}
	}
}

// poll does one round: read the board, ask GitHub and the tickets' pipeline,
// notify on what changed.
//
// The GitHub client and the ticket reader are passed in rather than built
// here, so this -- the part that decides what is worth waking someone for --
// can be tested without a network.
func poll(ctx context.Context, stateDir string, notify bool, prs *gh.Client, tickets Tickets, refresh bool) error {
	client, err := herdr.New()
	if err != nil {
		return err
	}
	workspaces, err := client.Workspaces()
	if err != nil {
		return err
	}

	board, err := store.Load()
	if err != nil {
		return err
	}

	// A worktree that went without being accepted leaves no other trace, so
	// write it down on the first poll that sees it gone. This only reads the
	// board; an open board is the one that rewrites it.
	_, _ = board.RecordVanished(time.Now())

	// Watch every space the board knows about: the live ones, plus stored
	// entries whose workspace is currently closed. A PR you are waiting on
	// matters whether or not its window happens to be open.
	labels := map[string]string{}
	for _, ws := range workspaces {
		if key := store.Key(ws.Cwd); key != "" {
			labels[key] = ws.Label
		}
	}
	for key, entry := range board.Entries {
		if _, ok := labels[key]; !ok {
			labels[key] = entry.Label
		}
	}
	if len(labels) == 0 {
		relayNotices(stateDir, client, nil)
		return nil
	}

	raised, merged := prAlerts(ctx, stateDir, labels, prs)
	dirs := make([]string, 0, len(labels))
	for dir := range labels {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	raised = append(raised, pollTickets(ctx, stateDir, dirs, tickets, refresh, merged)...)

	// Notify after saving: a toast the user never sees is still recorded, but
	// a recorded alert the user was never told about is the worse failure.
	// With notifications off the bell still appears -- quiet, not blind.
	if notify {
		for _, a := range raised {
			_ = client.Notify(a.Title(), a.Text, a.Sound())
		}
	}
	relayNotices(stateDir, client, raised)
	return nil
}

// relayNotices keeps what was raised for the agent `ekanban relay` pointed the
// notices at, and hands over whatever waits once that agent can take it. A
// poll brought on by that agent going idle is what delivers what waited while
// it worked.
func relayNotices(stateDir string, client *herdr.Client, raised []alert.Alert) {
	r := relay.Load(stateDir)
	if !r.On() {
		return
	}
	changed := r.Queue(raised)
	agents, err := client.Agents()
	panes, perr := client.Panes()
	if err == nil && perr == nil {
		if delivered, _ := r.Deliver(agents, panes, client.SubmitToAgent); delivered {
			changed = true
		}
	}
	if changed {
		_ = r.Save()
	}
}

// prAlerts asks GitHub about every space's pull request and records what
// changed; merged are the spaces whose pull request it saw merge. gh missing
// or logged out is nothing to compare, and nothing worth waking the user for.
func prAlerts(ctx context.Context, stateDir string, labels map[string]string, prs *gh.Client) (raised []alert.Alert, merged map[string]bool) {
	merged = map[string]bool{}
	if prs == nil {
		return nil, merged
	}
	cache := gh.LoadCache(stateDir)
	targets := make([]gh.Target, 0, len(labels))
	for key := range labels {
		targets = append(targets, gh.Target{Dir: key})
	}

	found, problem := prs.FetchAll(ctx, targets)
	if problem != nil {
		return nil, merged
	}

	fresh := alert.Load(stateDir)

	for dir, after := range found {
		before := cache.Entries[dir]
		label := labels[dir]
		if label == "" {
			label = filepath.Base(dir)
		}
		for _, a := range alert.Transitions(label, before, after) {
			a.At = time.Now().UTC()
			fresh.Add(dir, a)
			raised = append(raised, a)
			if a.Kind == alert.Merged {
				merged[dir] = true
			}
		}
		cache.Put(dir, after)
	}
	for dir := range labels {
		if _, ok := found[dir]; !ok {
			cache.PutAbsent(dir)
		}
	}

	_ = cache.Save()
	if len(raised) > 0 {
		_ = fresh.Save()
	}
	return raised, merged
}
