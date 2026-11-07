package translate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/tjagithub/rosebud/internal/cache"
)

// Cached translates each distinct line once. Misses go to Inner Batch lines
// at a time, saved before the next batch, so a cancelled run keeps its progress.
type Cached struct {
	Inner Translator
	Dir   cache.Dir
	Salt  string // names the model and its setup; a change misses every entry
	Batch int
}

// Translate returns the translation of each line. Cache failures only cost a re-run.
func (c Cached) Translate(ctx context.Context, lines []string) ([]string, error) {
	if c.Inner == nil || c.Dir.Path == "" || c.Salt == "" || c.Batch <= 0 {
		return nil, errors.New("translate: cached needs Inner, Dir, Salt and a positive Batch")
	}
	got := map[string]string{}
	var miss []string
	for _, l := range lines {
		if _, seen := got[l]; seen || l == "" {
			continue
		}
		if b, ok, _ := c.Dir.Get(lineKey(c.Salt, l)); ok {
			got[l] = string(b)
			continue
		}
		got[l] = ""
		miss = append(miss, l)
	}
	total := len(got)
	report(ctx, total-len(miss), total)
	for len(miss) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := miss[:min(c.Batch, len(miss))]
		miss = miss[len(batch):]
		res, err := c.Inner.Translate(ctx, batch)
		if err != nil {
			return nil, err
		}
		if len(res) != len(batch) {
			return nil, fmt.Errorf("translate: translator returned %d lines for %d", len(res), len(batch))
		}
		for i, l := range batch {
			got[l] = res[i]
			_ = c.Dir.Put(lineKey(c.Salt, l), []byte(res[i]))
		}
		report(ctx, total-len(miss), total)
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = got[l]
	}
	return out, nil
}

func lineKey(salt, line string) string {
	h := sha256.Sum256([]byte(salt + "\x00" + line))
	return hex.EncodeToString(h[:])
}
