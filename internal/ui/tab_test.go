package ui

import "testing"

// A board in its own tab stays open when enter sends you to a ticket; a popup
// closes.
func TestTabBoardStaysOpenOnJump(t *testing.T) {
	for _, stays := range []bool{true, false} {
		m := pipelineBoard(t)
		m.stays = stays
		selectSpace(t, m, tmp+"api")
		send(t, m, key("enter"))
		if m.quitting == stays {
			t.Fatalf("stays=%v: quitting=%v after enter", stays, m.quitting)
		}
	}
}
