package screen

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ezemacchi/ekanban/internal/look"
)

// A message can name a key ("press n to add one"). Key marks it where the
// message is built, which is where the current binding is known; Say draws it
// in the key colour where the message is shown, so a key reads the same in a
// status line as in the footer's buttons.
const (
	keyOpen  = "\x01"
	keyClose = "\x02"
)

// Key marks k as a key inside a message; "" stays "" (an unbound action).
func Key(k string) string {
	if k == "" {
		return ""
	}
	return keyOpen + k + keyClose
}

// Plain is s without its key marks.
func Plain(s string) string {
	return strings.NewReplacer(keyOpen, "", keyClose, "").Replace(s)
}

// Say draws s in base with its marked keys in the key colour, cut to width
// cells; width <= 0 does not cut.
func Say(s string, base lipgloss.Style, width int) string {
	var b strings.Builder
	used := 0
	for i, part := range strings.Split(s, keyOpen) {
		key, text := "", part
		if i > 0 {
			if k, rest, ok := strings.Cut(part, keyClose); ok {
				key, text = k, rest
			}
		}
		for _, seg := range []struct {
			text  string
			style lipgloss.Style
		}{{key, look.Key}, {text, base}} {
			if seg.text == "" {
				continue
			}
			w := lipgloss.Width(seg.text)
			if width > 0 && used+w > width {
				b.WriteString(seg.style.Render(look.Truncate(seg.text, width-used)))
				return b.String()
			}
			b.WriteString(seg.style.Render(seg.text))
			used += w
		}
	}
	return b.String()
}
