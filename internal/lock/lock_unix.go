//go:build unix

// Package lock stops two rose commands from writing to one bundle at once.
package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// Acquire takes an exclusive lock on dir without waiting. The operating
// system drops it when the process ends, so a crash leaves nothing stale.
func Acquire(dir string) (release func(), err error) {
	path := filepath.Join(dir, ".rose.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("lock: %s is in use by another rose command (%s)", dir, path)
		}
		return nil, fmt.Errorf("lock: %s: %w", path, err)
	}
	var once sync.Once
	return func() { once.Do(func() { _ = f.Close() }) }, nil
}
