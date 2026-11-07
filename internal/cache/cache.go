// Package cache keeps small results on disk, keyed by a content hash, and
// trims them by age and size.
package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

var keyRE = regexp.MustCompile(`^[0-9a-f]{16,128}$`)

// Dir is a cache directory. Keys are lower-case hex hashes, so a key can
// never name a path outside it.
type Dir struct{ Path string }

func (d Dir) file(key string) (string, error) {
	if d.Path == "" {
		return "", errors.New("cache: no directory set")
	}
	if !keyRE.MatchString(key) {
		return "", fmt.Errorf("cache: bad key %q", key)
	}
	return filepath.Join(d.Path, key[:2], key), nil
}

// Get returns the value for key. A hit counts as use, so it is trimmed last.
func (d Dir) Get(key string) ([]byte, bool, error) {
	p, err := d.file(key)
	if err != nil {
		return nil, false, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache: %w", err)
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now)
	return b, true, nil
}

// Put stores value under key. It writes a temp file and renames it, so a
// crash never leaves a half-written entry.
func (d Dir) Put(key string, value []byte) error {
	p, err := d.file(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("cache: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(p), key+".tmp")
	if err != nil {
		return fmt.Errorf("cache: %w", err)
	}
	_, err = f.Write(value)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), p)
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("cache: %w", err)
	}
	return nil
}

// Sweep removes files unused for longer than maxAge, then the least recently
// used until the rest fit in maxBytes. It returns how many it removed.
func (d Dir) Sweep(ctx context.Context, maxBytes int64, maxAge time.Duration) (int, error) {
	type entry struct {
		path string
		size int64
		used time.Time
	}
	var all []entry
	var total int64
	err := filepath.WalkDir(d.Path, func(p string, e fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil || e.IsDir() {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		all = append(all, entry{p, info.Size(), info.ModTime()})
		total += info.Size()
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("cache: sweep: %w", err)
	}
	slices.SortFunc(all, func(a, b entry) int { return a.used.Compare(b.used) })
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range all {
		if e.used.After(cutoff) && total <= maxBytes {
			break
		}
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if err := os.Remove(e.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return removed, fmt.Errorf("cache: sweep: %w", err)
		}
		total -= e.size
		removed++
	}
	return removed, nil
}
