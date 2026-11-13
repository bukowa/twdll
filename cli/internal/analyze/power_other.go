//go:build !windows

package analyze

func acquirePowerAssertion() func() {
	return func() {}
}
