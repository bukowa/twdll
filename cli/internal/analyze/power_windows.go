//go:build windows

package analyze

import (
	"runtime"
	"sync"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

var procSetThreadExecutionState = modKernel32.NewProc("SetThreadExecutionState")

func acquirePowerAssertion() func() {
	readyCh := make(chan struct{})
	releaseCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		_, _, _ = procSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired))
		close(readyCh)
		<-releaseCh
		_, _, _ = procSetThreadExecutionState.Call(uintptr(esContinuous))
		close(doneCh)
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
