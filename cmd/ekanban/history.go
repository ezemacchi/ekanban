package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ezemacchi/ekanban/internal/store"
)

// historyCmd is `ekanban history`: write down any worktree that went without
// being accepted, then print the history, oldest first.
//
//	ekanban history                 record what vanished, then list
//	ekanban history --note DIR TEXT add a line explaining something about DIR
//
// It needs no Herdr, because it reads the board file and never changes it.
func historyCmd(args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "--note" {
		if len(args) < 3 || strings.TrimSpace(strings.Join(args[2:], " ")) == "" {
			return errors.New("usage: ekanban history --note <worktree folder> <text>")
		}
		return store.AppendHistory(store.Event{
			At: time.Now().UTC(), Kind: store.KindNote,
			Key: store.Key(args[1]), Detail: strings.Join(args[2:], " "),
		})
	}
	if len(args) > 0 {
		return fmt.Errorf("unknown history option %q", args[0])
	}

	board, err := store.Load()
	if err != nil {
		return err
	}
	fresh, err := board.RecordVanished(time.Now())
	if err != nil {
		return err
	}
	if len(fresh) > 0 {
		fmt.Fprintf(out, "recorded %d worktrees that went without being accepted\n\n", len(fresh))
	}

	events, err := store.ReadHistory()
	if err != nil {
		return err
	}
	if len(events) == 0 {
		fmt.Fprintln(out, "no history yet")
		return nil
	}
	for _, e := range events {
		fmt.Fprintf(out, "%s  %-8s  %s\n", e.At.Local().Format("2006-01-02 15:04"), e.Kind, e.Key)
		for _, line := range eventLines(e) {
			fmt.Fprintf(out, "    %s\n", line)
		}
	}
	return nil
}

func eventLines(e store.Event) []string {
	var lines []string
	if e.Label != "" {
		lines = append(lines, "label: "+e.Label)
	}
	if e.PR != 0 {
		lines = append(lines, fmt.Sprintf("pull request: %d", e.PR))
	}
	if e.Status != "" {
		lines = append(lines, "status: "+e.Status)
	}
	if e.Note != "" {
		lines = append(lines, "board note: "+e.Note)
	}
	if e.Seen != nil {
		lines = append(lines, "board last updated: "+e.Seen.Local().Format("2006-01-02 15:04"))
	}
	if e.Detail != "" {
		lines = append(lines, e.Detail)
	}
	return lines
}
