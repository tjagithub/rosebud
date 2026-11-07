package main

import (
	"context"
	"errors"
	"testing"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
)

func TestRunJobReportsSkipAsSkippedError(t *testing.T) {
	eng := engine.New(nil, engine.WithStage("ocr", func(context.Context, core.JobSpec, func(int64, int64)) error {
		return core.Skip("no track")
	}))
	t.Cleanup(func() { _ = eng.Close() })
	_, err := runJob(context.Background(), eng, core.JobSpec{Stage: "ocr", Film: "f"}, &bytesDiscard{})
	var sk *skippedError
	if !errors.As(err, &sk) || sk.reason != "no track" || sk.stage != "ocr" {
		t.Errorf("err = %#v", err)
	}
}

type bytesDiscard struct{}

func (*bytesDiscard) Write(p []byte) (int, error) { return len(p), nil }
