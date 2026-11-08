package langid

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"time"
)

// RMS is the loudness of a 16-bit mono PCM WAV, from 0 to 1.
func RMS(wav string) (float64, error) {
	b, err := os.ReadFile(wav)
	if err != nil {
		return 0, err
	}
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0, fmt.Errorf("langid: %s is not a WAV", wav)
	}
	for p := 12; p+8 <= len(b); {
		size := int(binary.LittleEndian.Uint32(b[p+4:]))
		body := b[p+8:]
		if string(b[p:p+4]) == "data" {
			body = body[:min(size, len(body))]
			n := len(body) / 2
			if n == 0 {
				return 0, nil
			}
			var sum float64
			for i := range n {
				s := float64(int16(binary.LittleEndian.Uint16(body[2*i:]))) / 32768
				sum += s * s
			}
			return math.Sqrt(sum / float64(n)), nil
		}
		p += 8 + size + size%2
	}
	return 0, fmt.Errorf("langid: %s has no audio data", wav)
}

// Spread is where n windows of length win start so that they sit at the
// centres of n equal parts of a film. A film shorter than a window gets one at 0.
func Spread(total time.Duration, n int, win time.Duration) []time.Duration {
	if n <= 0 {
		return nil
	}
	if total <= win {
		return []time.Duration{0}
	}
	out := make([]time.Duration, n)
	for i := range out {
		centre := total * time.Duration(2*i+1) / time.Duration(2*n)
		out[i] = min(max(centre-win/2, 0), total-win)
	}
	return out
}
