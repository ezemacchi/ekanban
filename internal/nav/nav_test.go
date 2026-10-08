package nav

import "testing"

func counts(c ...int) func(int) int { return func(i int) int { return c[i] } }

func TestStepSkipsEmptyAndStopsAtEdge(t *testing.T) {
	cards := counts(1, 0, 0, 2)
	if got := Step(0, 1, 4, cards); got != 3 {
		t.Fatalf("right from 0: %d", got)
	}
	if got := Step(3, 1, 4, cards); got != 3 {
		t.Fatalf("right from last: %d", got)
	}
	if got := Step(3, -1, 4, cards); got != 0 {
		t.Fatalf("left from 3: %d", got)
	}
	if got := Step(0, -1, 4, cards); got != 0 {
		t.Fatalf("left from first: %d", got)
	}
}

func TestSettleFindsNearestNonEmpty(t *testing.T) {
	if got := Settle(0, 4, counts(0, 0, 0, 1)); got != 3 {
		t.Fatalf("got %d", got)
	}
	if got := Settle(2, 4, counts(1, 0, 0, 1)); got != 3 {
		t.Fatalf("got %d", got)
	}
	if got := Settle(1, 4, counts(1, 0, 0, 1)); got != 0 {
		t.Fatalf("got %d", got)
	}
	if got := Settle(9, 4, counts(0, 0, 0, 0)); got != 3 {
		t.Fatalf("all empty clamps: %d", got)
	}
}
