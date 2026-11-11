//go:build windows

package ui

import (
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modpsapi    = syscall.NewLazyDLL("psapi.dll")

	procOpenProcess              = modkernel32.NewProc("OpenProcess")
	procCloseHandle              = modkernel32.NewProc("CloseHandle")
	procGetProcessTimes          = modkernel32.NewProc("GetProcessTimes")
	procCreateToolhelp32Snapshot = modkernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = modkernel32.NewProc("Process32FirstW")
	procProcess32NextW           = modkernel32.NewProc("Process32NextW")
	procGetProcessMemoryInfo     = modpsapi.NewProc("GetProcessMemoryInfo")
)

const (
	processQueryInformation = 0x0400
	processVMRead           = 0x0010
	th32csSnapProcess       = 0x00000002
)

type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

type processEntry32W struct {
	dwSize              uint32
	cntUsage            uint32
	th32ProcessID       uint32
	th32DefaultHeapID   uintptr
	th32ModuleID        uint32
	cntThreads          uint32
	th32ParentProcessID uint32
	pcPriClassBase      int32
	dwFlags             uint32
	szExeFile           [260]uint16
}

// ProcessMetrics stores sampled CPU and memory metrics for a process tree.
type ProcessMetrics struct {
	CPUPercent float64
	RAMBytes   uint64
}

// Format returns human-readable representation: e.g. "CPU: 42% | RAM: 640 MB"
func (m ProcessMetrics) Format() string {
	if m.RAMBytes == 0 && m.CPUPercent == 0 {
		return ""
	}
	ramStr := ""
	if m.RAMBytes >= 1024*1024*1024 {
		ramStr = fmt.Sprintf("%.1f GB", float64(m.RAMBytes)/(1024*1024*1024))
	} else {
		ramStr = fmt.Sprintf("%d MB", m.RAMBytes/(1024*1024))
	}
	return fmt.Sprintf("CPU: %2.0f%% | RAM: %s", m.CPUPercent, ramStr)
}

// ProcessTracker samples CPU and RAM for a process and any of its children.
type ProcessTracker struct {
	mu             sync.Mutex
	pid            int
	lastWallTime   time.Time
	lastCPUTime    uint64
	lastMetrics    ProcessMetrics
	lastSampleTime time.Time
}

// NewProcessTracker creates a new tracker for root pid.
func NewProcessTracker(pid int) *ProcessTracker {
	return &ProcessTracker{
		pid: pid,
	}
}

// Sample measures process CPU percentage and working set memory.
func (pt *ProcessTracker) Sample() ProcessMetrics {
	if pt == nil || pt.pid <= 0 {
		return ProcessMetrics{}
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	// Return cached metrics if within 400ms to avoid sampling overhead
	if !pt.lastSampleTime.IsZero() && now.Sub(pt.lastSampleTime) < 400*time.Millisecond && pt.lastMetrics.RAMBytes > 0 {
		return pt.lastMetrics
	}

	pids := pt.getProcessTreePids()

	var totalRAM uint64
	var totalCPUTime uint64

	for _, pid := range pids {
		ram, cpu := getSingleProcessMetrics(pid)
		totalRAM += ram
		totalCPUTime += cpu
	}

	if !pt.lastWallTime.IsZero() && totalCPUTime >= pt.lastCPUTime {
		wallDelta := now.Sub(pt.lastWallTime).Seconds()
		if wallDelta >= 0.3 {
			// FILETIME is in 100-nanosecond intervals (10,000,000 per second)
			cpuDeltaSec := float64(totalCPUTime-pt.lastCPUTime) / 10000000.0
			cpuPercent := (cpuDeltaSec / wallDelta) * 100.0
			pt.lastWallTime = now
			pt.lastCPUTime = totalCPUTime
			pt.lastMetrics.CPUPercent = cpuPercent
		}
	} else {
		pt.lastWallTime = now
		pt.lastCPUTime = totalCPUTime
	}

	pt.lastMetrics.RAMBytes = totalRAM
	pt.lastSampleTime = now
	return pt.lastMetrics
}

func (pt *ProcessTracker) getProcessTreePids() []int {
	pids := []int{pt.pid}

	snapshot, _, _ := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snapshot == uintptr(syscall.InvalidHandle) {
		return pids
	}
	defer procCloseHandle.Call(snapshot)

	var entry processEntry32W
	entry.dwSize = uint32(unsafe.Sizeof(entry))

	ret, _, _ := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return pids
	}

	// Map parent to children
	childrenMap := make(map[uint32][]uint32)
	for {
		childrenMap[entry.th32ParentProcessID] = append(childrenMap[entry.th32ParentProcessID], entry.th32ProcessID)
		ret, _, _ := procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}

	// Breadth-first collect all descendants
	queue := []uint32{uint32(pt.pid)}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range childrenMap[parent] {
			pids = append(pids, int(child))
			queue = append(queue, child)
		}
	}

	return pids
}

func getSingleProcessMetrics(pid int) (uint64, uint64) {
	h, _, _ := procOpenProcess.Call(processQueryInformation|processVMRead, 0, uintptr(pid))
	if h == 0 {
		return 0, 0
	}
	defer procCloseHandle.Call(h)

	var mem processMemoryCounters
	mem.cb = uint32(unsafe.Sizeof(mem))
	_, _, _ = procGetProcessMemoryInfo.Call(h, uintptr(unsafe.Pointer(&mem)), uintptr(mem.cb))

	var creationTime, exitTime, kernelTime, userTime syscall.Filetime
	ret, _, _ := procGetProcessTimes.Call(
		h,
		uintptr(unsafe.Pointer(&creationTime)),
		uintptr(unsafe.Pointer(&exitTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)

	var totalCPUTime uint64
	if ret != 0 {
		k := uint64(kernelTime.HighDateTime)<<32 | uint64(kernelTime.LowDateTime)
		u := uint64(userTime.HighDateTime)<<32 | uint64(userTime.LowDateTime)
		totalCPUTime = k + u
	}

	return uint64(mem.workingSetSize), totalCPUTime
}
