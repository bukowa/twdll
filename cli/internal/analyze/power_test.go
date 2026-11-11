package analyze

import (
	"testing"
)

func TestAcquirePowerAssertion(t *testing.T) {
	release := acquirePowerAssertion()
	if release == nil {
		t.Fatal("expected non-nil cleanup function")
	}
	// Calling cleanup should succeed without panicking
	release()
}
