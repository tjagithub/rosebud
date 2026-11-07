package scenes

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
)

// ParseHWAccels reads the method names out of `ffmpeg -hwaccels` output.
func ParseHWAccels(out string) []string {
	var list []string
	in := false
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(ln, "Hardware acceleration methods"):
			in = true
		case in && ln != "" && !strings.ContainsAny(ln, " :") && !slices.Contains(list, ln):
			list = append(list, ln)
		}
	}
	return list
}

// HWAccels lists the hardware decode methods this ffmpeg offers. It gives up
// after timeout, which must be positive.
func HWAccels(ctx context.Context, ex Exec, timeout time.Duration) ([]string, error) {
	out, err := limit.Run(ctx, "ffmpeg", timeout, func(ctx context.Context) ([]byte, error) {
		return ex(ctx, "ffmpeg", "-hide_banner", "-hwaccels")
	})
	if err != nil {
		return nil, fmt.Errorf("scenes: list hardware decoders: %w", err)
	}
	return ParseHWAccels(string(out)), nil
}

// CheckHWAccel says whether method can be used: empty and "auto" always can,
// anything else must be in avail.
func CheckHWAccel(method string, avail []string) error {
	if method == "" || method == "auto" || slices.Contains(avail, method) {
		return nil
	}
	if len(avail) == 0 {
		return fmt.Errorf("this ffmpeg offers no hardware decoders, so %q cannot be used", method)
	}
	return fmt.Errorf("this ffmpeg does not offer %q; it offers %s", method, strings.Join(avail, ", "))
}
