package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/lichenghsu/synccraft/internal/conn"
	"github.com/lichenghsu/synccraft/internal/dialect"
	"github.com/lichenghsu/synccraft/internal/store"
)

// newTestSchemaService saves a SQLite connection to a database built from ddl
// and returns the service and the saved connection's id.
func newTestSchemaService(t *testing.T, ddl ...string) (*SchemaService, string) {
	t.Helper()
	keyring.MockInit()
	dir := t.TempDir()
	path := filepath.Join(dir, "shop.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	db.Close()

	st := store.New(filepath.Join(dir, "config", "connections.json"), store.Keyring{Service: "synccraft-test"})
	saved, err := st.Save(store.Connection{Name: "shop", Config: conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pool := conn.NewPool()
	t.Cleanup(func() { pool.CloseAll() })
	return NewSchemaService(st, pool), saved.ID
}

func TestSchemaServiceSQLite(t *testing.T) {
	s, id := newTestSchemaService(t,
		`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE)`,
		`INSERT INTO items (name) VALUES ('a'), ('b'), ('c')`,
	)
	ctx := context.Background()
	items := dialect.Table{Name: "items"}

	// The database argument is ignored for SQLite.
	tables, err := s.ListTables(ctx, id, "other")
	if err != nil || !reflect.DeepEqual(tables, []dialect.Table{items}) {
		t.Fatalf("ListTables() = %v, %v", tables, err)
	}

	info, err := s.DescribeTable(ctx, id, "", items)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Columns) != 2 || !reflect.DeepEqual(info.PrimaryKey, []string{"id"}) ||
		len(info.Indexes) != 1 || !info.Indexes[0].Unique {
		t.Errorf("DescribeTable() = %+v", info)
	}

	page, err := s.PreviewRows(ctx, id, "", items, 1, 1)
	if err != nil || len(page.Rows) != 1 || page.Rows[0][1] != "b" {
		t.Errorf("PreviewRows() = %+v, %v", page, err)
	}

	n, err := s.CountRows(ctx, id, "", items)
	if err != nil || n != 3 {
		t.Errorf("CountRows() = %d, %v; want 3", n, err)
	}

	if s.pool.Len() != 1 {
		t.Errorf("pool.Len() = %d, want 1 shared connection", s.pool.Len())
	}
}

func TestSchemaServicePageSizeCapped(t *testing.T) {
	s, id := newTestSchemaService(t,
		`CREATE TABLE n (v INTEGER)`,
		`WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c WHERE x < 600) INSERT INTO n SELECT x FROM c`,
	)
	page, err := s.PreviewRows(context.Background(), id, "", dialect.Table{Name: "n"}, 0, 10_000)
	if err != nil || len(page.Rows) != MaxPageSize {
		t.Errorf("PreviewRows(limit 10000) = %d rows, %v; want %d", len(page.Rows), err, MaxPageSize)
	}
}

func TestSchemaServiceErrors(t *testing.T) {
	s, id := newTestSchemaService(t)
	ctx := context.Background()

	if _, err := s.ListTables(ctx, "nope", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ListTables(unknown id) = %v, want store.ErrNotFound", err)
	}
	if _, err := s.DescribeTable(ctx, id, "", dialect.Table{Name: "missing"}); !errors.Is(err, dialect.ErrTableNotFound) {
		t.Errorf("DescribeTable(missing) = %v, want ErrTableNotFound", err)
	}
}
