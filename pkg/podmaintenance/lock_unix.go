//go:build linux || darwin

package podmaintenance

import (
	"os"
	"path/filepath"
	"syscall"
)

func lock(dir string) (func(), error) {
	fd, e := syscall.Open(filepath.Join(dir, "operation.lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "operation.lock")
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return func() { syscall.Flock(fd, syscall.LOCK_UN); f.Close() }, nil
}
