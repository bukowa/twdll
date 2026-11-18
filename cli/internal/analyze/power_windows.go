//go:build windows

package analyze

import (
	"runtime"
	"sync"
	"time"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

var procSetThreadExecutionState = modKernel32.NewProc("SetThreadExecutionState")

var (
	powerMu             sync.Mutex
	powerRefs           int
	powerRelease        func()
	startPowerAssertion = acquirePowerAssertion
)

// AcquirePowerAssertion prevents the OS from entering sleep or idle hibernation while
// a long-running analysis or extraction job is active. Nested calls share one assertion.
func AcquirePowerAssertion() func() {
	powerMu.Lock()
	if powerRefs == 0 {
		powerRelease = startPowerAssertion()
	}
	powerRefs++
	powerMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			powerMu.Lock()
			powerRefs--
			var rel func()
			if powerRefs == 0 {
				rel = powerRelease
				powerRelease = nil
			}
			powerMu.Unlock()
			if rel != nil {
				rel()
			}
		})
	}
}

func acquirePowerAssertion() func() {
	readyCh := make(chan struct{})
	releaseCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		activeFlags := uintptr(esContinuous | esSystemRequired)
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
