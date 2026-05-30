package clikit

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// This file holds the live (repainting) elements: a Spinner for indeterminate
// work and a Progress bar for measured work. Both write to stderr so stdout
// stays clean for piped data, and both animate only on an interactive color
// TTY — off it, every method is a silent no-op, so scripts and JSON consumers
// never see a stray carriage return. No third-party TUI dependency: the
// animation is a goroutine plus carriage-return repaints.

const (
	clearLine     = "\r\x1b[K" // carriage return + erase to end of line
	spinnerPeriod = 90 * time.Millisecond
)

var (
	spinnerUnicode = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerASCII   = []string{"|", "/", "-", "\\"}
)

// Spinner is an indeterminate progress indicator. Create it with
// Context.NewSpinner, Start it, optionally Update the message, and finish with
// Success, Fail, or Stop.
type Spinner struct {
	w       io.Writer
	th      theme
	gl      Glyph
	frames  []string
	animate bool

	mu     sync.Mutex
	msg    string
	stopc  chan struct{}
	donec  chan struct{}
	active bool
}

// NewSpinner returns a Spinner bound to this Context's stderr, theme, and
// glyph set. It animates only on an interactive color TTY.
func (c *Context) NewSpinner(msg string) *Spinner {
	frames := spinnerUnicode
	if !c.unicode {
		frames = spinnerASCII
	}
	return &Spinner{w: c.Stderr, th: c.th, gl: c.gl, frames: frames, animate: c.Color, msg: msg}
}

// Start begins the animation. It is a no-op (and safe) off an interactive TTY.
func (s *Spinner) Start() {
	if !s.animate || s.active {
		return
	}
	s.active = true
	s.stopc = make(chan struct{})
	s.donec = make(chan struct{})
	go s.spin()
}

func (s *Spinner) spin() {
	defer close(s.donec)
	t := time.NewTicker(spinnerPeriod)
	defer t.Stop()
	for i := 0; ; i++ {
		select {
		case <-s.stopc:
			return
		case <-t.C:
			s.mu.Lock()
			frame := s.th.accent.s.Render(s.frames[i%len(s.frames)])
			fmt.Fprintf(s.w, "%s%s %s", clearLine, frame, s.msg)
			s.mu.Unlock()
		}
	}
}

// Update changes the message shown next to the spinner.
func (s *Spinner) Update(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.mu.Unlock()
}

// Success stops the spinner and replaces it with a "✓ msg" line.
func (s *Spinner) Success(msg string) { s.finish(s.th.ok.render(s.animate, s.gl.OK), msg) }

// Fail stops the spinner and replaces it with a "✗ msg" line.
func (s *Spinner) Fail(msg string) { s.finish(s.th.err.render(s.animate, s.gl.Err), msg) }

// Stop clears the spinner without printing a final line.
func (s *Spinner) Stop() { s.finish("", "") }

func (s *Spinner) finish(glyph, msg string) {
	if !s.animate {
		return
	}
	if s.active {
		close(s.stopc)
		<-s.donec
		s.active = false
	}
	fmt.Fprint(s.w, clearLine)
	switch {
	case msg != "" && glyph != "":
		fmt.Fprintf(s.w, "%s %s\n", glyph, msg)
	case msg != "":
		fmt.Fprintln(s.w, msg)
	}
}

// Progress is a determinate progress bar (filled/empty cells + percentage).
// Create it with Context.NewProgress, advance it with Set, and finish with
// Done. Like Spinner, it animates only on an interactive color TTY.
type Progress struct {
	w       io.Writer
	th      theme
	gl      Glyph
	total   int
	width   int
	full    string
	empty   string
	animate bool
}

// NewProgress returns a Progress bar over total steps, bound to this Context's
// stderr. total <= 0 is treated as 1 to avoid a divide-by-zero.
func (c *Context) NewProgress(total int) *Progress {
	if total <= 0 {
		total = 1
	}
	full, empty := "█", "░"
	if !c.unicode {
		full, empty = "#", "-"
	}
	return &Progress{w: c.Stderr, th: c.th, gl: c.gl, total: total, width: 28, full: full, empty: empty, animate: c.Color}
}

// Set repaints the bar at step n (clamped to [0,total]) with a trailing label.
func (p *Progress) Set(n int, label string) {
	if !p.animate {
		return
	}
	if n < 0 {
		n = 0
	}
	if n > p.total {
		n = p.total
	}
	frac := float64(n) / float64(p.total)
	filled := int(frac * float64(p.width))
	bar := p.th.accent.s.Render(strings.Repeat(p.full, filled)) +
		p.th.faint.s.Render(strings.Repeat(p.empty, p.width-filled))
	fmt.Fprintf(p.w, "%s%s %3.0f%%  %s", clearLine, bar, frac*100, label)
}

// Done clears the bar and prints a final "✓ msg" line (or nothing if msg is
// empty).
func (p *Progress) Done(msg string) {
	if !p.animate {
		return
	}
	fmt.Fprint(p.w, clearLine)
	if msg != "" {
		fmt.Fprintf(p.w, "%s %s\n", p.th.ok.s.Render(p.gl.OK), msg)
	}
}
