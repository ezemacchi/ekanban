package watch

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/herdrtest"
)

// The board spawns a watcher every time it opens, so without a working lock
// every open would add another poller.
func TestLockIsExclusive(t *testing.T) {
	dir := t.TempDir()

	first, ok := Acquire(dir)
	if !ok {
		t.Fatal("could not take a free lock")
	}
	if _, ok := Acquire(dir); ok {
		t.Fatal("a second watcher took a held lock")
	}

	first.Release()
	second, ok := Acquire(dir)
	if !ok {
		t.Fatal("the lock was not released")
	}
	second.Release()
}

// A machine that lost power leaves a pidfile behind. That must not stop a
// watcher ever starting again.
func TestStalePidfileIsReclaimed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watch.pid")

	// A pid that cannot be running: max_pid is well below this everywhere.
	if err := os.WriteFile(path, []byte("4294967294"), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, ok := Acquire(dir)
	if !ok {
		t.Fatal("a stale pidfile permanently blocked the watcher")
	}
	defer lock.Release()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if pid, _ := strconv.Atoi(string(data)); pid != os.Getpid() {
		t.Fatalf("pidfile holds %s, want this process", data)
	}
}

func TestGarbagePidfileIsReclaimed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "watch.pid"), []byte("not-a-pid"), 0o644); err != nil {
		t.Fatal(err)
	}

	lock, ok := Acquire(dir)
	if !ok {
		t.Fatal("an unreadable pidfile blocked the watcher")
	}
	lock.Release()
}

// Releasing a lock that was never taken must not panic -- spawnWatcher does
// exactly that on the path where it declines to spawn.
func TestReleaseOfNilIsSafe(t *testing.T) {
	var lock *Lock
	lock.Release()
}

// When Herdr goes, the watcher must go too -- and promptly. The interval here
// is an hour, so a watcher that waited for its next tick would fail this.
func TestExitsPromptlyWhenHerdrIsGone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(dir, "nothing-listening.sock"))

	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), dir, time.Hour, nil) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher was still running with no Herdr to watch")
	}
}

// The lock must be released on the way out, or the next board to open would
// decline to start a watcher for ever.
func TestLockIsFreedOnExit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(dir, "nothing-listening.sock"))

	_ = Run(context.Background(), dir, time.Hour, nil)

	lock, ok := Acquire(dir)
	if !ok {
		t.Fatal("the lock outlived the watcher")
	}
	lock.Release()
}

// A cancelled context stops the watcher even while Herdr is fine.
func TestContextCancelStops(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(dir, "nothing-listening.sock"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() { _ = Run(ctx, dir, time.Hour, nil); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled context did not stop the watcher")
	}
}

// Herdr emits events freely, and gh calls are the expensive part, so a burst
// must cost one poll rather than one per event.
func TestSettleBurstCoalescesABurst(t *testing.T) {
	spaces, agents := make(chan struct{}, 8), make(chan struct{}, 8)
	for i := 0; i < 5; i++ {
		agents <- struct{}{}
	}

	start := time.Now()
	if settleBurst(50*time.Millisecond, spaces, agents) {
		t.Fatal("agents alone were taken for a workspace event")
	}
	if len(agents) != 0 {
		t.Fatalf("%d nudges survived the settle", len(agents))
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("settle took %s, far longer than its window", elapsed)
	}
}

// A workspace event inside an agents' burst still makes the poll a full one.
func TestSettleBurstSeesAWorkspaceEvent(t *testing.T) {
	spaces, agents := make(chan struct{}, 8), make(chan struct{}, 8)
	agents <- struct{}{}
	spaces <- struct{}{}
	if !settleBurst(20*time.Millisecond, spaces, agents) {
		t.Fatal("the workspace event was lost in the burst")
	}
}

// Herdr dropping the subscription (events_lost) is not Herdr going away: the
// watcher subscribes again, naming each agent's pane for its status.
func TestSubscribesAgainWhenHerdrDropsTheStream(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	f := herdrtest.Start(t)
	f.Route(map[string]any{
		"agent.list": map[string]any{"agents": []map[string]any{
			{"pane_id": "w1:p2", "agent": "cursor", "agent_status": "working"},
			{"pane_id": "w1:p3", "agent_status": "idle"},
		}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	spaces, _ := subscribe(ctx, cancel)
	herdrtest.WaitFor(t, "the first subscription", func() bool { return f.Called("events.subscribe") == 1 })

	sub := herdrtest.Params(t, f.Last(t, "events.subscribe"))
	var status []string
	for _, s := range sub["subscriptions"].([]any) {
		if e := s.(map[string]any); e["type"] == herdr.StatusChanged {
			status = append(status, e["pane_id"].(string))
		}
	}
	if len(status) != 1 || status[0] != "w1:p2" {
		t.Fatalf("status subscribed for %v, want only the agent's pane w1:p2", status)
	}

	f.Stream <- `{"id":"x","error":{"code":"events_lost","message":"fell behind"}}`
	herdrtest.WaitFor(t, "a second subscription", func() bool { return f.Called("events.subscribe") == 2 })
	select {
	case <-spaces:
	default:
		t.Fatal("no full poll was asked for to catch up")
	}
	if ctx.Err() != nil {
		t.Fatal("the watcher gave up while Herdr was still there")
	}
}
