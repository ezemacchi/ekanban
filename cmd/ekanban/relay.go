package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/herdr"
	"github.com/ezemacchi/ekanban/internal/relay"
)

// `ekanban relay` points the watcher's notices at an agent, so an agent
// working for the user hears what changed on the board without being asked.

const relayUsage = `usage:
  ekanban relay [--pane <id>]   send the watcher's notices to the agent in this pane, or in <id>
  ekanban relay --show          say where notices go and what is waiting
  ekanban relay --off           stop sending them

Each notice the watcher raises (an orchestrator finished or is waiting at a
prompt, a question for you, a pull request merged...) is also sent to that
agent, as one message starting with [ekanban], once it is idle or done. An
agent that is working, or asking you something, is not interrupted. The relay
ends by itself when its pane closes.`

func relayCmd(args []string, out io.Writer) error {
	pane, show, off := "", false, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(out, relayUsage)
			return nil
		case a == "--show":
			show = true
		case a == "--off":
			off = true
		case a == "--pane":
			if i+1 >= len(args) {
				return errors.New(relayUsage)
			}
			i++
			pane = args[i]
		case strings.HasPrefix(a, "--pane="):
			pane = strings.TrimPrefix(a, "--pane=")
		default:
			return fmt.Errorf("unknown argument %s\n%s", a, relayUsage)
		}
	}
	if show && off {
		return errors.New(relayUsage)
	}
	dir, err := stateDir()
	if err != nil {
		return err
	}
	r := relay.Load(dir)

	switch {
	case show:
		if !r.On() {
			fmt.Fprintln(out, "notices go to nobody")
			return nil
		}
		fmt.Fprintf(out, "notices go to pane %s since %s, %d waiting\n", r.Pane, r.Since.Local().Format(time.DateTime), len(r.Pending))
		for _, n := range r.Pending {
			fmt.Fprintln(out, "  "+n.Text)
		}
		return nil
	case off:
		was := r.Pane
		r.Off()
		if err := r.Replace(); err != nil {
			return err
		}
		if was == "" {
			fmt.Fprintln(out, "notices already went to nobody")
		} else {
			fmt.Fprintf(out, "notices no longer go to pane %s\n", was)
		}
		return nil
	}

	if pane == "" {
		pane = os.Getenv("HERDR_PANE_ID")
	}
	if pane == "" {
		return errors.New("no pane: run this from the agent's own pane, or name one with --pane")
	}
	client, err := herdr.New()
	if err != nil {
		return fmt.Errorf("a relay needs Herdr: %w", err)
	}
	agents, err := client.Agents()
	if err != nil {
		return err
	}
	kind := ""
	for _, a := range agents {
		if a.PaneID == pane && a.Agent != nil {
			kind = *a.Agent
		}
	}
	if kind == "" {
		return fmt.Errorf("pane %s is running no agent Herdr recognises", pane)
	}
	before := r.Pane
	r.To(pane, time.Now())
	if err := r.Replace(); err != nil {
		return err
	}
	spawnWatcher()
	if before != "" && before != pane {
		fmt.Fprintf(out, "notices go to pane %s (%s) from now on, no longer to %s\n", pane, kind, before)
	} else {
		fmt.Fprintf(out, "notices go to pane %s (%s) from now on\n", pane, kind)
	}
	return nil
}
