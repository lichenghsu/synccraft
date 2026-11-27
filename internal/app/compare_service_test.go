package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/lichenghsu/synccraft/internal/conn"
	"github.com/lichenghsu/synccraft/internal/schema"
	"github.com/lichenghsu/synccraft/internal/store"
)

type compareFixture struct {
	svc            *CompareService
	st             *store.Store
	source, target Side
	dstPath        string
}

func sqliteFile(t *testing.T, path string, ddl ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func newCompareFixture(t *testing.T, srcDDL, dstDDL []string) compareFixture {
	t.Helper()
	keyring.MockInit()
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "config", "connections.json"), store.Keyring{Service: "synccraft-test"})
	save := func(name string, ddl []string) (Side, string) {
		path := filepath.Join(dir, name+".db")
		sqliteFile(t, path, ddl...)
		c, err := st.Save(store.Connection{Name: name, Config: conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return Side{ConnectionID: c.ID}, path
	}
	src, _ := save("src", srcDDL)
	dst, dstPath := save("dst", dstDDL)
	pool := conn.NewPool()
	t.Cleanup(func() { pool.CloseAll() })
	return compareFixture{svc: NewCompareService(st, pool), st: st, source: src, target: dst, dstPath: dstPath}
}

func ids(steps []schema.Step, keep func(schema.Change) bool) []string {
	out := []string{}
	for _, s := range steps {
		if keep(s.Change) {
			out = append(out, s.Change.ID)
		}
	}
	return out
}

func all(schema.Change) bool { return true }

func TestCompareApply(t *testing.T) {
	f := newCompareFixture(t,
		[]string{
			`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL, nick TEXT)`,
			`CREATE INDEX ix_email ON users (email)`,
			`CREATE TABLE orders (id INTEGER PRIMARY KEY)`,
		},
		[]string{
			`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL, legacy TEXT)`,
			`CREATE TABLE audit (id INTEGER)`,
		},
	)
	ctx := context.Background()

	cmp, err := f.svc.Compare(ctx, f.source, f.target)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.TransactionalDDL || cmp.Token == "" || len(cmp.Steps) != 5 {
		t.Fatalf("Compare = %+v", cmp)
	}

	// Leave the destructive changes (drop column, drop table) out.
	safe := ids(cmp.Steps, func(c schema.Change) bool { return !c.Destructive })
	preview, err := f.svc.Preview(cmp.Token, safe)
	if err != nil || len(preview) != 3 {
		t.Fatalf("Preview = %+v, %v", preview, err)
	}
	if err := f.svc.Apply(ctx, cmp.Token, safe); err != nil {
		t.Fatal(err)
	}

	// The token is spent; the remaining differences are the destructive ones.
	if _, err := f.svc.Preview(cmp.Token, nil); !errors.Is(err, ErrComparisonExpired) {
		t.Errorf("Preview after Apply: %v, want ErrComparisonExpired", err)
	}
	again, err := f.svc.Compare(ctx, f.source, f.target)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range again.Steps {
		if !s.Change.Destructive {
			t.Errorf("left after Apply: %s", s.Change.ID)
		}
	}
	if len(again.Steps) != 2 {
		t.Errorf("left %d changes, want 2", len(again.Steps))
	}
}

func TestCompareApplyRefusesChangedSchema(t *testing.T) {
	f := newCompareFixture(t,
		[]string{`CREATE TABLE t (id INTEGER, a TEXT)`},
		[]string{`CREATE TABLE t (id INTEGER)`},
	)
	ctx := context.Background()
	cmp, err := f.svc.Compare(ctx, f.source, f.target)
	if err != nil {
		t.Fatal(err)
	}
	sqliteFile(t, f.dstPath, `CREATE TABLE sneaky (x INTEGER)`)

	if err := f.svc.Apply(ctx, cmp.Token, ids(cmp.Steps, all)); !errors.Is(err, ErrSchemaChanged) {
		t.Fatalf("Apply = %v, want ErrSchemaChanged", err)
	}
	db, _ := sql.Open("sqlite", f.dstPath)
	defer db.Close()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('t') WHERE name = 'a'`).Scan(&n)
	if n != 0 {
		t.Error("Apply ran despite the changed schema")
	}
}

func TestCompareErrors(t *testing.T) {
	f := newCompareFixture(t, []string{`CREATE TABLE t (id INTEGER)`}, nil)
	ctx := context.Background()

	if _, err := f.svc.Preview("nope", nil); !errors.Is(err, ErrComparisonExpired) {
		t.Errorf("Preview(bad token) = %v", err)
	}
	cmp, err := f.svc.Compare(ctx, f.source, f.target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Preview(cmp.Token, []string{"made up"}); !errors.Is(err, ErrUnknownChange) {
		t.Errorf("Preview(unknown id) = %v", err)
	}
	if err := f.svc.Apply(ctx, cmp.Token, nil); err == nil {
		t.Error("Apply with no selection succeeded")
	}

	// A newer comparison expires the older token.
	if _, err := f.svc.Compare(ctx, f.source, f.target); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Apply(ctx, cmp.Token, ids(cmp.Steps, all)); !errors.Is(err, ErrComparisonExpired) {
		t.Errorf("Apply(old token) = %v", err)
	}

	pg, err := f.st.Save(store.Connection{Name: "pg", Config: conn.ConnectionConfig{
		Type: conn.PostgreSQL, Host: "127.0.0.1", Port: 1, User: "u", Database: "d",
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Port 1 refuses connections: the mismatch must be found before connecting.
	if _, err := f.svc.Compare(ctx, f.source, Side{ConnectionID: pg.ID}); !errors.Is(err, ErrEngineMismatch) {
		t.Errorf("Compare(SQLite, PostgreSQL) = %v, want ErrEngineMismatch", err)
	}
	if _, err := f.svc.Compare(ctx, f.source, Side{ConnectionID: "missing"}); err == nil {
		t.Error("Compare against a missing connection succeeded")
	}
}
