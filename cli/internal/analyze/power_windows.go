//go:build windows

package analyze

import (
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	esContinuous       = 0x80000000
	esSystemRequired   = 0x00000001
	esAwayModeRequired = 0x00000040

	powerRequestContextVersion      = 0
	powerRequestContextSimpleString = 0x00000001

	powerRequestSystemRequired    = 1
	powerRequestAwayModeRequired  = 2
	powerRequestExecutionRequired = 3

	defaultPowerRequestReason = "twdll: analysis or extraction in progress"
)

// reasonContext matches Win32 REASON_CONTEXT (winnt.h / minwinbase.h) when
// flags == powerRequestContextSimpleString. The trailing fields preserve the
// full size of the Detailed union branch (32 bytes on 64-bit, 24 on 32-bit).
type reasonContext struct {
	version            uint32
	flags              uint32
	simpleReasonString *uint16
	_                  uint32
	_                  uint32
	_                  uintptr
}

var (
	procPowerCreateRequest      = modKernel32.NewProc("PowerCreateRequest")
	procPowerSetRequest         = modKernel32.NewProc("PowerSetRequest")
	procPowerClearRequest       = modKernel32.NewProc("PowerClearRequest")
	procSetThreadExecutionState = modKernel32.NewProc("SetThreadExecutionState")
)

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
	if rel, active, ok := acquirePowerRequest(defaultPowerRequestReason); ok && len(active) > 0 {
		return rel
	}
	return acquireThreadExecutionState()
}

func acquirePowerRequest(reason string) (release func(), active []uintptr, ok bool) {
	if err := procPowerCreateRequest.Find(); err != nil {
		return nil, nil, false
	}
	if err := procPowerSetRequest.Find(); err != nil {
		return nil, nil, false
	}
	if err := procPowerClearRequest.Find(); err != nil {
		return nil, nil, false
	}

	reasonPtr, err := windows.UTF16PtrFromString(reason)
	if err != nil {
		return nil, nil, false
	}
	ctx := reasonContext{
		version:            powerRequestContextVersion,
		flags:              powerRequestContextSimpleString,
		simpleReasonString: reasonPtr,
	}

	h, _, _ := procPowerCreateRequest.Call(uintptr(unsafe.Pointer(&ctx)))
	runtime.KeepAlive(reasonPtr)
	runtime.KeepAlive(&ctx)
	if h == 0 || h == uintptr(windows.InvalidHandle) {
		return nil, nil, false
	}

	desired := []uintptr{
		powerRequestSystemRequired,
		powerRequestAwayModeRequired,
		powerRequestExecutionRequired,
	}
	active = make([]uintptr, 0, len(desired))
	for _, reqType := range desired {
		r1, _, _ := procPowerSetRequest.Call(h, reqType)
		if r1 != 0 {
			active = append(active, reqType)
		}
	}
	if len(active) == 0 {
		_ = windows.CloseHandle(windows.Handle(h))
		return nil, nil, false
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			for _, reqType := range active {
				_, _, _ = procPowerClearRequest.Call(h, reqType)
			}
			_ = windows.CloseHandle(windows.Handle(h))
		})
	}
	return release, active, true
}

func acquireThreadExecutionState() func() {
	readyCh := make(chan struct{})
	releaseCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		activeFlags := uintptr(esContinuous | esSystemRequired | esAwayModeRequired)
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
