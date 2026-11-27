//go:build integration

package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

// checkReadRows expects tbl to hold (id, name, big) rows 1..3 named a, b, c
// with big = 2^53+1, plus a table "nokey" with two rows and no primary key.
func checkReadRows(t *testing.T, d Dialect, db *sql.DB, tbl, nokey Table) {
	t.Helper()
	ctx := context.Background()

	page, err := ReadRows(ctx, d, db, tbl, 1, 1)
	if err != nil {
		t.Fatalf("ReadRows() = %v", err)
	}
	if !page.Ordered || len(page.Rows) != 1 || len(page.Columns) != 3 {
		t.Fatalf("ReadRows() = %#v, want one ordered row of 3 columns", page)
	}
	row := page.Rows[0]
	if got := fmt.Sprint(row[0], " ", row[1], " ", row[2]); got != "2 b 9007199254740993" {
		t.Errorf("row = %q, want %q", got, "2 b 9007199254740993")
	}
	if _, ok := row[2].(string); !ok {
		t.Errorf("big = %#v, want a string (past 2^53)", row[2])
	}

	n, err := CountRows(ctx, d, db, tbl)
	if err != nil || n != 3 {
		t.Errorf("CountRows() = %d, %v; want 3", n, err)
	}

	page, err = ReadRows(ctx, d, db, nokey, 0, 10)
	if err != nil || page.Ordered || len(page.Rows) != 2 {
		t.Errorf("ReadRows(nokey) = %#v, %v; want 2 unordered rows", page, err)
	}
}

const rowsInsert = ` VALUES (3, 'c', 9007199254740993), (1, 'a', 9007199254740993), (2, 'b', 9007199254740993)`

func TestPGReadRows(t *testing.T) {
	db := openPG(t,
		`CREATE TABLE "Mixed Case" (id int PRIMARY KEY, name text, big bigint)`,
		`INSERT INTO "Mixed Case"`+rowsInsert,
		`CREATE TABLE nokey (v int)`,
		`INSERT INTO nokey VALUES (1), (2)`,
	)
	checkReadRows(t, PostgreSQL{}, db, Table{Schema: "public", Name: "Mixed Case"}, Table{Schema: "public", Name: "nokey"})
}

func TestMySQLReadRows(t *testing.T) {
	db := openMySQL(t,
		"CREATE TABLE `Mixed Case` (id int PRIMARY KEY, name varchar(10), big bigint)",
		"INSERT INTO `Mixed Case`"+rowsInsert,
		`CREATE TABLE nokey (v int)`,
		`INSERT INTO nokey VALUES (1), (2)`,
	)
	checkReadRows(t, MySQL{}, db, Table{Name: "Mixed Case"}, Table{Name: "nokey"})
}

func TestMSSQLReadRows(t *testing.T) {
	db := openMSSQL(t,
		`CREATE TABLE [Mixed Case] (id int PRIMARY KEY, name nvarchar(10), big bigint)`,
		`INSERT INTO [Mixed Case]`+rowsInsert,
		`CREATE TABLE nokey (g uniqueidentifier)`,
		`INSERT INTO nokey VALUES ('01234567-89AB-CDEF-0123-456789ABCDEF'), (NULL)`,
	)
	checkReadRows(t, SQLServer{}, db, Table{Schema: "dbo", Name: "Mixed Case"}, Table{Schema: "dbo", Name: "nokey"})

	page, err := ReadRows(context.Background(), SQLServer{}, db, Table{Name: "nokey"}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range page.Rows {
		found = found || r[0] == "01234567-89AB-CDEF-0123-456789ABCDEF"
	}
	if !found {
		t.Errorf("rows = %#v, want the GUID in canonical form", page.Rows)
	}
}
