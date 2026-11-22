//go:build integration

package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// mssql in dev/docker-compose.yml:
//
//	podman compose -f dev/docker-compose.yml up -d mssql
func mssqlURL(database string) string {
	q := url.Values{}
	q.Set("database", database)
	q.Set("TrustServerCertificate", "true")
	u := url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword("sa", "Passw0rd!"),
		Host:     "127.0.0.1:1433",
		RawQuery: q.Encode(),
	}
	return u.String()
}

// openMSSQL creates a throwaway database, applies ddl (one batch per entry),
// and drops it on cleanup.
func openMSSQL(t *testing.T, ddl ...string) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	admin, err := sql.Open("sqlserver", mssqlURL("master"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v (is mssql in dev/docker-compose.yml up?)", err)
	}

	name := fmt.Sprintf("dialect_it_%d_%d", time.Now().UnixNano(), testDBSeq.Add(1))
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := admin.Exec("ALTER DATABASE " + name + " SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE " + name)
		if err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
	})

	db, err := sql.Open("sqlserver", mssqlURL(name))
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

func TestMSSQLListTables(t *testing.T) {
	db := openMSSQL(t,
		`CREATE TABLE users (id int)`,
		`CREATE SCHEMA audit`,
		`CREATE TABLE audit.UserLog (id int)`,
		`CREATE VIEW user_ids AS SELECT id FROM users`,
	)

	got, err := SQLServer{}.ListTables(context.Background(), db)
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	want := []Table{
		{Schema: "audit", Name: "UserLog"},
		{Schema: "dbo", Name: "users"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListTables = %+v, want %+v", got, want)
	}
}

func TestMSSQLColumns(t *testing.T) {
	db := openMSSQL(t,
		`CREATE TABLE users (
			id         int IDENTITY PRIMARY KEY,
			name       nvarchar(20) NOT NULL DEFAULT 'guest',
			code       varchar(max),
			qty        int DEFAULT 0,
			price      decimal(10,2),
			created_at datetime2(3) DEFAULT getdate(),
			doubled    AS price * 2
		)`,
		`CREATE SCHEMA [My Schema]`,
		`CREATE TABLE [My Schema].[it's] (a int)`,
	)
	ctx := context.Background()

	got, err := SQLServer{}.Columns(ctx, db, Table{Name: "users"}) // empty schema = dbo
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	want := []Column{
		{Name: "id", Type: "int", Nullable: false, Position: 1},
		{Name: "name", Type: "nvarchar(20)", Nullable: false, Default: ptr("'guest'"), Position: 2},
		{Name: "code", Type: "varchar(max)", Nullable: true, Position: 3},
		{Name: "qty", Type: "int", Nullable: true, Default: ptr("0"), Position: 4},
		{Name: "price", Type: "decimal(10,2)", Nullable: true, Position: 5},
		{Name: "created_at", Type: "datetime2(3)", Nullable: true, Default: ptr("getdate()"), Position: 6},
		{Name: "doubled", Type: "decimal(12,2)", Nullable: true, Position: 7},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Columns =\n%s\nwant\n%s", formatColumns(got), formatColumns(want))
	}

	got, err = SQLServer{}.Columns(ctx, db, Table{Schema: "My Schema", Name: "it's"})
	if err != nil {
		t.Fatalf("Columns(quoted names): %v", err)
	}
	if want := []Column{{Name: "a", Type: "int", Nullable: true, Position: 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Columns(quoted names) =\n%s\nwant\n%s", formatColumns(got), formatColumns(want))
	}
}

func TestMSSQLPrimaryKey(t *testing.T) {
	db := openMSSQL(t,
		`CREATE TABLE single (id int PRIMARY KEY)`,
		`CREATE TABLE composite (a int NOT NULL, b int NOT NULL, PRIMARY KEY (b, a))`,
		`CREATE TABLE nokey (v int)`,
	)
	tests := map[string][]string{
		"single":    {"id"},
		"composite": {"b", "a"},
		"nokey":     {},
	}
	for table, want := range tests {
		t.Run(table, func(t *testing.T) {
			got, err := SQLServer{}.PrimaryKey(context.Background(), db, Table{Schema: "dbo", Name: table})
			if err != nil {
				t.Fatalf("PrimaryKey: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("PrimaryKey = %#v, want %#v", got, want)
			}
		})
	}
}

func TestMSSQLIndexes(t *testing.T) {
	db := openMSSQL(t,
		`CREATE TABLE t (id int PRIMARY KEY, email varchar(50) CONSTRAINT t_email UNIQUE, first varchar(20), last varchar(20), name varchar(20), note varchar(20))`,
		`CREATE INDEX t_last_first ON t (last, first)`,
		`CREATE UNIQUE INDEX t_name ON t (name)`,
		`CREATE INDEX t_first_incl ON t (first) INCLUDE (note)`,
		`CREATE TABLE empty (id int PRIMARY KEY)`,
	)
	ctx := context.Background()

	got, err := SQLServer{}.Indexes(ctx, db, Table{Schema: "dbo", Name: "t"})
	if err != nil {
		t.Fatalf("Indexes: %v", err)
	}
	want := []Index{
		{Name: "t_email", Columns: []string{"email"}, Unique: true, Constraint: true},
		{Name: "t_first_incl", Columns: []string{"first"}},
		{Name: "t_last_first", Columns: []string{"last", "first"}},
		{Name: "t_name", Columns: []string{"name"}, Unique: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Indexes =\n%+v\nwant\n%+v", got, want)
	}

	got, err = SQLServer{}.Indexes(ctx, db, Table{Schema: "dbo", Name: "empty"})
	if err != nil {
		t.Fatalf("Indexes(empty): %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Indexes(empty) = %#v, want empty non-nil slice", got)
	}
}

func TestMSSQLTableNotFound(t *testing.T) {
	db := openMSSQL(t, `CREATE TABLE t (id int)`, `CREATE VIEW v AS SELECT id FROM t`)

	assertNotFound(t, SQLServer{}, db, Table{Schema: "dbo", Name: "missing"})
	assertNotFound(t, SQLServer{}, db, Table{Schema: "nope", Name: "t"})
	assertNotFound(t, SQLServer{}, db, Table{Schema: "dbo", Name: "v"}) // a view is not a table
}

func TestMSSQLCanceled(t *testing.T) {
	db := openMSSQL(t, `CREATE TABLE t (id int PRIMARY KEY)`)
	assertCanceled(t, SQLServer{}, db, Table{Schema: "dbo", Name: "t"})
}
