package look

// The terminal spinner both boards show while something loads. It ticks only
// while asked to: a board at rest does not redraw.

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const every = 90 * time.Millisecond

// SpinMsg advances a Spinner. id keeps two spinners in one program apart.
type SpinMsg struct{ id int }

// Spinner is one spinner's frame and whether its clock is running.
type Spinner struct {
	id      int
	frame   int
	running bool
}

var nextID int

// NewSpinner returns a stopped spinner.
func NewSpinner() Spinner {
	nextID++
	return Spinner{id: nextID}
}

// Frame is the glyph to draw now.
func (s *Spinner) Frame() string { return frames[s.frame%len(frames)] }

// Start begins ticking, unless it already is. Call it whenever loading
// begins; it is safe to call repeatedly.
func (s *Spinner) Start() tea.Cmd {
	if s.running {
		return nil
	}
	s.running = true
	return s.tick()
}

// Update advances on its own SpinMsg while busy, and stops otherwise.
func (s *Spinner) Update(msg SpinMsg, busy bool) tea.Cmd {
	if msg.id != s.id {
		return nil
	}
	if !busy {
		s.running = false
		return nil
	}
	s.frame++
	return s.tick()
}

func (s *Spinner) tick() tea.Cmd {
	id := s.id
	return tea.Tick(every, func(time.Time) tea.Msg { return SpinMsg{id: id} })
}
