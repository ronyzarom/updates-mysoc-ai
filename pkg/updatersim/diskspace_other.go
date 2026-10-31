//go:build !unix

package updatersim

import "errors"

// freeDiskBytes is unsupported off unix; callers treat the error as "skip the
// free-disk check" so an install is never blocked on an unsupported platform.
func freeDiskBytes(string) (uint64, error) {
	return 0, errors.New("free disk measurement unsupported on this platform")
}
