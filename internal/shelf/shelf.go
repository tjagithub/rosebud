// Package shelf presents several film bundles as one library.
package shelf

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/limit"
	"github.com/tjagithub/rosebud/internal/store"
)

// Shelf reads the catalogs of a set of bundles. It answers the read side of
// core.Client that the browser needs.
type Shelf struct {
	dbs   []*sql.DB
	books []*store.Store
}

// statTimeout bounds checking a path, in case a stalled mount hangs it.
const statTimeout = 15 * time.Second

// Open finds the bundles at path: path itself if it is a bundle, otherwise
// each NAME.film directly inside it. It fails if there are none.
func Open(ctx context.Context, path string) (*Shelf, error) {
	dirs, err := find(path)
	if err != nil {
		return nil, err
	}
	s := &Shelf{}
	for _, d := range dirs {
		db, err := store.Open(filepath.Join(d, "rose.db"))
		if err != nil {
			_ = s.Close()
			return nil, err
		}
		st, err := store.New(ctx, db)
		if err != nil {
			_ = db.Close()
			_ = s.Close()
			return nil, fmt.Errorf("%s: %w", d, err)
		}
		s.dbs, s.books = append(s.dbs, db), append(s.books, st)
	}
	return s, nil
}

// find lists the bundle directories at path.
func find(path string) ([]string, error) {
	has := func(dir string) bool {
		_, err := statWithin(filepath.Join(dir, "rose.db"))
		return err == nil
	}
	if has(path) {
		return []string{path}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if d := filepath.Join(path, e.Name()); strings.HasSuffix(e.Name(), ".film") && has(d) {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no film bundles in %s: run `rose scenes FILE` first", path)
	}
	return out, nil
}

func statWithin(path string) (os.FileInfo, error) {
	type result struct {
		fi  os.FileInfo
		err error
	}
	done := make(chan result, 1)
	go func() { fi, err := os.Stat(path); done <- result{fi, err} }()
	select {
	case r := <-done:
		return r.fi, r.err
	case <-time.After(statTimeout):
		return nil, &limit.TimeoutError{What: "checking " + path, After: statTimeout}
	}
}

// Close closes every catalog.
func (s *Shelf) Close() error {
	var errs []error
	for _, db := range s.dbs {
		errs = append(errs, db.Close())
	}
	return errors.Join(errs...)
}

// Films lists every film of every bundle, by title.
func (s *Shelf) Films(ctx context.Context) iter.Seq2[core.Film, error] {
	return func(yield func(core.Film, error) bool) {
		var all []core.Film
		for _, st := range s.books {
			for f, err := range st.Films(ctx) {
				if err != nil {
					yield(core.Film{}, err)
					return
				}
				all = append(all, f)
			}
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].Title < all[j].Title })
		for _, f := range all {
			if !yield(f, nil) {
				return
			}
		}
	}
}

// owner is the catalog that holds film, or nil.
func (s *Shelf) owner(ctx context.Context, film core.FilmID) (*store.Store, error) {
	for _, st := range s.books {
		for f, err := range st.Films(ctx) {
			if err != nil {
				return nil, err
			}
			if f.ID == film {
				return st, nil
			}
		}
	}
	return nil, fmt.Errorf("shelf: no film %q", film)
}

// Scenes lists a film's scenes in time order.
func (s *Shelf) Scenes(ctx context.Context, film core.FilmID) iter.Seq2[core.Scene, error] {
	return func(yield func(core.Scene, error) bool) {
		st, err := s.owner(ctx, film)
		if err != nil {
			yield(core.Scene{}, err)
			return
		}
		for sc, err := range st.Scenes(ctx, film) {
			if !yield(sc, err) {
				return
			}
		}
	}
}

// Search searches one film, or all of them with the best hits first.
func (s *Shelf) Search(ctx context.Context, q core.SearchQuery) ([]core.Hit, error) {
	if q.Film != "" {
		st, err := s.owner(ctx, q.Film)
		if err != nil {
			return nil, err
		}
		return st.Search(ctx, q)
	}
	var all []core.Hit
	for _, st := range s.books {
		hits, err := st.Search(ctx, q)
		if err != nil {
			return nil, err
		}
		all = append(all, hits...)
	}
	slices.SortStableFunc(all, func(a, b core.Hit) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		}
		return 0
	})
	if len(all) > q.Limit {
		all = all[:q.Limit]
	}
	return all, nil
}
