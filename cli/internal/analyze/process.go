package analyze

import (
	"sync"
	"time"
)

type ProcessMetrics struct {
	WallDuration         time.Duration
	CPUTime              time.Duration
	UserTime             time.Duration
	KernelTime           time.Duration
	PeakJobMemoryBytes   uint64
	PeakProcMemoryBytes  uint64
	TotalProcesses       uint32
	AvailPhysBeforeBytes uint64
	AvailPhysMinBytes    uint64
	AvailPhysAfterBytes  uint64
	JobAssigned          bool
}

type LiveProcessStats struct {
	CPUPercent         float64
	CPUTime            time.Duration
	CurrentMemoryBytes uint64
	PeakMemoryBytes    uint64
	AvailPhysBytes     uint64
}

func sampleAvailMemory(stop <-chan struct{}, initial uint64, minOut *uint64, onSample func(LiveProcessStats)) {
	if initial > 0 && (*minOut == 0 || initial < *minOut) {
		*minOut = initial
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			avail := ProbeSystemMemory().AvailPhysBytes
			if avail > 0 && (*minOut == 0 || avail < *minOut) {
				*minOut = avail
			}
			if onSample != nil {
				onSample(LiveProcessStats{AvailPhysBytes: avail})
			}
		}
	}
}

func startAvailMemorySampler(initial uint64, onSample func(LiveProcessStats)) func(after uint64) uint64 {
	minAvail := initial
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		sampleAvailMemory(stop, initial, &minAvail, onSample)
	}()
	var once sync.Once
	return func(after uint64) uint64 {
		once.Do(func() {
			close(stop)
			<-done
		})
		if after > 0 && (minAvail == 0 || after < minAvail) {
			minAvail = after
		}
		return minAvail
	}
}
