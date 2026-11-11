//go:build !windows

package analyze

// acquirePowerAssertion is a no-op on non-Windows platforms.
func acquirePowerAssertion() func() {
	return func() {}
}
