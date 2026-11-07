package translate

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
)

// Store is the part of the catalog the stage uses.
type Store interface {
	Texts(ctx context.Context, film core.FilmID) iter.Seq2[store.SceneLines, error]
	SetTranslation(ctx context.Context, film core.FilmID, tr map[core.SceneID]string) error
}

// Stage writes an English translation for each scene that has text: the
// on-screen subtitle when there is one, else the spoken text.
type Stage struct {
	Store      Store
	Translator Translator
}

// Run is a jobs.Stage. Translations are written once, at the end, so a failed
// or cancelled run leaves the earlier ones alone.
func (s Stage) Run(ctx context.Context, spec core.JobSpec, progress func(done, total int64)) error {
	if s.Translator == nil || s.Store == nil {
		return errors.New("translate: stage needs a Store and a Translator")
	}
	var ids []core.SceneID
	var lines []string
	for l, err := range s.Store.Texts(ctx, spec.Film) {
		if err != nil {
			return fmt.Errorf("translate: text of %q: %w", spec.Film, err)
		}
		src := strings.TrimSpace(l.Subtitle)
		if src == "" {
			src = strings.TrimSpace(l.Text)
		}
		if src != "" {
			ids, lines = append(ids, l.ID), append(lines, src)
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("translate: film %q has no text; run subs or ocr first", spec.Film)
	}
	out, err := s.Translator.Translate(WithProgress(ctx, progress), lines)
	if err != nil {
		return err
	}
	if len(out) != len(ids) {
		return fmt.Errorf("translate: got %d translations for %d lines", len(out), len(ids))
	}
	res := make(map[core.SceneID]string, len(ids))
	for i, id := range ids {
		res[id] = out[i]
	}
	return s.Store.SetTranslation(ctx, spec.Film, res)
}
