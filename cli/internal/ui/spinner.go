package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SlotState indicates current lifecycle state of a task slot.
type SlotState int

const (
	// SlotQueued indicates the task is waiting for a concurrency slot.
	SlotQueued SlotState = iota
	// SlotRunning indicates the task is actively executing.
	SlotRunning
	// SlotFinished indicates the task has completed or failed.
	SlotFinished
)

// TaskSlot represents an active analysis task displayed in the spinner.
type TaskSlot struct {
	mu         sync.Mutex
	Index      int
	Total      int
	Label      string
	LogPath    string
	StartTime  time.Time
	State      SlotState
	FinishMsg  string
	Tracker    *ProcessTracker
	LastLine   string
	MetricsStr string
}

// Start transitions the slot to running state with timestamp.
func (s *TaskSlot) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StartTime = time.Now()
	s.State = SlotRunning
}

// SetPID begins tracking CPU and RAM for the given process ID.
func (s *TaskSlot) SetPID(pid int) {
	if s == nil || pid <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tracker = NewProcessTracker(pid)
}

// Finish marks the task complete with a final message.
func (s *TaskSlot) Finish(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = SlotFinished
	s.FinishMsg = msg
}

// MultiSpinner manages real-time terminal progress spinners with CPU, RAM, and log tailing.
type MultiSpinner struct {
	printer  *Printer
	slots    []*TaskSlot
	mu       sync.Mutex
	stopCh   chan struct{}
	doneCh   chan struct{}
	frameIdx int
	started  bool
	stopped  bool
}

// NewMultiSpinner creates a spinner manager.
func NewMultiSpinner(printer *Printer) *MultiSpinner {
	return &MultiSpinner{
		printer: printer,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// AddSlot registers a new target slot to be animated.
func (m *MultiSpinner) AddSlot(index, total int, label, logPath string) *TaskSlot {
	m.mu.Lock()
	defer m.mu.Unlock()

	slot := &TaskSlot{
		Index:   index,
		Total:   total,
		Label:   label,
		LogPath: logPath,
		State:   SlotQueued,
	}
	m.slots = append(m.slots, slot)
	return slot
}

// Slot returns the slot at idx.
func (m *MultiSpinner) Slot(idx int) *TaskSlot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx >= 0 && idx < len(m.slots) {
		return m.slots[idx]
	}
	return nil
}

// Start begins animating all registered slots.
func (m *MultiSpinner) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	if len(m.slots) > 1 && m.printer != nil {
		// Allocate blank lines for the multi-line block
		for range m.slots {
			fmt.Fprintln(m.printer.Out)
		}
	}

	go m.run()
}

// Stop terminates animation and renders final static output for all slots.
func (m *MultiSpinner) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	m.mu.Unlock()

	close(m.stopCh)
	<-m.doneCh

	m.render(true)
}

func (m *MultiSpinner) run() {
	defer close(m.doneCh)

	if m.printer == nil || len(m.slots) == 0 {
		<-m.stopCh
		return
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

func (m *MultiSpinner) tick() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Sample metrics and tail logs for running slots
	for _, s := range m.slots {
		s.mu.Lock()
		if s.State == SlotRunning {
			if s.Tracker != nil {
				metrics := s.Tracker.Sample()
				s.MetricsStr = metrics.Format()
			}
			s.LastLine = readLastLogLine(s.LogPath)
		}
		s.mu.Unlock()
	}

	m.render(false)
}

func (m *MultiSpinner) render(final bool) {
	n := len(m.slots)
	if n == 0 || m.printer == nil {
		return
	}

	frame := spinnerFrames[m.frameIdx%len(spinnerFrames)]
	m.frameIdx++

	if n == 1 {
		s := m.slots[0]
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.State == SlotFinished {
			fmt.Fprintf(m.printer.Out, "\r\033[2K%s\n", s.FinishMsg)
		} else if s.State == SlotQueued {
			fmt.Fprintf(m.printer.Out, "\r\033[2K  %s %s %s", m.printer.Dim("[-]"), m.printer.Bold(s.Label), m.printer.Dim("[queued]"))
		} else {
			elapsed := time.Since(s.StartTime).Round(time.Second)
			mins := int(elapsed.Minutes())
			secs := int(elapsed.Seconds()) % 60
			timeStr := fmt.Sprintf("%02dm %02ds", mins, secs)

			metrics := ""
			if s.MetricsStr != "" {
				metrics = " | " + s.MetricsStr
			}

			tag := fmt.Sprintf("[%s%s]", timeStr, metrics)
			cleanLog := truncateLine(s.LastLine, 60)
			if cleanLog != "" {
				fmt.Fprintf(m.printer.Out, "\r\033[2K  %s %s %s %s", m.printer.Cyan(frame), m.printer.Bold(s.Label), m.printer.Dim(tag), m.printer.Dim(cleanLog))
			} else {
				fmt.Fprintf(m.printer.Out, "\r\033[2K  %s %s %s", m.printer.Cyan(frame), m.printer.Bold(s.Label), m.printer.Dim(tag))
			}
		}
		return
	}

	// Move cursor UP by n lines to rewrite the multi-line slot block in place
	fmt.Fprintf(m.printer.Out, "\033[%dA\r", n)

	for _, s := range m.slots {
		s.mu.Lock()
		if s.State == SlotFinished {
			fmt.Fprintf(m.printer.Out, "\033[2K%s\n", s.FinishMsg)
		} else if s.State == SlotQueued {
			fmt.Fprintf(m.printer.Out, "\033[2K  [%d/%d] %s %-10s %s\n", s.Index, s.Total, m.printer.Dim("[-]"), m.printer.Bold(s.Label), m.printer.Dim("[queued]"))
		} else {
			elapsed := time.Since(s.StartTime).Round(time.Second)
			mins := int(elapsed.Minutes())
			secs := int(elapsed.Seconds()) % 60
			timeStr := fmt.Sprintf("%02dm %02ds", mins, secs)

			metrics := ""
			if s.MetricsStr != "" {
				metrics = " | " + s.MetricsStr
			}

			tag := fmt.Sprintf("[%s%s]", timeStr, metrics)
			cleanLog := truncateLine(s.LastLine, 45)
			if cleanLog != "" {
				fmt.Fprintf(m.printer.Out, "\033[2K  [%d/%d] %s %-10s %s %s\n", s.Index, s.Total, m.printer.Cyan(frame), m.printer.Bold(s.Label), m.printer.Dim(tag), m.printer.Dim(cleanLog))
			} else {
				fmt.Fprintf(m.printer.Out, "\033[2K  [%d/%d] %s %-10s %s\n", s.Index, s.Total, m.printer.Cyan(frame), m.printer.Bold(s.Label), m.printer.Dim(tag))
			}
		}
		s.mu.Unlock()
	}
}

func truncateLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		if maxLen > 3 {
			return s[:maxLen-3] + "..."
		}
		return s[:maxLen]
	}
	return s
}

// readLastLogLine reads the final non-empty line of path by seeking near the EOF.
func readLastLogLine(path string) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.Size() == 0 {
		return ""
	}

	readSize := int64(2048)
	if stat.Size() < readSize {
		readSize = stat.Size()
	}

	buf := make([]byte, readSize)
	_, err = f.ReadAt(buf, stat.Size()-readSize)
	if err != nil && err != io.EOF {
		return ""
	}

	lines := strings.Split(string(buf), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		clean := strings.TrimRight(lines[i], "\r\n ")
		clean = strings.TrimSpace(clean)
		if clean != "" {
			return clean
		}
	}
	return ""
}
