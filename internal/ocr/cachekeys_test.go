package ocr_test

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tjagithub/rosebud/internal/cache"
)

func keysIn(t *testing.T, d cache.Dir) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(d.Path, func(p string, e fs.DirEntry, _ error) error {
		if !e.IsDir() && !strings.Contains(e.Name(), ".tmp") {
			out = append(out, e.Name())
		}
		return nil
	})
	return out
}
