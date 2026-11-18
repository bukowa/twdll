//go:build windows

package analyze

import "testing"

func TestAcquirePowerAssertion_RefCountAndIdempotentRelease(t *testing.T) {
	origStart := startPowerAssertion
	defer func() {
		powerMu.Lock()
		startPowerAssertion = origStart
		powerRefs = 0
		powerRelease = nil
		powerMu.Unlock()
	}()

	starts := 0
	releases := 0
	startPowerAssertion = func() func() {
		starts++
		return func() {
			releases++
		}
	}

	rel1 := AcquirePowerAssertion()
	rel2 := AcquirePowerAssertion()
	if starts != 1 || releases != 0 {
		t.Fatalf("after 2 acquires: starts=%d releases=%d, want 1/0", starts, releases)
	}

	// Double-calling rel2 must only decrement once.
	rel2()
	rel2()
	if releases != 0 {
		t.Fatalf("after releasing inner twice: releases=%d, want 0", releases)
	}

	rel1()
	rel1()
	if releases != 1 {
		t.Fatalf("after releasing outer: releases=%d, want 1", releases)
	}

	// Subsequent cycle starts cleanly again.
	rel3 := AcquirePowerAssertion()
	if starts != 2 {
		t.Fatalf("second cycle starts=%d, want 2", starts)
	}
	rel3()
	if releases != 2 {
		t.Fatalf("second cycle releases=%d, want 2", releases)
	}
}

func TestAcquirePowerAssertion_RealWin32Lifecycle(t *testing.T) {
	rel1 := AcquirePowerAssertion()
	rel2 := AcquirePowerAssertion()
	rel2()
	rel1()
}
