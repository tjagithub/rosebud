// Package subload lists the subtitles a film could be read from and loads one
// into its scenes, for frontends that let the user choose.
package subload

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/lock"
	"github.com/tjagithub/rosebud/internal/ocr"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/subsrc"
)

// Loader reads subtitles into the catalog that holds a film.
type Loader struct {
	Exec      ocr.Exec
	Store     func(ctx context.Context, film core.FilmID) (*store.Store, error)
	NewReader func(bundle string) (ocr.Reader, error) // for bitmap tracks; nil if OCR is not set up
	Timeout   time.Duration                           // for reading a track
}

// probeTimeout bounds the quick ffprobe calls.
const probeTimeout = time.Minute

// Choices lists the film's subtitle tracks, then the subtitle files beside its video.
func (l Loader) Choices(ctx context.Context, film core.Film) ([]core.SubtitleChoice, error) {
	src, err := Source(film)
	if err != nil {
		return nil, err
	}
	tracks, err := subsrc.Probe(ctx, l.Exec, src, probeTimeout)
	if err != nil {
		return nil, err
	}
	files, err := subsrc.Sidecars(src)
	if err != nil {
		return nil, err
	}
	var out []core.SubtitleChoice
	for _, t := range append(tracks, files...) {
		if t.Kind() == subsrc.Unknown {
			continue
		}
		c := core.SubtitleChoice{Ref: strconv.Itoa(t.Pos), Lang: t.Lang, Codec: t.Codec, Title: t.Title, Bitmap: t.Kind() == subsrc.Bitmap}
		if t.Path != "" {
			c.Ref, c.File = t.Path, filepath.Base(t.Path)
		}
		out = append(out, c)
	}
	return out, nil
}

// Load reads the choice ref, a track number or a subtitle file, into the
// film's scenes, holding the bundle's lock. progress may be nil.
func (l Loader) Load(ctx context.Context, film core.Film, ref string, progress func(done, total int64)) error {
	src, err := Source(film)
	if err != nil {
		return err
	}
	var rd ocr.Reader = noOCR{}
	if l.NewReader != nil {
		if rd, err = l.NewReader(film.Bundle); err != nil {
			return err
		}
	} else if l.isBitmap(ctx, film, ref) {
		return errors.New("bitmap subtitles need OCR: run `rose ocr --track " + ref + " FILE`")
	}
	st, err := l.Store(ctx, film.ID)
	if err != nil {
		return err
	}
	unlock, err := lock.Acquire(film.Bundle)
	if err != nil {
		return err
	}
	defer unlock()
	if progress == nil {
		progress = func(int64, int64) {}
	}
	stage := ocr.Stage{Exec: l.Exec, Store: st, Reader: rd, ProbeTimeout: probeTimeout, Timeout: l.Timeout}
	return stage.Run(ctx, core.JobSpec{Stage: "ocr", Film: film.ID, Params: map[string]string{"file": src, "track": ref}}, progress)
}

// isBitmap is whether ref is one of the film's bitmap tracks.
func (l Loader) isBitmap(ctx context.Context, film core.Film, ref string) bool {
	choices, err := l.Choices(ctx, film)
	if err != nil {
		return false
	}
	for _, c := range choices {
		if c.Ref == ref {
			return c.Bitmap
		}
	}
	return false
}

// noOCR is the reader when OCR is not set up.
type noOCR struct{}

func (noOCR) Read(context.Context, []image.Image) ([][]ocr.Block, error) {
	return nil, errors.New("bitmap subtitles need OCR, which is not set up here")
}

// Source is the film's video: the one recorded when its scenes were cut, or
// else the file named like its bundle beside it, for bundles made before.
func Source(film core.Film) (string, error) {
	if film.Source != "" {
		return film.Source, nil
	}
	stem := strings.TrimSuffix(filepath.Base(film.Bundle), ".film")
	dir := filepath.Dir(film.Bundle)
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() && strings.TrimSuffix(name, filepath.Ext(name)) == stem && isVideo(name) {
				return filepath.Join(dir, name), nil
			}
		}
	}
	return "", fmt.Errorf("no video found for %s: run `rose scenes FILE` again to record it", film.Bundle)
}

// isVideo is whether name has the extension of a video container.
func isVideo(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mkv", ".mp4", ".m4v", ".mov", ".webm", ".avi", ".ts", ".m2ts", ".mpg", ".mpeg", ".wmv", ".flv", ".ogv":
		return true
	}
	return false
}
