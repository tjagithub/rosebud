package cache_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/cache"
)

func key(c byte) string { return strings.Repeat(string(c), 64) }

func TestPutGet(t *testing.T) {
	d := cache.Dir{Path: t.TempDir()}
	if _, ok, err := d.Get(key('a')); ok || err != nil {
		t.Fatalf("miss: ok=%t err=%v", ok, err)
	}
	if err := d.Put(key('a'), []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(key('a'), []byte("two")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := d.Get(key('a'))
	if err != nil || !ok || string(got) != "two" {
		t.Errorf("got %q ok=%t err=%v", got, ok, err)
	}
	var tmp []string
	_ = filepath.WalkDir(d.Path, func(p string, e os.DirEntry, _ error) error {
		if strings.Contains(e.Name(), ".tmp") {
			tmp = append(tmp, p)
		}
		return nil
	})
	if len(tmp) != 0 {
		t.Errorf("temp files left: %v", tmp)
	}
}

func TestRejectsUnsafeKeys(t *testing.T) {
	d := cache.Dir{Path: t.TempDir()}
	for _, k := range []string{"", "a", "../../etc/passwd", "ab/cd", strings.Repeat("g", 64)} {
		if err := d.Put(k, []byte("x")); err == nil {
			t.Errorf("Put(%q): no error", k)
		}
		if _, _, err := d.Get(k); err == nil {
			t.Errorf("Get(%q): no error", k)
		}
	}
}

func TestEmptyDirIsAnError(t *testing.T) {
	if err := (cache.Dir{}).Put(key('a'), nil); err == nil {
		t.Error("no error for an unset path")
	}
}

func age(t *testing.T, d cache.Dir, k string, by time.Duration) {
	t.Helper()
	p := filepath.Join(d.Path, k[:2], k)
	old := time.Now().Add(-by)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestSweepByAgeAndSize(t *testing.T) {
	d := cache.Dir{Path: t.TempDir()}
	for i, c := range []byte("abcd") {
		if err := d.Put(key(c), []byte("0123456789")); err != nil {
			t.Fatal(err)
		}
		age(t, d, key(c), time.Duration(4-i)*24*time.Hour) // a is oldest
	}
	n, err := d.Sweep(t.Context(), 1<<20, 3*24*time.Hour+time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("age sweep removed %d, err %v; want 1", n, err)
	}
	if _, ok, _ := d.Get(key('a')); ok {
		t.Error("the oldest entry survived the age limit")
	}
	n, err = d.Sweep(t.Context(), 20, 365*24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("size sweep removed %d, err %v; want 1", n, err)
	}
	if _, ok, _ := d.Get(key('b')); ok {
		t.Error("size limit kept the oldest and dropped a newer one")
	}
}

func TestGetRefreshesAge(t *testing.T) {
	d := cache.Dir{Path: t.TempDir()}
	_ = d.Put(key('a'), []byte("x"))
	_ = d.Put(key('b'), []byte("x"))
	age(t, d, key('a'), 10*24*time.Hour)
	age(t, d, key('b'), 9*24*time.Hour)
	if _, ok, _ := d.Get(key('a')); !ok {
		t.Fatal("miss")
	}
	if _, err := d.Sweep(t.Context(), 1, 365*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := d.Get(key('a')); !ok {
		t.Error("recently used entry was evicted before an older one")
	}
}

func TestSweepMissingDirAndCancel(t *testing.T) {
	if n, err := (cache.Dir{Path: filepath.Join(t.TempDir(), "none")}).Sweep(t.Context(), 1, time.Hour); err != nil || n != 0 {
		t.Errorf("missing dir: %d %v", n, err)
	}
	d := cache.Dir{Path: t.TempDir()}
	_ = d.Put(key('a'), []byte("x"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.Sweep(ctx, 1, time.Hour); err == nil {
		t.Error("cancelled sweep: no error")
	}
}
