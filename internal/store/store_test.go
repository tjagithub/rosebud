package store

import (
	"context"
	"strings"
	"testing"

	"github.com/ncruces/go-sqlite3"
)

// Fails loudly if a dependency bump breaks the wasm ABI.
func TestSmoke(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	exec := func(q string, a ...any) {
		t.Helper()
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	exec(`create virtual table lines using fts5(text)`)
	exec(`insert into lines values ('rosebud'),('kane')`)
	var n int
	if err := db.QueryRow(`select count(*) from lines where lines match 'rosebud'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("fts5: n=%d err=%v", n, err)
	}

	exec(`create virtual table v using vec0(e float[2])`)
	exec(`insert into v(rowid, e) values (1, '[1,0]'), (2, '[0,1]')`)
	var id int
	if err := db.QueryRow(`select rowid from v where e match '[1,0.1]' and k = 1`).Scan(&id); err != nil || id != 1 {
		t.Fatalf("vec: id=%d err=%v", id, err)
	}

	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	err = c.Raw(func(dc any) error {
		conn := dc.(interface{ Raw() *sqlite3.Conn }).Raw()
		return conn.CreateFunction("shout", 1, sqlite3.DETERMINISTIC, func(ctx sqlite3.Context, arg ...sqlite3.Value) {
			ctx.ResultText(strings.ToUpper(arg[0].Text()))
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	var s string
	if err := c.QueryRowContext(context.Background(), `select shout('x')`).Scan(&s); err != nil || s != "X" {
		t.Fatalf("go func: %q err=%v", s, err)
	}
}
