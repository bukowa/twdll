package term

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var asciiSpinnerFrames = [4]string{"|", "/", "-", "\\"}

const maxSpinnerTextRunes = 72

type Spinner struct {
	ui       *UI
	noop     bool
	mu       sync.Mutex
	text     string
	start    time.Time
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func (u *UI) StartSpinner(text string) *Spinner {
	if u == nil || !u.CanAnimate() {
		return &Spinner{
			ui:   u,
			noop: true,
			text: text,
		}
	}
	s := &Spinner{
		ui:     u,
		text:   text,
		start:  time.Now(),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	s.render(0)
	go s.loop()
	return s
}

func (s *Spinner) SetText(text string) {
	if s == nil || s.noop {
		return
	}
	s.mu.Lock()
	s.text = text
	s.mu.Unlock()
}

func (s *Spinner) Stop() {
	if s == nil || s.noop {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		<-s.doneCh
		s.ui.ClearProgress()
	})
}

func (s *Spinner) loop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frameIdx := 1
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.render(frameIdx)
			frameIdx = (frameIdx + 1) % len(asciiSpinnerFrames)
		}
	}
}

func (s *Spinner) render(frameIdx int) {
	s.mu.Lock()
	rawText := s.text
	elapsed := time.Since(s.start)
	s.mu.Unlock()

	truncatedText := truncatePlainRunes(rawText, maxSpinnerTextRunes)
	frame := s.ui.ErrStyle().Cyan(asciiSpinnerFrames[frameIdx%len(asciiSpinnerFrames)])
	line := "  [" + frame + "] " + truncatedText + " (" + formatElapsed(elapsed) + ")"
	s.ui.Progress(line)
}

func truncatePlainRunes(text string, maxRunes int) string {
	clean := strings.Join(strings.Fields(text), " ")
	if maxRunes <= 3 || utf8.RuneCountInString(clean) <= maxRunes {
		return clean
	}
	runes := []rune(clean)
	return string(runes[:maxRunes-3]) + "..."
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d.Round(time.Second).Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	remSecs := secs % 60
	if mins < 60 {
		return fmt.Sprintf("%dm%02ds", mins, remSecs)
	}
	hours := mins / 60
	remMins := mins % 60
	return fmt.Sprintf("%dh%02dm%02ds", hours, remMins, remSecs)
}
