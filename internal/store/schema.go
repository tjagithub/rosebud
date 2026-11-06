package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migrations[i] upgrades user_version i to i+1. Append only; never edit.
var migrations = []string{`
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
`}

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
	if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`pragma user_version = %d`, v+1)); err != nil {
		return err
	}
	return tx.Commit()
}
