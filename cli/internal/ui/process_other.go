//go:build !windows

package ui

// ProcessMetrics stores sampled CPU and memory metrics.
type ProcessMetrics struct {
	CPUPercent float64
	RAMBytes   uint64
}

func (m ProcessMetrics) Format() string {
	return ""
}

// ProcessTracker is a stub on non-Windows platforms.
type ProcessTracker struct{}

func NewProcessTracker(pid int) *ProcessTracker {
	return &ProcessTracker{}
}

func (pt *ProcessTracker) Sample() ProcessMetrics {
	return ProcessMetrics{}
}
