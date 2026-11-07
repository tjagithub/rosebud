package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// migrations[i] upgrades user_version i to i+1. Append only; never edit.
var migrations = []func(context.Context, *sql.Tx) error{toV1, toV2}

func toV1(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
create table films (
	id          text primary key,
	title       text    not null,
	year        integer not null,
	bundle      text    not null,
	duration_ns integer not null
);
create table scenes (
	id       integer primary key,
	film     text    not null references films(id) on delete cascade,
	start_ns integer not null,
	end_ns   integer not null,
	frame    text    not null,
	text     text    not null
);
create index scenes_film_start on scenes(film, start_ns);
create virtual table scene_fts using fts5(text, content='scenes', content_rowid='id');
create trigger scenes_ai after insert on scenes begin
	insert into scene_fts(rowid, text) values (new.id, new.text);
end;
create trigger scenes_ad after delete on scenes begin
	insert into scene_fts(scene_fts, rowid, text) values ('delete', old.id, old.text);
end;
create trigger scenes_au after update on scenes begin
	insert into scene_fts(scene_fts, rowid, text) values ('delete', old.id, old.text);
	insert into scene_fts(rowid, text) values (new.id, new.text);
end;
`)
	return err
}

// toV2 indexes folded text in trigrams. Rows are folded before the index
// exists, so no trigger removes entries that were never added.
func toV2(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
drop trigger scenes_ai;
drop trigger scenes_ad;
drop trigger scenes_au;
drop table scene_fts;
alter table scenes add column norm text not null default '';`); err != nil {
		return err
	}
	if err := refold(ctx, tx); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
create virtual table scene_fts using fts5(norm, content='scenes', content_rowid='id', tokenize='trigram');
create trigger scenes_ai after insert on scenes begin
	insert into scene_fts(rowid, norm) values (new.id, new.norm);
end;
create trigger scenes_ad after delete on scenes begin
	insert into scene_fts(scene_fts, rowid, norm) values ('delete', old.id, old.norm);
end;
create trigger scenes_au after update on scenes begin
	insert into scene_fts(scene_fts, rowid, norm) values ('delete', old.id, old.norm);
	insert into scene_fts(rowid, norm) values (new.id, new.norm);
end;
insert into scene_fts(scene_fts) values ('rebuild');`)
	return err
}

func refold(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `select id, text from scenes`)
	if err != nil {
		return err
	}
	norm := map[int64]string{}
	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			_ = rows.Close()
			return err
		}
		norm[id] = Fold(text)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for id, n := range norm {
		if _, err := tx.ExecContext(ctx, `update scenes set norm = ? where id = ?`, n, id); err != nil {
			return err
		}
	}
	return nil
}

// migrate brings the schema up to date, one transaction per version.
func migrate(ctx context.Context, db *sql.DB) error {
	var v int
	if err := db.QueryRowContext(ctx, `pragma user_version`).Scan(&v); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	if v > len(migrations) {
		return fmt.Errorf("store: database schema v%d is newer than this build (v%d)", v, len(migrations))
	}
	for ; v < len(migrations); v++ {
		if err := apply(ctx, db, v); err != nil {
			return fmt.Errorf("store: migrate to v%d: %w", v+1, err)
		}
	}
	return nil
}

func apply(ctx context.Context, db *sql.DB, v int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := migrations[v](ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`pragma user_version = %d`, v+1)); err != nil {
		return err
	}
	return tx.Commit()
}
