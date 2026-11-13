package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type targetRunResult struct {
	Metrics      ProcessMetrics
	GhidraMaxMem string
	Started      bool
}

type ResourceRecord struct {
	Timestamp            string  `json:"timestamp"`
	Engine               string  `json:"engine"`
	Target               string  `json:"target"`
	Status               string  `json:"status"`
	Error                string  `json:"error,omitempty"`
	ConcurrentJobs       int     `json:"concurrent_jobs"`
	GhidraMaxMem         string  `json:"ghidra_maxmem,omitempty"`
	WallSeconds          float64 `json:"wall_seconds"`
	CPUSeconds           float64 `json:"cpu_seconds"`
	UserSeconds          float64 `json:"user_seconds"`
	KernelSeconds        float64 `json:"kernel_seconds"`
	PeakJobMemoryBytes   uint64  `json:"peak_job_memory_bytes"`
	PeakProcMemoryBytes  uint64  `json:"peak_proc_memory_bytes"`
	TotalProcesses       uint32  `json:"total_processes,omitempty"`
	JobAssigned          bool    `json:"job_assigned"`
	TotalPhysBytes       uint64  `json:"total_phys_bytes,omitempty"`
	AvailPhysBeforeBytes uint64  `json:"avail_phys_before_bytes,omitempty"`
	AvailPhysMinBytes    uint64  `json:"avail_phys_min_bytes,omitempty"`
	AvailPhysAfterBytes  uint64  `json:"avail_phys_after_bytes,omitempty"`
}

func appendResourceRecord(path string, rec ResourceRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure resource log dir: %w", err)
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal resource record: %w", err)
	}
	line = append(line, '\n')
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open resource log %s: %w", path, err)
	}
	_, writeErr := f.Write(line)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("write resource log %s: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close resource log %s: %w", path, closeErr)
	}
	return nil
}

func formatResourceSummary(m ProcessMetrics, ghidraMaxMem string) string {
	var parts []string

	peak := m.PeakJobMemoryBytes
	if peak == 0 {
		peak = m.PeakProcMemoryBytes
	}
	if peak > 0 {
		parts = append(parts, "peak "+formatMemoryBytes(peak))
	}

	if m.CPUTime > 0 {
		rounded := m.CPUTime.Round(100 * time.Millisecond)
		if rounded <= 0 {
			rounded = m.CPUTime.Round(time.Millisecond)
		}
		if rounded > 0 {
			parts = append(parts, "cpu "+rounded.String())
		}
	}

	if m.AvailPhysMinBytes > 0 {
		parts = append(parts, "min-avail "+formatMemoryBytes(m.AvailPhysMinBytes))
	}

	if trimmed := strings.TrimSpace(ghidraMaxMem); trimmed != "" {
		parts = append(parts, "Xmx="+trimmed)
	}

	return strings.Join(parts, ", ")
}

func formatMemoryBytes(b uint64) string {
	const (
		kiB = uint64(1024)
		miB = uint64(1024 * 1024)
	)
	switch {
	case b >= giB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(giB))
	case b >= miB:
		return fmt.Sprintf("%d MB", (b+miB/2)/miB)
	case b >= kiB:
		return fmt.Sprintf("%d KB", (b+kiB/2)/kiB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
