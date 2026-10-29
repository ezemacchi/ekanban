package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/ezemacchi/ekanban/internal/state"
)

// No answer, or anything but yes, is a no: a broadcast is an order to
// several agents at once.
func TestConfirm(t *testing.T) {
	var out strings.Builder
	for in, want := range map[string]bool{"y\n": true, "YES\n": true, "n\n": false, "": false, "sure\n": false} {
		if got := confirm(strings.NewReader(in), &out, "? "); got != want {
			t.Errorf("confirm(%q) = %t", in, got)
		}
	}
}

// Each recipient is sent its own message; one that fails is said and makes
// the command fail, the others still get theirs.
func TestSendAll(t *testing.T) {
	to := []state.Recipient{{Key: "ABC-1", Pane: "w1:p1", Message: "one"}, {Key: "ABC-2", Pane: "w2:p1", Message: "two"}}
	got := map[string]string{}
	submit := func(pane, text string) error {
		if pane == "w1:p1" {
			return errors.New("gone")
		}
		got[pane] = text
		return nil
	}
	var out strings.Builder
	err := sendAll(&out, to, submit)
	if err == nil || !strings.Contains(err.Error(), "ABC-1") {
		t.Fatalf("err %v", err)
	}
	if got["w2:p1"] != "two" || !strings.Contains(out.String(), "sent to ABC-2") || !strings.Contains(out.String(), "could not reach ABC-1") {
		t.Fatalf("sent %v, output %q", got, out.String())
	}
}

func TestBroadcastNeedsAMessage(t *testing.T) {
	var out strings.Builder
	if err := broadcastCmd([]string{"--filter", "pr"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("a broadcast with no message ran")
	}
	if err := broadcastCmd([]string{"--message", "x", "--filter", "nobody"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("an unknown filter was accepted")
	}
}
