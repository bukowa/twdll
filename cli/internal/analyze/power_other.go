//go:build !windows

package analyze

// AcquirePowerAssertion prevents the OS from entering sleep/standby while
// a long-running analysis or extraction job is active. Returns a release func.
func AcquirePowerAssertion() func() {
	return acquirePowerAssertion()
}

func acquirePowerAssertion() func() {
	return func() {}
}
