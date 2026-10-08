// Package nav is cursor movement shared by the kanban boards.
package nav

// Step returns the next column from cur in dir (-1 or +1) that has at least
// one card, skipping empty ones. With none that way it returns cur.
func Step(cur, dir, columns int, cards func(col int) int) int {
	for c := cur + dir; c >= 0 && c < columns; c += dir {
		if cards(c) > 0 {
			return c
		}
	}
	return cur
}

// Settle keeps the cursor in range and, when its column is empty, moves it to
// the nearest column that has a card (left first on a tie). With every column
// empty it stays where it was.
func Settle(cur, columns int, cards func(col int) int) int {
	if columns <= 0 {
		return 0
	}
	if cur < 0 {
		cur = 0
	}
	if cur >= columns {
		cur = columns - 1
	}
	if cards(cur) > 0 {
		return cur
	}
	for d := 1; d < columns; d++ {
		if c := cur - d; c >= 0 && cards(c) > 0 {
			return c
		}
		if c := cur + d; c < columns && cards(c) > 0 {
			return c
		}
	}
	return cur
}
