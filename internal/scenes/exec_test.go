package scenes_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tjagithub/rosebud/internal/scenes"
)

// TestMain doubles as the helper program: tests run os.Args[0] with
// SCENES_HELPER set to pick a behaviour.
func TestMain(m *testing.M) {
	switch os.Getenv("SCENES_HELPER") {
	case "":
		goleak.VerifyTestMain(m)
	case "echo":
		fmt.Print("hello")
	case "fail":
		fmt.Fprint(os.Stderr, "bad input")
		os.Exit(3)
	case "sleep":
		time.Sleep(5 * time.Second)
	case "orphan":
		// Leave a grandchild holding our stdout open, then hang.
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "SCENES_HELPER=sleep")
		child.Stdout = os.Stdout
		_ = child.Start()
		time.Sleep(5 * time.Second)
	}
	os.Exit(0)
}

func helper(t *testing.T, mode string, opts ...scenes.ExecOption) (scenes.Exec, string) {
	t.Helper()
	t.Setenv("SCENES_HELPER", mode)
	return scenes.NewExec(opts...), os.Args[0]
}

func TestExec_Output(t *testing.T) {
	run, bin := helper(t, "echo")
	out, err := run(context.Background(), bin)
	if err != nil || string(out) != "hello" {
		t.Errorf("out = %q, err = %v; want hello", out, err)
	}
}

func TestExec_Failure(t *testing.T) {
	run, bin := helper(t, "fail")
	_, err := run(context.Background(), bin)
	if err == nil || !strings.Contains(err.Error(), "bad input") || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("err = %v, want exit status 3 with stderr", err)
	}
}

func TestExec_MissingProgram(t *testing.T) {
	run := scenes.NewExec()
	if _, err := run(context.Background(), "definitely-not-a-program-rose"); err == nil {
		t.Error("err = nil, want a start error")
	}
}

func TestExec_Cancel(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{"kills a running program", "sleep"},
		{"does not hang on an orphan holding the pipe", "orphan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, bin := helper(t, tt.mode, scenes.WithWaitDelay(100*time.Millisecond))
			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(200*time.Millisecond, cancel)

			start := time.Now()
			_, err := run(ctx, bin)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("err = %v, want context.Canceled", err)
			}
			if d := time.Since(start); d > 3*time.Second {
				t.Errorf("returned after %v, want well under the helper's 5s sleep", d)
			}
		})
	}
}
