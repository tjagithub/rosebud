//go:build unix

package lock_test

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/lock"
)

// TestHelperHolder is the child process: it takes the lock and waits to be killed.
func TestHelperHolder(t *testing.T) {
	dir := os.Getenv("LOCK_HELPER_DIR")
	if dir == "" {
		t.Skip("only runs as a child of the cross-process tests")
	}
	if _, err := lock.Acquire(dir); err != nil {
		os.Stdout.WriteString("error " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Stdout.WriteString("held\n")
	time.Sleep(time.Minute)
}

// startHolder runs the helper and returns once it holds the lock.
func startHolder(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHelperHolder$")
	cmd.Env = append(os.Environ(), "LOCK_HELPER_DIR="+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(out).ReadString('\n')
		line <- strings.TrimSpace(s)
	}()
	select {
	case l := <-line:
		if l != "held" {
			t.Fatalf("helper said %q", l)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("helper never took the lock")
	}
	return cmd
}

func TestOtherProcessHoldsTheLock(t *testing.T) {
	dir := t.TempDir()
	cmd := startHolder(t, dir)
	_, err := lock.Acquire(dir)
	if err == nil || !strings.Contains(err.Error(), "pid "+strconv.Itoa(cmd.Process.Pid)) {
		t.Fatalf("err = %v, want a refusal naming pid %d", err, cmd.Process.Pid)
	}
}

func TestKilledHolderReleasesTheLock(t *testing.T) {
	dir := t.TempDir()
	cmd := startHolder(t, dir)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	release, err := lock.Acquire(dir)
	if err != nil {
		t.Fatalf("lock still held after the holder was killed: %v", err)
	}
	release()
}
