//go:build unix

package updatersim

import "syscall"

// freeDiskBytes returns the bytes available to an unprivileged process on the
// filesystem backing path. It is used to refuse staging an artifact that would
// not fit (invariant 7: a failed, half-written install is worse than a clean
// skip-and-retry). Works on Linux and macOS; other platforms fall back to the
// no-op in diskspace_other.go, which disables the check rather than blocking.
func freeDiskBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	// Bavail is blocks available to a non-root user; Bsize is the block size.
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
