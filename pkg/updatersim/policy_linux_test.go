//go:build linux

package updatersim

import (
	"errors"
	"testing"
)

func TestPolicyProcessLockSerializesIndependentSimulators(t *testing.T) {
	cfg := newSimulatorTestConfig(t, "http://localhost", ModeReal)
	a := &Simulator{config: cfg}
	b := &Simulator{config: cfg}
	release, err := a.policyCycleLock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.policyCycleLock(); !errors.Is(err, ErrCycleInProgress) {
		t.Fatalf("second process accepted: %v", err)
	}
	release()
	release2, err := b.policyCycleLock()
	if err != nil {
		t.Fatal(err)
	}
	release2()
}
