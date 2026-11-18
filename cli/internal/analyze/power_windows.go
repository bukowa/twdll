//go:build windows

package analyze

import (
	"runtime"
	"sync"
	"time"
)

const (
	esContinuous       = 0x80000000
	esSystemRequired   = 0x00000001
	esDisplayRequired  = 0x00000002
	esAwayModeRequired = 0x00000040
)

var procSetThreadExecutionState = modKernel32.NewProc("SetThreadExecutionState")

// AcquirePowerAssertion prevents the OS from entering sleep/standby while
// a long-running analysis or extraction job is active. Returns a release func.
func AcquirePowerAssertion() func() {
	return acquirePowerAssertion()
}

func acquirePowerAssertion() func() {
	readyCh := make(chan struct{})
	releaseCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		activeFlags := uintptr(esContinuous | esSystemRequired | esDisplayRequired | esAwayModeRequired)
		_, _, _ = procSetThreadExecutionState.Call(activeFlags)
		close(readyCh)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, _ = procSetThreadExecutionState.Call(activeFlags)
			case <-releaseCh:
				_, _, _ = procSetThreadExecutionState.Call(uintptr(esContinuous))
				close(doneCh)
				return
			}
		}
	}()

	<-readyCh
	var once sync.Once
	return func() {
		once.Do(func() {
			close(releaseCh)
			<-doneCh
		})
	}
}
