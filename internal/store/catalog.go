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
// A scene with the same span as before and no text of its own keeps its old
// text and subtitle, so a rescan does not wipe what was read since.
func (s *Store) ReplaceScenes(ctx context.Context, film core.FilmID, scenes []SceneText) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	kept, err := keptText(ctx, tx, film)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from scenes where film = ?`, film); err != nil {
		return err
	}
	ins, err := tx.PrepareContext(ctx,
		`insert into scenes(film, start_ns, end_ns, frame, text, subtitle, translation, norm) values (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = ins.Close() }()
	for _, sc := range scenes {
		text, sub, tr := sc.Text, "", ""
		if old, ok := kept[span{sc.Start, sc.End}]; ok && text == "" {
			text, sub, tr = old.text, old.sub, old.tr
		}
		if _, err := ins.ExecContext(ctx, film, int64(sc.Start), int64(sc.End), sc.Frame, text, sub, tr, foldAll(text, sub, tr)); err != nil {
			return fmt.Errorf("store: scenes for film %q: %w", film, err)
		}
	}
	return tx.Commit()
}

// SetText sets the spoken text of a film's scenes, keeping the scenes themselves.
// It is all or nothing: a scene that is not this film's fails the whole call.
func (s *Store) SetText(ctx context.Context, film core.FilmID, text map[core.SceneID]string) error {
	return s.setField(ctx, film, "text", text)
}

// SetSubtitle sets the on-screen subtitle of a film's scenes, all or nothing like SetText.
func (s *Store) SetSubtitle(ctx context.Context, film core.FilmID, sub map[core.SceneID]string) error {
	return s.setField(ctx, film, "subtitle", sub)
}

// SetTranslation sets the English translation of a film's scenes, all or nothing like SetText.
func (s *Store) SetTranslation(ctx context.Context, film core.FilmID, tr map[core.SceneID]string) error {
	return s.setField(ctx, film, "translation", tr)
}

// setField sets one text column and refolds norm from all three.
func (s *Store) setField(ctx context.Context, film core.FilmID, field string, vals map[core.SceneID]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const get = `select text, subtitle, translation from scenes where id = ? and film = ?`
	upd, err := tx.PrepareContext(ctx, `update scenes set `+field+` = ?, norm = ? where id = ? and film = ?`)
	if err != nil {
		return err
	}
	defer func() { _ = upd.Close() }()
	for id, v := range vals {
		cols := map[string]*string{}
		var text, sub, tr string
		cols["text"], cols["subtitle"], cols["translation"] = &text, &sub, &tr
		if err := tx.QueryRowContext(ctx, get, id, film).Scan(&text, &sub, &tr); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: scene %d is not in film %q", id, film)
		} else if err != nil {
			return fmt.Errorf("store: %s for film %q: %w", field, film, err)
		}
		*cols[field] = v
		if _, err := upd.ExecContext(ctx, v, foldAll(text, sub, tr), id, film); err != nil {
			return fmt.Errorf("store: %s for film %q: %w", field, film, err)
		}
	}
	return tx.Commit()
}

// foldAll is the search form of a scene's spoken text, subtitle and translation together.
func foldAll(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return Fold(strings.Join(kept, " "))
}

// SceneLines is the text of one scene.
type SceneLines struct {
	ID                          core.SceneID
	Text, Subtitle, Translation string
}

// Texts lists a film's scenes with their text, in time order.
func (s *Store) Texts(ctx context.Context, film core.FilmID) iter.Seq2[SceneLines, error] {
	return query(ctx, s.db, func(r *sql.Rows) (l SceneLines, err error) {
		err = r.Scan(&l.ID, &l.Text, &l.Subtitle, &l.Translation)
		return l, err
	}, `select id, text, subtitle, translation from scenes where film = ? order by start_ns, id`, film)
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

	stmt := `select s.id, s.film, s.start_ns, s.end_ns, s.frame, s.text, s.subtitle, s.translation, `
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
		err = r.Scan(&h.Scene.ID, &h.Scene.Film, &start, &end, &h.Scene.Frame, &h.Text, &h.Subtitle, &h.Translation, &h.Score)
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

type span struct{ start, end time.Duration }

type oldText struct{ text, sub, tr string }

// keptText reads the text of a film's scenes, keyed by their start and end.
func keptText(ctx context.Context, tx *sql.Tx, film core.FilmID) (map[span]oldText, error) {
	rows, err := tx.QueryContext(ctx, `select start_ns, end_ns, text, subtitle, translation from scenes where film = ?`, film)
	if err != nil {
		return nil, fmt.Errorf("store: scenes of film %q: %w", film, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[span]oldText{}
	for rows.Next() {
		var s, e int64
		var o oldText
		if err := rows.Scan(&s, &e, &o.text, &o.sub, &o.tr); err != nil {
			return nil, fmt.Errorf("store: scenes of film %q: %w", film, err)
		}
		out[span{time.Duration(s), time.Duration(e)}] = o
	}
	return out, rows.Err()
}
