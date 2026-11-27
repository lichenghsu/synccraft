package schema

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/lichenghsu/synccraft/internal/dialect"
	_ "modernc.org/sqlite"
)

func openSQLite(t *testing.T, ddl ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return db
}

func TestLoad(t *testing.T) {
	ctx := context.Background()
	empty, err := Load(ctx, dialect.SQLite{}, openSQLite(t))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("Load(empty) = %v, %v; want empty slice", empty, err)
	}

	db := openSQLite(t,
		`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
		`CREATE INDEX ix_email ON users (email)`,
		`CREATE TABLE notes (body TEXT)`,
	)
	got, err := Load(ctx, dialect.SQLite{}, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Table.Name != "notes" || got[1].Table.Name != "users" {
		t.Fatalf("Load tables = %+v", got)
	}
	u := got[1]
	if len(u.Columns) != 2 || len(u.PrimaryKey) != 1 || len(u.Indexes) != 1 {
		t.Errorf("users = %+v", u)
	}
	if got[0].PrimaryKey == nil || got[0].Indexes == nil {
		t.Errorf("notes key/indexes should be empty, not nil: %+v", got[0])
	}

	// Comparing a database with itself finds nothing.
	if d := Diff(got, got); len(d) != 0 {
		t.Errorf("Diff(self) = %+v", d)
	}
}
