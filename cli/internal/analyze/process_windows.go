//go:build windows

package analyze

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const jobObjectBasicProcessIdList = 3

type jobBasicProcessIDList struct {
	NumberOfAssignedProcesses uint32
	NumberOfProcessIdsInList  uint32
	ProcessIdList             [128]uintptr
}

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

var procGetProcessMemoryInfo = modKernel32.NewProc("K32GetProcessMemoryInfo")

type jobBasicAccountingInfo struct {
	TotalUserTime             int64 // 100ns FILETIME ticks
	TotalKernelTime           int64 // 100ns FILETIME ticks
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

type jobController struct {
	mu                 sync.Mutex
	job                windows.Handle
	closed             bool
	assigned           bool
	terminateRequested bool
}

func newJobController() *jobController {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil || job == 0 {
		return &jobController{}
	}
	var extInfo windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	extInfo.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&extInfo)),
		uint32(unsafe.Sizeof(extInfo)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return &jobController{}
	}
	return &jobController{job: job}
}

func (c *jobController) assign(pid int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.job == 0 || pid <= 0 {
		return false
	}
	procHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE,
		false,
		uint32(pid),
	)
	if err != nil {
		return false
	}
	assignErr := windows.AssignProcessToJobObject(c.job, procHandle)
	_ = windows.CloseHandle(procHandle)
	if assignErr != nil {
		return false
	}
	c.assigned = true
	if c.terminateRequested {
		_ = windows.TerminateJobObject(c.job, 1)
	}
	return true
}

func (c *jobController) terminate() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.terminateRequested = true
	if c.closed || c.job == 0 || !c.assigned {
		return false, nil
	}
	return true, windows.TerminateJobObject(c.job, 1)
}

func (c *jobController) closeOnly() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed && c.job != 0 {
		_ = windows.CloseHandle(c.job)
		c.closed = true
	}
}

func (c *jobController) sample() (cpuTime time.Duration, currentMem, peakMem uint64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.job == 0 || !c.assigned {
		return 0, 0, 0, false
	}

	var acct jobBasicAccountingInfo
	if err := windows.QueryInformationJobObject(
		c.job,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&acct)),
		uint32(unsafe.Sizeof(acct)),
		nil,
	); err == nil {
		cpuTime = time.Duration(acct.TotalUserTime+acct.TotalKernelTime) * 100 * time.Nanosecond
	}

	var ext windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(
		c.job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&ext)),
		uint32(unsafe.Sizeof(ext)),
		nil,
	); err == nil {
		peakMem = uint64(ext.PeakJobMemoryUsed)
		if peakMem == 0 {
			peakMem = uint64(ext.PeakProcessMemoryUsed)
		}
	}

	var pidList jobBasicProcessIDList
	err := windows.QueryInformationJobObject(
		c.job,
		jobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&pidList)),
		uint32(unsafe.Sizeof(pidList)),
		nil,
	)
	if err == nil || errors.Is(err, windows.ERROR_MORE_DATA) {
		n := min(int(pidList.NumberOfProcessIdsInList), len(pidList.ProcessIdList))
		for _, pid := range pidList.ProcessIdList[:n] {
			if pid == 0 {
				continue
			}
			h, oErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
			if oErr != nil {
				continue
			}
			var pmc processMemoryCounters
			pmc.CB = uint32(unsafe.Sizeof(pmc))
			r1, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.CB))
			_ = windows.CloseHandle(h)
			if r1 != 0 {
				currentMem += uint64(pmc.WorkingSetSize)
			}
		}
	}
	if currentMem > peakMem {
		peakMem = currentMem
	}
	return cpuTime, currentMem, peakMem, true
}

func (c *jobController) queryAndClose() (userTime, kernelTime time.Duration, peakJob, peakProc uint64, totalProcs uint32, assigned bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	assigned = c.assigned
	if c.closed || c.job == 0 {
		return 0, 0, 0, 0, 0, assigned
	}
	if c.assigned {
		var ext windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		if err := windows.QueryInformationJobObject(
			c.job,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&ext)),
			uint32(unsafe.Sizeof(ext)),
			nil,
		); err == nil {
			peakJob = uint64(ext.PeakJobMemoryUsed)
			peakProc = uint64(ext.PeakProcessMemoryUsed)
		}

		var acct jobBasicAccountingInfo
		if err := windows.QueryInformationJobObject(
			c.job,
			windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&acct)),
			uint32(unsafe.Sizeof(acct)),
			nil,
		); err == nil {
			userTime = time.Duration(acct.TotalUserTime) * 100 * time.Nanosecond
			kernelTime = time.Duration(acct.TotalKernelTime) * 100 * time.Nanosecond
			totalProcs = acct.TotalProcesses
		}
	}
	_ = windows.CloseHandle(c.job)
	c.closed = true
	return userTime, kernelTime, peakJob, peakProc, totalProcs, assigned
}

func runObservedCommand(ctx context.Context, cmd *exec.Cmd, onSample func(LiveProcessStats)) (ProcessMetrics, error) {
	_ = ctx
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	} else {
		attr := *cmd.SysProcAttr
		cmd.SysProcAttr = &attr
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
	cmd.WaitDelay = 10 * time.Second

	jobCtrl := newJobController()
	defer jobCtrl.closeOnly()

	cmd.Cancel = func() error {
		if ok, err := jobCtrl.terminate(); ok && err == nil {
			return nil
		}
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}

	before := ProbeSystemMemory().AvailPhysBytes
	start := time.Now()
	if err := cmd.Start(); err != nil {
		jobCtrl.closeOnly()
		return ProcessMetrics{
			AvailPhysBeforeBytes: before,
			AvailPhysMinBytes:    before,
			AvailPhysAfterBytes:  before,
		}, err
	}

	if cmd.Process != nil {
		jobCtrl.assign(cmd.Process.Pid)
	}

	minAvail := before
	sampleStop := make(chan struct{})
	sampleDone := make(chan struct{})
	go func() {
		defer close(sampleDone)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		prevTime := start
		prevCPU := time.Duration(0)
		for {
			select {
			case <-sampleStop:
				return
			case now := <-ticker.C:
				avail := ProbeSystemMemory().AvailPhysBytes
				if avail > 0 && (minAvail == 0 || avail < minAvail) {
					minAvail = avail
				}
				cpuTime, currentMem, peakMem, _ := jobCtrl.sample()
				wallDelta := now.Sub(prevTime)
				cpuDelta := max(time.Duration(0), cpuTime-prevCPU)
				cpuPct := 0.0
				if wallDelta > 0 {
					cpuPct = float64(cpuDelta) / float64(wallDelta) * 100.0
				}
				prevTime = now
				prevCPU = cpuTime
				if onSample != nil {
					onSample(LiveProcessStats{
						CPUPercent:         cpuPct,
						CPUTime:            cpuTime,
						CurrentMemoryBytes: currentMem,
						PeakMemoryBytes:    peakMem,
						AvailPhysBytes:     avail,
					})
				}
			}
		}
	}()

	waitErr := cmd.Wait()
	close(sampleStop)
	<-sampleDone

	after := ProbeSystemMemory().AvailPhysBytes
	if after > 0 && (minAvail == 0 || after < minAvail) {
		minAvail = after
	}

	userTime, kernelTime, peakJob, peakProc, totalProcs, jobAssigned := jobCtrl.queryAndClose()
	if !jobAssigned && cmd.ProcessState != nil {
		userTime = cmd.ProcessState.UserTime()
		kernelTime = cmd.ProcessState.SystemTime()
	}

	return ProcessMetrics{
		WallDuration:         time.Since(start),
		CPUTime:              userTime + kernelTime,
		UserTime:             userTime,
		KernelTime:           kernelTime,
		PeakJobMemoryBytes:   peakJob,
		PeakProcMemoryBytes:  peakProc,
		TotalProcesses:       totalProcs,
		AvailPhysBeforeBytes: before,
		AvailPhysMinBytes:    minAvail,
		AvailPhysAfterBytes:  after,
		JobAssigned:          jobAssigned,
	}, waitErr
}
