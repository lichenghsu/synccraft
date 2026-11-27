//go:build integration

package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// mysql-src in dev/docker-compose.yml:
//
//	podman compose -f dev/docker-compose.yml up -d mysql-src
func mysqlDSN(database string) string {
	c := mysql.NewConfig()
	c.User, c.Passwd = "root", "root"
	c.Net, c.Addr = "tcp", "127.0.0.1:3306"
	c.DBName = database
	return c.FormatDSN()
}

// openMySQL creates a throwaway database, applies ddl, and drops it on cleanup.
func openMySQL(t *testing.T, ddl ...string) *sql.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := sql.Open("mysql", mysqlDSN(""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v (is mysql-src in dev/docker-compose.yml up?)", err)
	}

	name := fmt.Sprintf("dialect_it_%d_%d", time.Now().UnixNano(), testDBSeq.Add(1))
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE " + name); err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
	})

	db, err := sql.Open("mysql", mysqlDSN(name))
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

func TestMySQLListTables(t *testing.T) {
	db := openMySQL(t,
		`CREATE TABLE users (id int)`,
		`CREATE TABLE orders (id int)`,
		`CREATE VIEW user_ids AS SELECT id FROM users`,
	)
	ctx := context.Background()

	got, err := MySQL{}.ListTables(ctx, db)
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	want := []Table{{Name: "orders"}, {Name: "users"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListTables = %+v, want %+v", got, want)
	}

	empty := openMySQL(t)
	got, err = MySQL{}.ListTables(ctx, empty)
	if err != nil {
		t.Fatalf("ListTables(empty): %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("ListTables(empty) = %#v, want empty non-nil slice", got)
	}
}

func TestMySQLColumns(t *testing.T) {
	db := openMySQL(t, `CREATE TABLE users (
		id         int unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
		name       varchar(20) NOT NULL DEFAULT 'guest',
		qty        int DEFAULT 0,
		price      decimal(10,2),
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		expr_d     int DEFAULT (1 + 1),
		quote_d    varchar(10) DEFAULT 'it''s',
		doubled    decimal(12,2) GENERATED ALWAYS AS (price * 2) STORED
	)`)

	got, err := MySQL{}.Columns(context.Background(), db, Table{Name: "users"})
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	want := []Column{
		{Name: "id", Type: "int unsigned", Nullable: false, Position: 1, AutoIncrement: true},
		{Name: "name", Type: "varchar(20)", Nullable: false, Default: ptr("'guest'"), Position: 2},
		{Name: "qty", Type: "int", Nullable: true, Default: ptr("'0'"), Position: 3},
		{Name: "price", Type: "decimal(10,2)", Nullable: true, Position: 4},
		{Name: "created_at", Type: "datetime", Nullable: true, Default: ptr("CURRENT_TIMESTAMP"), Position: 5},
		{Name: "expr_d", Type: "int", Nullable: true, Default: ptr("(1 + 1)"), Position: 6},
		{Name: "quote_d", Type: "varchar(10)", Nullable: true, Default: ptr("'it''s'"), Position: 7},
		{Name: "doubled", Type: "decimal(12,2)", Nullable: true, Position: 8},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Columns =\n%s\nwant\n%s", formatColumns(got), formatColumns(want))
	}
}

func TestMySQLPrimaryKey(t *testing.T) {
	db := openMySQL(t,
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
			got, err := MySQL{}.PrimaryKey(context.Background(), db, Table{Name: table})
			if err != nil {
				t.Fatalf("PrimaryKey: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("PrimaryKey = %#v, want %#v", got, want)
			}
		})
	}
}

func TestMySQLIndexes(t *testing.T) {
	db := openMySQL(t,
		`CREATE TABLE t (id int PRIMARY KEY, email varchar(50) UNIQUE, first varchar(20), last varchar(20), name varchar(20))`,
		`CREATE INDEX t_last_first ON t (last, first)`,
		`CREATE UNIQUE INDEX t_name ON t (name)`,
		`CREATE INDEX t_lower_name ON t ((lower(name)))`,
		`CREATE TABLE bare (id int PRIMARY KEY)`,
	)
	ctx := context.Background()

	got, err := MySQL{}.Indexes(ctx, db, Table{Name: "t"})
	if err != nil {
		t.Fatalf("Indexes: %v", err)
	}
	want := []Index{
		{Name: "email", Columns: []string{"email"}, Unique: true},
		{Name: "t_last_first", Columns: []string{"last", "first"}},
		{Name: "t_lower_name", Columns: []string{""}},
		{Name: "t_name", Columns: []string{"name"}, Unique: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Indexes =\n%+v\nwant\n%+v", got, want)
	}

	got, err = MySQL{}.Indexes(ctx, db, Table{Name: "bare"})
	if err != nil {
		t.Fatalf("Indexes(bare): %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Indexes(bare) = %#v, want empty non-nil slice", got)
	}
}

func TestMySQLTableNotFound(t *testing.T) {
	db := openMySQL(t, `CREATE TABLE t (id int)`, `CREATE VIEW v AS SELECT id FROM t`)

	assertNotFound(t, MySQL{}, db, Table{Name: "missing"})
	assertNotFound(t, MySQL{}, db, Table{Name: "v"}) // a view is not a table
	assertNotFound(t, MySQL{}, db, Table{Schema: "nope", Name: "t"})
}

func TestMySQLCanceled(t *testing.T) {
	db := openMySQL(t, `CREATE TABLE t (id int PRIMARY KEY)`)
	assertCanceled(t, MySQL{}, db, Table{Name: "t"})
}
