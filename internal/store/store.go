// Package store opens CGO-free SQLite with FTS5 and sqlite-vec.
// go-sqlite3 is pinned to v0.21.0: the vec wasm only matches some versions.
package store

import (
	"database/sql"

	_ "github.com/asg017/sqlite-vec-go-bindings/ncruces"
	_ "github.com/ncruces/go-sqlite3/driver"
)

// Open opens a database; use ":memory:" for tests.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
