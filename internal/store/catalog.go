package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
)

// Store is the catalog: films, scenes and their searchable text.
type Store struct{ db *sql.DB }

// SceneText is a scene with the line spoken in it. Scene.ID and Scene.Film
// are ignored on write.
type SceneText struct {
	core.Scene
	Text string
}

// New migrates db to the current schema and returns a Store on it.
func New(ctx context.Context, db *sql.DB) (*Store, error) {
	if err := migrate(ctx, db); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// PutFilm inserts a film or updates the one with the same ID.
func (s *Store) PutFilm(ctx context.Context, f core.Film) error {
	_, err := s.db.ExecContext(ctx, `
		insert into films(id, title, year, bundle, duration_ns) values (?, ?, ?, ?, ?)
		on conflict(id) do update set
			title = excluded.title, year = excluded.year,
			bundle = excluded.bundle, duration_ns = excluded.duration_ns`,
		f.ID, f.Title, f.Year, f.Bundle, int64(f.Duration))
	return err
}

// DeleteFilm removes a film with its scenes and search entries.
func (s *Store) DeleteFilm(ctx context.Context, id core.FilmID) error {
	_, err := s.db.ExecContext(ctx, `delete from films where id = ?`, id)
	return err
}

// ReplaceScenes swaps all of a film's scenes for the given ones, atomically.
func (s *Store) ReplaceScenes(ctx context.Context, film core.FilmID, scenes []SceneText) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `delete from scenes where film = ?`, film); err != nil {
		return err
	}
	ins, err := tx.PrepareContext(ctx,
		`insert into scenes(film, start_ns, end_ns, frame, text) values (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = ins.Close() }()
	for _, sc := range scenes {
		if _, err := ins.ExecContext(ctx, film, int64(sc.Start), int64(sc.End), sc.Frame, sc.Text); err != nil {
			return fmt.Errorf("store: scenes for film %q: %w", film, err)
		}
	}
	return tx.Commit()
}

// Films lists films by title, then ID.
func (s *Store) Films(ctx context.Context) iter.Seq2[core.Film, error] {
	return query(ctx, s.db, func(r *sql.Rows) (f core.Film, err error) {
		var dur int64
		err = r.Scan(&f.ID, &f.Title, &f.Year, &f.Bundle, &dur)
		f.Duration = time.Duration(dur)
		return f, err
	}, `select id, title, year, bundle, duration_ns from films order by title, id`)
}

// Scenes lists a film's scenes in time order.
func (s *Store) Scenes(ctx context.Context, film core.FilmID) iter.Seq2[core.Scene, error] {
	return query(ctx, s.db, func(r *sql.Rows) (sc core.Scene, err error) {
		var start, end int64
		err = r.Scan(&sc.ID, &sc.Film, &start, &end, &sc.Frame)
		sc.Start, sc.End = time.Duration(start), time.Duration(end)
		return sc, err
	}, `select id, film, start_ns, end_ns, frame from scenes where film = ? order by start_ns, id`, film)
}

// Search runs a full-text match, best first. Every word must match.
func (s *Store) Search(ctx context.Context, q core.SearchQuery) ([]core.Hit, error) {
	match := matchExpr(q.Text)
	if match == "" {
		return nil, errors.New("store: empty search text")
	}
	if q.Limit <= 0 {
		return nil, fmt.Errorf("store: search limit must be positive, got %d", q.Limit)
	}
	rows := query(ctx, s.db, func(r *sql.Rows) (h core.Hit, err error) {
		var start, end int64
		err = r.Scan(&h.Scene.ID, &h.Scene.Film, &start, &end, &h.Scene.Frame, &h.Text, &h.Score)
		h.Scene.Start, h.Scene.End = time.Duration(start), time.Duration(end)
		return h, err
	}, `select s.id, s.film, s.start_ns, s.end_ns, s.frame, s.text, -bm25(scene_fts)
		from scene_fts join scenes s on s.id = scene_fts.rowid
		where scene_fts match ? and (? = '' or s.film = ?)
		order by bm25(scene_fts), s.id limit ?`, match, q.Film, q.Film, q.Limit)

	var hits []core.Hit
	for h, err := range rows {
		if err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, nil
}

// matchExpr quotes each word so user text can never be FTS5 syntax.
func matchExpr(text string) string {
	words := strings.Fields(text)
	for i, w := range words {
		words[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}
	return strings.Join(words, " ")
}

// query streams rows through scan, stopping on error or when the caller does.
func query[T any](ctx context.Context, db *sql.DB, scan func(*sql.Rows) (T, error), q string, args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			yield(zero, err)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			v, err := scan(rows)
			if !yield(v, err) || err != nil {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(zero, err)
		}
	}
}
