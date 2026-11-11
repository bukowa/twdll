//go:build windows

package analyze

import (
	"syscall"
)

var (
	modkernel32Power            = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = modkernel32Power.NewProc("SetThreadExecutionState")
)

const (
	esSystemRequired = 0x00000001
	esContinuous     = 0x80000000
)

// acquirePowerAssertion requests Windows to stay awake (prevent idle sleep)
// while long-running decompiler analysis batches are in progress.
// It returns a cleanup function that restores default power/sleep behavior.
func acquirePowerAssertion() func() {
	if procSetThreadExecutionState.Find() != nil {
		return func() {}
	}
	// ES_CONTINUOUS | ES_SYSTEM_REQUIRED
	procSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired))

	return func() {
		// Clear ES_SYSTEM_REQUIRED by setting ES_CONTINUOUS alone
		procSetThreadExecutionState.Call(uintptr(esContinuous))
	}
}
