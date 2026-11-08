package langid_test

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/langid"
)

// writeWAV writes 16-bit mono PCM behind a header with an extra LIST chunk.
func writeWAV(t *testing.T, samples []int16) string {
	t.Helper()
	var b []byte
	le := binary.LittleEndian
	b = append(b, "RIFF\x00\x00\x00\x00WAVE"...)
	b = append(b, "fmt "...)
	b = le.AppendUint32(b, 16)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint32(b, 16000)
	b = le.AppendUint32(b, 32000)
	b = le.AppendUint16(b, 2)
	b = le.AppendUint16(b, 16)
	b = append(b, "LIST"...)
	b = le.AppendUint32(b, 4)
	b = append(b, "INFO"...)
	b = append(b, "data"...)
	b = le.AppendUint32(b, uint32(2*len(samples)))
	for _, s := range samples {
		b = le.AppendUint16(b, uint16(s))
	}
	p := filepath.Join(t.TempDir(), "w.wav")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRMS(t *testing.T) {
	got, err := langid.RMS(writeWAV(t, []int16{16384, -16384, 16384, -16384}))
	if err != nil || math.Abs(got-0.5) > 1e-9 {
		t.Errorf("rms = %v, %v, want 0.5", got, err)
	}
	if got, err := langid.RMS(writeWAV(t, nil)); err != nil || got != 0 {
		t.Errorf("empty rms = %v, %v", got, err)
	}
	if _, err := langid.RMS(filepath.Join(t.TempDir(), "none.wav")); err == nil {
		t.Error("missing file: no error")
	}
}

func TestSpread(t *testing.T) {
	got := langid.Spread(100*time.Second, 4, 10*time.Second)
	if len(got) != 4 {
		t.Fatalf("got %v", got)
	}
	for i, w := range []time.Duration{7500 * time.Millisecond, 32500 * time.Millisecond, 57500 * time.Millisecond, 82500 * time.Millisecond} {
		if got[i] != w {
			t.Errorf("start %d = %v, want %v", i, got[i], w)
		}
	}
	if s := langid.Spread(5*time.Second, 3, 10*time.Second); len(s) != 1 || s[0] != 0 {
		t.Errorf("short film: %v, want one window at 0", s)
	}
	if s := langid.Spread(time.Hour, 0, time.Second); s != nil {
		t.Errorf("n=0: %v", s)
	}
}
