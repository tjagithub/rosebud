package lock_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tjagithub/rosebud/internal/lock"
)

func TestSecondAcquireFailsUntilReleased(t *testing.T) {
	dir := t.TempDir()
	release, err := lock.Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = lock.Acquire(dir)
	if err == nil || !strings.Contains(err.Error(), dir) {
		t.Fatalf("second acquire: err = %v, want one naming %s", err, dir)
	}
	if !strings.Contains(err.Error(), "pid "+strconv.Itoa(os.Getpid())) {
		t.Errorf("err = %v, want the holder's pid", err)
	}
	release()
	release() // releasing twice is harmless
	again, err := lock.Acquire(dir)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}

func TestDifferentDirsDoNotClash(t *testing.T) {
	a, err := lock.Acquire(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a()
	b, err := lock.Acquire(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b()
}

func TestMissingDirIsAnError(t *testing.T) {
	if _, err := lock.Acquire(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("no error for a missing directory")
	}
}

func TestStaleLockFileDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".rose.lock"), []byte("left by a crash"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := lock.Acquire(dir)
	if err != nil {
		t.Fatalf("stale file blocked: %v", err)
	}
	release()
}

func TestHolderIsRecordedWhileHeld(t *testing.T) {
	dir := t.TempDir()
	release, err := lock.Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	data, err := os.ReadFile(filepath.Join(dir, ".rose.lock"))
	if err != nil || len(data) == 0 {
		t.Fatalf("holder info missing while held: %q %v", data, err)
	}
}
