//go:build !windows

package analyze

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func runObservedCommand(cmd *exec.Cmd, onSample func(LiveProcessStats)) (ProcessMetrics, error) {
	var attr syscall.SysProcAttr
	if cmd.SysProcAttr != nil {
		attr = *cmd.SysProcAttr
	}
	attr.Setpgid = true
	attr.Pgid = 0
	cmd.SysProcAttr = &attr
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil || cmd.Process.Pid <= 0 {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return cmd.Process.Kill()
		}
		return nil
	}

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
