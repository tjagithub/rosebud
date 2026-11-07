//go:build unix

// Package lock stops two rose commands from writing to one bundle at once.
package lock

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Acquire locks dir without waiting; the OS drops the lock when the process
// ends. The lock file names the holder, so a refusal can say who has it.
func Acquire(dir string) (release func(), err error) {
	path := filepath.Join(dir, ".rose.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		defer func() { _ = f.Close() }()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("lock: %s: %w", path, err)
		}
		by := "another rose command"
		if b, _ := io.ReadAll(io.LimitReader(f, 256)); len(strings.TrimSpace(string(b))) > 0 {
			by = strings.TrimSpace(string(b))
		}
		return nil, fmt.Errorf("lock: %s is in use by %s", dir, by)
	}
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(holder()), 0)
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = f.Truncate(0)
			_ = f.Close()
		})
	}, nil
}

// holder describes this process, such as "rose ocr (pid 4821, since 15:02:07)".
func holder() string {
	name := filepath.Base(os.Args[0])
	if len(os.Args) > 1 {
		name += " " + os.Args[1]
	}
	return fmt.Sprintf("%s (pid %d, since %s)", name, os.Getpid(), time.Now().Format("15:04:05"))
}
