//go:build windows

package analyze

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modkernel32Mem           = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = modkernel32Mem.NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func getSystemMemory() (SystemMemory, error) {
	if err := procGlobalMemoryStatusEx.Find(); err != nil {
		return SystemMemory{}, fmt.Errorf("GlobalMemoryStatusEx not found: %w", err)
	}
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	ret, _, callErr := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if ret == 0 {
		return SystemMemory{}, fmt.Errorf("GlobalMemoryStatusEx failed: %w", callErr)
	}
	return SystemMemory{
		TotalPhysBytes:   ms.TotalPhys,
		AvailPhysBytes:   ms.AvailPhys,
		TotalCommitBytes: ms.TotalPageFile,
		AvailCommitBytes: ms.AvailPageFile,
	}, nil
}
