package dialect

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

// openSQLite returns a fresh database file with ddl applied.
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

func TestSQLiteListTables(t *testing.T) {
	tests := []struct {
		name string
		ddl  []string
		want []Table
	}{
		{
			name: "empty database returns empty slice, not nil",
			want: []Table{},
		},
		{
			name: "tables sorted by name",
			ddl: []string{
				`CREATE TABLE users (id INTEGER PRIMARY KEY)`,
				`CREATE TABLE orders (id INTEGER PRIMARY KEY)`,
			},
			want: []Table{{Name: "orders"}, {Name: "users"}},
		},
		{
			name: "views, indexes and sqlite_ internal tables excluded",
			ddl: []string{
				// AUTOINCREMENT makes SQLite create sqlite_sequence.
				`CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)`,
				`CREATE INDEX items_name ON items (name)`,
				`CREATE VIEW item_names AS SELECT name FROM items`,
			},
			want: []Table{{Name: "items"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openSQLite(t, tt.ddl...)

			got, err := SQLite{}.ListTables(context.Background(), db)
			if err != nil {
				t.Fatalf("ListTables: %v", err)
			}
			if got == nil {
				t.Fatal("ListTables returned nil, want non-nil slice")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ListTables = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSQLiteListTablesCanceled(t *testing.T) {
	db := openSQLite(t, `CREATE TABLE t (id INTEGER)`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (SQLite{}).ListTables(ctx, db); err == nil {
		t.Fatal("ListTables with canceled context: want error, got nil")
	}
}

func ptr(s string) *string { return &s }

func TestSQLiteColumns(t *testing.T) {
	tests := []struct {
		name  string
		ddl   []string
		table string
		want  []Column
	}{
		{
			name: "types, nullability, defaults and positions",
			ddl: []string{`CREATE TABLE users (
				id         INTEGER NOT NULL,
				name       VARCHAR(20) NOT NULL DEFAULT 'guest',
				age        INTEGER,
				created_at TEXT DEFAULT CURRENT_TIMESTAMP,
				note
			)`},
			table: "users",
			want: []Column{
				{Name: "id", Type: "INTEGER", Nullable: false, Default: nil, Position: 1},
				{Name: "name", Type: "VARCHAR(20)", Nullable: false, Default: ptr("'guest'"), Position: 2},
				{Name: "age", Type: "INTEGER", Nullable: true, Default: nil, Position: 3},
				{Name: "created_at", Type: "TEXT", Nullable: true, Default: ptr("CURRENT_TIMESTAMP"), Position: 4},
				{Name: "note", Type: "", Nullable: true, Default: nil, Position: 5},
			},
		},
		{
			name:  "table name with a quote is passed as a parameter, not concatenated",
			ddl:   []string{`CREATE TABLE "it's" (a TEXT)`},
			table: "it's",
			want:  []Column{{Name: "a", Type: "TEXT", Nullable: true, Position: 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openSQLite(t, tt.ddl...)

			got, err := SQLite{}.Columns(context.Background(), db, Table{Name: tt.table})
			if err != nil {
				t.Fatalf("Columns: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Columns =\n%s\nwant\n%s", formatColumns(got), formatColumns(tt.want))
			}
		})
	}
}

func TestSQLiteColumnsTableNotFound(t *testing.T) {
	db := openSQLite(t)

	_, err := SQLite{}.Columns(context.Background(), db, Table{Name: "missing"})
	if !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("Columns on missing table: err = %v, want ErrTableNotFound", err)
	}
}

func TestSQLiteColumnsCanceled(t *testing.T) {
	db := openSQLite(t, `CREATE TABLE t (id INTEGER)`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := SQLite{}.Columns(ctx, db, Table{Name: "t"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Columns with canceled context: err = %v, want context.Canceled", err)
	}
}

// formatColumns prints Default by value so failures are readable.
func formatColumns(cols []Column) string {
	s := ""
	for _, c := range cols {
		def := "<nil>"
		if c.Default != nil {
			def = *c.Default
		}
		s += fmt.Sprintf("  {Name:%q Type:%q Nullable:%v Default:%s Position:%d}\n",
			c.Name, c.Type, c.Nullable, def, c.Position)
	}
	return s
}
