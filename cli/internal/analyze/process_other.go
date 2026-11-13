//go:build !windows

package analyze

import (
	"context"
	"os/exec"
	"time"
)

func runObservedCommand(ctx context.Context, cmd *exec.Cmd, onSample func(LiveProcessStats)) (ProcessMetrics, error) {
	_ = ctx
	cmd.WaitDelay = 10 * time.Second

	before := ProbeSystemMemory().AvailPhysBytes
	stopSampler := startAvailMemorySampler(before, onSample)
	start := time.Now()
	runErr := cmd.Run()
	after := ProbeSystemMemory().AvailPhysBytes
	minAvail := stopSampler(after)

	var userTime, kernelTime time.Duration
	if cmd.ProcessState != nil {
		userTime = cmd.ProcessState.UserTime()
		kernelTime = cmd.ProcessState.SystemTime()
	}

	return ProcessMetrics{
		WallDuration:         time.Since(start),
		CPUTime:              userTime + kernelTime,
		UserTime:             userTime,
		KernelTime:           kernelTime,
		AvailPhysBeforeBytes: before,
		AvailPhysMinBytes:    minAvail,
		AvailPhysAfterBytes:  after,
	}, runErr
}
