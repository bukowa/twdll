//go:build windows

package analyze

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type memoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

var (
	modKernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = modKernel32.NewProc("GlobalMemoryStatusEx")
)

func ProbeSystemMemory() SystemMemory {
	var msx memoryStatusEx
	msx.length = uint32(unsafe.Sizeof(msx))
	r1, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&msx)))
	if r1 == 0 {
		return SystemMemory{}
	}
	return SystemMemory{
		TotalPhysBytes:   msx.totalPhys,
		AvailPhysBytes:   msx.availPhys,
		TotalCommitBytes: msx.totalPageFile,
		AvailCommitBytes: msx.availPageFile,
	}
}
