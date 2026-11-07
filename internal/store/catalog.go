package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"
	"unicode/utf8"

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
		`insert into scenes(film, start_ns, end_ns, frame, text, norm) values (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = ins.Close() }()
	for _, sc := range scenes {
		if _, err := ins.ExecContext(ctx, film, int64(sc.Start), int64(sc.End), sc.Frame, sc.Text, Fold(sc.Text)); err != nil {
			return fmt.Errorf("store: scenes for film %q: %w", film, err)
		}
	}
	return tx.Commit()
}

// SetText sets the spoken text of a film's scenes, keeping the scenes themselves.
// It is all or nothing: a scene that is not this film's fails the whole call.
func (s *Store) SetText(ctx context.Context, film core.FilmID, text map[core.SceneID]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	upd, err := tx.PrepareContext(ctx, `update scenes set text = ?, norm = ? where id = ? and film = ?`)
	if err != nil {
		return err
	}
	defer func() { _ = upd.Close() }()
	for id, t := range text {
		res, err := upd.ExecContext(ctx, t, Fold(t), id, film)
		if err != nil {
			return fmt.Errorf("store: text for film %q: %w", film, err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("store: scene %d is not in film %q", id, film)
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

// Search finds scenes whose folded text holds every word, best first. Words
// under three characters, which the trigram index cannot see, match as substrings.
func (s *Store) Search(ctx context.Context, q core.SearchQuery) ([]core.Hit, error) {
	var long, short []string
	for _, w := range strings.Fields(Fold(q.Text)) {
		if utf8.RuneCountInString(w) < 3 {
			short = append(short, w)
		} else {
			long = append(long, w)
		}
	}
	if len(long)+len(short) == 0 {
		return nil, errors.New("store: empty search text")
	}
	if q.Limit <= 0 {
		return nil, fmt.Errorf("store: search limit must be positive, got %d", q.Limit)
	}

	stmt := `select s.id, s.film, s.start_ns, s.end_ns, s.frame, s.text, `
	order := `s.id`
	var args []any
	if len(long) > 0 {
		stmt += `-bm25(scene_fts) from scene_fts join scenes s on s.id = scene_fts.rowid where scene_fts match ?`
		order = `bm25(scene_fts), s.id`
		args = append(args, matchExpr(long))
	} else {
		stmt += `0.0 from scenes s where 1`
	}
	stmt += ` and (? = '' or s.film = ?)`
	args = append(args, q.Film, q.Film)
	for _, w := range short {
		stmt += ` and s.norm like ? escape '\'`
		args = append(args, "%"+likeEscaper.Replace(w)+"%")
	}
	stmt += ` order by ` + order + ` limit ?`
	args = append(args, q.Limit)

	rows := query(ctx, s.db, func(r *sql.Rows) (h core.Hit, err error) {
		var start, end int64
		err = r.Scan(&h.Scene.ID, &h.Scene.Film, &start, &end, &h.Scene.Frame, &h.Text, &h.Score)
		h.Scene.Start, h.Scene.End = time.Duration(start), time.Duration(end)
		return h, err
	}, stmt, args...)

	var hits []core.Hit
	for h, err := range rows {
		if err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, nil
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// matchExpr quotes each word so user text can never be FTS5 syntax.
func matchExpr(words []string) string {
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " ")
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
