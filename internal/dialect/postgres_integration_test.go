//go:build integration

package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// pg-src in dev/docker-compose.yml. Run:
//
//	podman compose -f dev/docker-compose.yml up -d pg-src
//	go test -tags integration ./internal/dialect/...
func pgURL(database string) string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("postgres", "postgres"),
		Host:     "127.0.0.1:5432",
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

var testDBSeq atomic.Int64

// openPG creates a throwaway database, applies ddl, and drops it on cleanup.
func openPG(t *testing.T, ddl ...string) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := sql.Open("pgx", pgURL("postgres"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v (is pg-src in dev/docker-compose.yml up?)", err)
	}

	name := fmt.Sprintf("dialect_it_%d_%d", time.Now().UnixNano(), testDBSeq.Add(1))
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`); err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
	})

	db, err := sql.Open("pgx", pgURL(name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range ddl {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return db
}

func TestPGListTables(t *testing.T) {
	db := openPG(t,
		`CREATE TABLE users (id int)`,
		`CREATE SCHEMA audit`,
		`CREATE TABLE audit."UserLog" (id int)`,
		`CREATE VIEW user_ids AS SELECT id FROM users`,
		`CREATE TABLE events (at date) PARTITION BY RANGE (at)`,
		`CREATE TABLE events_2026 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`,
	)

	got, err := PostgreSQL{}.ListTables(context.Background(), db)
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	want := []Table{
		{Schema: "audit", Name: "UserLog"},
		{Schema: "public", Name: "events"},
		{Schema: "public", Name: "users"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListTables = %+v, want %+v", got, want)
	}
}

func TestPGListTablesEmpty(t *testing.T) {
	db := openPG(t)

	got, err := PostgreSQL{}.ListTables(context.Background(), db)
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("ListTables = %#v, want empty non-nil slice", got)
	}
}

func TestPGColumns(t *testing.T) {
	tests := []struct {
		name  string
		ddl   []string
		table Table
		want  []Column
	}{
		{
			name: "full types, defaults, serial, identity and generated columns",
			ddl: []string{`CREATE TABLE users (
				id         serial,
				ident      int GENERATED ALWAYS AS IDENTITY,
				name       varchar(20) NOT NULL DEFAULT 'guest',
				price      numeric(10,2),
				created_at timestamptz DEFAULT now(),
				doubled    numeric GENERATED ALWAYS AS (price * 2) STORED
			)`},
			table: Table{Schema: "public", Name: "users"},
			want: []Column{
				{Name: "id", Type: "integer", Nullable: false, Default: ptr("nextval('users_id_seq'::regclass)"), Position: 1},
				{Name: "ident", Type: "integer", Nullable: false, Position: 2},
				{Name: "name", Type: "character varying(20)", Nullable: false, Default: ptr("'guest'::character varying"), Position: 3},
				{Name: "price", Type: "numeric(10,2)", Nullable: true, Position: 4},
				{Name: "created_at", Type: "timestamp with time zone", Nullable: true, Default: ptr("now()"), Position: 5},
				{Name: "doubled", Type: "numeric", Nullable: true, Position: 6},
			},
		},
		{
			name: "dropped columns skipped; positions renumbered without gaps",
			ddl: []string{
				`CREATE TABLE t (a int, b int, c int)`,
				`ALTER TABLE t DROP COLUMN b`,
			},
			table: Table{Schema: "public", Name: "t"},
			want: []Column{
				{Name: "a", Type: "integer", Nullable: true, Position: 1},
				{Name: "c", Type: "integer", Nullable: true, Position: 2},
			},
		},
		{
			name: "mixed-case schema and table",
			ddl: []string{
				`CREATE SCHEMA "Audit"`,
				`CREATE TABLE "Audit"."UserLog" ("Id" int)`,
			},
			table: Table{Schema: "Audit", Name: "UserLog"},
			want:  []Column{{Name: "Id", Type: "integer", Nullable: true, Position: 1}},
		},
		{
			name:  "empty schema means current schema",
			ddl:   []string{`CREATE TABLE t (a int)`},
			table: Table{Name: "t"},
			want:  []Column{{Name: "a", Type: "integer", Nullable: true, Position: 1}},
		},
		{
			name:  "zero-column table returns empty slice",
			ddl:   []string{`CREATE TABLE t ()`},
			table: Table{Schema: "public", Name: "t"},
			want:  []Column{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openPG(t, tt.ddl...)

			got, err := PostgreSQL{}.Columns(context.Background(), db, tt.table)
			if err != nil {
				t.Fatalf("Columns: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Columns =\n%s\nwant\n%s", formatColumns(got), formatColumns(tt.want))
			}
		})
	}
}

func TestPGPrimaryKey(t *testing.T) {
	db := openPG(t,
		`CREATE TABLE single (id int PRIMARY KEY)`,
		`CREATE TABLE composite (a int, b int, PRIMARY KEY (b, a))`,
		`CREATE TABLE nokey (v int)`,
	)
	tests := map[string][]string{
		"single":    {"id"},
		"composite": {"b", "a"},
		"nokey":     {},
	}

	for table, want := range tests {
		t.Run(table, func(t *testing.T) {
			got, err := PostgreSQL{}.PrimaryKey(context.Background(), db, Table{Schema: "public", Name: table})
			if err != nil {
				t.Fatalf("PrimaryKey: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("PrimaryKey = %#v, want %#v", got, want)
			}
		})
	}
}

func TestPGIndexes(t *testing.T) {
	db := openPG(t,
		`CREATE TABLE t (id int PRIMARY KEY, email text UNIQUE, first text, last text, name text, note text)`,
		`CREATE INDEX t_last_first ON t (last, first)`,
		`CREATE UNIQUE INDEX t_name ON t (name)`,
		`CREATE INDEX t_lower_name ON t (lower(name))`,
		`CREATE INDEX t_first_incl ON t (first) INCLUDE (note)`,
		`CREATE TABLE empty (id int PRIMARY KEY)`,
	)

	got, err := PostgreSQL{}.Indexes(context.Background(), db, Table{Schema: "public", Name: "t"})
	if err != nil {
		t.Fatalf("Indexes: %v", err)
	}
	want := []Index{
		{Name: "t_email_key", Columns: []string{"email"}, Unique: true, Constraint: true},
		{Name: "t_first_incl", Columns: []string{"first"}},
		{Name: "t_last_first", Columns: []string{"last", "first"}},
		{Name: "t_lower_name", Columns: []string{""}},
		{Name: "t_name", Columns: []string{"name"}, Unique: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Indexes =\n%+v\nwant\n%+v", got, want)
	}

	got, err = PostgreSQL{}.Indexes(context.Background(), db, Table{Schema: "public", Name: "empty"})
	if err != nil {
		t.Fatalf("Indexes(empty): %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Indexes(empty) = %#v, want empty non-nil slice", got)
	}
}

func TestPGTableNotFound(t *testing.T) {
	db := openPG(t, `CREATE TABLE t (id int)`, `CREATE VIEW v AS SELECT id FROM t`)

	assertNotFound(t, PostgreSQL{}, db, Table{Schema: "public", Name: "missing"})
	assertNotFound(t, PostgreSQL{}, db, Table{Schema: "nope", Name: "t"})
	assertNotFound(t, PostgreSQL{}, db, Table{Schema: "public", Name: "v"}) // a view is not a table
	assertNotFound(t, PostgreSQL{}, db, Table{Schema: "public", Name: "T"}) // names are case-sensitive
}

func TestPGCanceled(t *testing.T) {
	db := openPG(t, `CREATE TABLE t (id int PRIMARY KEY)`)
	assertCanceled(t, PostgreSQL{}, db, Table{Schema: "public", Name: "t"})
}
