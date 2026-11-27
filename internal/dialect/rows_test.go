package dialect

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lichenghsu/synccraft/internal/conn"
)

func TestReadRowsSQLite(t *testing.T) {
	db := openSQLite(t,
		`CREATE TABLE "odd ""name""" (b TEXT, a INTEGER, PRIMARY KEY (a, b))`,
		`INSERT INTO "odd ""name""" VALUES ('x', 2), ('y', 1), ('z', 1), ('w', 3)`,
	)
	ctx := context.Background()
	tbl := Table{Name: `odd "name"`}

	got, err := ReadRows(ctx, SQLite{}, db, tbl, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := RowPage{
		Columns: []string{"b", "a"},
		Rows:    [][]any{{"y", int64(1)}, {"z", int64(1)}, {"x", int64(2)}},
		Ordered: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("page 1 = %#v\nwant %#v", got, want)
	}

	got, err = ReadRows(ctx, SQLite{}, db, tbl, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]any{{"w", int64(3)}}; !reflect.DeepEqual(got.Rows, want) {
		t.Errorf("page 2 rows = %#v, want %#v", got.Rows, want)
	}

	got, err = ReadRows(ctx, SQLite{}, db, tbl, 10, 3)
	if err != nil || got.Rows == nil || len(got.Rows) != 0 {
		t.Errorf("past the end = %#v, %v; want empty non-nil rows", got.Rows, err)
	}
}

func TestReadRowsNoPrimaryKey(t *testing.T) {
	db := openSQLite(t,
		`CREATE TABLE log (msg TEXT, data BLOB)`,
		`INSERT INTO log VALUES ('hi', x'00ff'), (NULL, NULL)`,
	)
	got, err := ReadRows(context.Background(), SQLite{}, db, Table{Name: "log"}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ordered {
		t.Error("Ordered = true for a table without primary key")
	}
	if len(got.Rows) != 2 || got.Rows[0][1] != "0x00ff" || got.Rows[1][0] != nil {
		t.Errorf("rows = %#v", got.Rows)
	}
}

func TestReadRowsErrors(t *testing.T) {
	db := openSQLite(t, `CREATE TABLE t (id INTEGER PRIMARY KEY)`, `CREATE VIEW v AS SELECT id FROM t`)
	ctx := context.Background()

	for _, name := range []string{"missing", "v"} {
		if _, err := ReadRows(ctx, SQLite{}, db, Table{Name: name}, 0, 10); !errors.Is(err, ErrTableNotFound) {
			t.Errorf("ReadRows(%s) = %v, want ErrTableNotFound", name, err)
		}
		if _, err := CountRows(ctx, SQLite{}, db, Table{Name: name}); !errors.Is(err, ErrTableNotFound) {
			t.Errorf("CountRows(%s) = %v, want ErrTableNotFound", name, err)
		}
	}
	if _, err := ReadRows(ctx, SQLite{}, db, Table{Name: "t"}, -1, 10); err == nil {
		t.Error("negative offset accepted")
	}
	if _, err := ReadRows(ctx, SQLite{}, db, Table{Name: "t"}, 0, 0); err == nil {
		t.Error("zero limit accepted")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := ReadRows(canceled, SQLite{}, db, Table{Name: "t"}, 0, 10); !errors.Is(err, context.Canceled) {
		t.Errorf("ReadRows(canceled) = %v, want context.Canceled", err)
	}
}

func TestCountRowsSQLite(t *testing.T) {
	db := openSQLite(t, `CREATE TABLE t (id INTEGER)`, `INSERT INTO t VALUES (1), (2), (3)`)
	n, err := CountRows(context.Background(), SQLite{}, db, Table{Name: "t"})
	if err != nil || n != 3 {
		t.Errorf("CountRows() = %d, %v; want 3", n, err)
	}
}

func TestCellValue(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 600, time.UTC)
	long := strings.Repeat("é", maxCellRunes+5)
	blob := make([]byte, maxCellBytes+8)
	blob[0] = 0xff
	tests := []struct {
		name   string
		in     any
		dbType string
		want   any
	}{
		{"nil", nil, "", nil},
		{"bool", true, "", true},
		{"small int", int64(-42), "", int64(-42)},
		{"max safe int", int64(maxSafeInt), "", int64(maxSafeInt)},
		{"int past 2^53", int64(maxSafeInt + 1), "", "9007199254740992"},
		{"negative int past 2^53", int64(-maxSafeInt - 1), "", "-9007199254740992"},
		{"float", 1.5, "", 1.5},
		{"float32", float32(0.25), "", 0.25},
		{"NaN", math.NaN(), "", "NaN"},
		{"+Inf", math.Inf(1), "", "+Inf"},
		{"time", ts, "", "2026-01-02T03:04:05.0000006Z"},
		{"string", "abc", "", "abc"},
		{"long string cut", long, "", strings.Repeat("é", maxCellRunes) + "…"},
		{"text bytes", []byte("héllo"), "", "héllo"},
		{"binary bytes", []byte{0xff, 0x00}, "", "0xff00"},
		{"long binary cut", blob, "", "0xff" + strings.Repeat("00", maxCellBytes-1) + "… (40 bytes)"},
		{"mssql guid", []byte{0x67, 0x45, 0x23, 0x01, 0xab, 0x89, 0xef, 0xcd, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}, "UNIQUEIDENTIFIER", "01234567-89AB-CDEF-0123-456789ABCDEF"},
		{"other type", struct{ A int }{1}, "", "{1}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cellValue(tt.in, tt.dbType); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cellValue(%#v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestQuoteAndPage(t *testing.T) {
	tbl := Table{Schema: "s", Name: "t"}
	tests := []struct {
		d       Dialect
		ident   string
		quoted  string
		ordered string
		plain   string
	}{
		{SQLite{}, `a"b`, `"a""b"`,
			`SELECT c FROM "s"."t" ORDER BY "id" LIMIT 10 OFFSET 20`,
			`SELECT c FROM "s"."t" LIMIT 10 OFFSET 20`},
		{PostgreSQL{}, `a"b`, `"a""b"`,
			`SELECT c FROM "s"."t" ORDER BY "id" LIMIT 10 OFFSET 20`,
			`SELECT c FROM "s"."t" LIMIT 10 OFFSET 20`},
		{MySQL{}, "a`b", "`a``b`",
			"SELECT c FROM `s`.`t` ORDER BY `id` LIMIT 10 OFFSET 20",
			"SELECT c FROM `s`.`t` LIMIT 10 OFFSET 20"},
		{SQLServer{}, "a]b", "[a]]b]",
			`SELECT c FROM [s].[t] ORDER BY [id] OFFSET 20 ROWS FETCH NEXT 10 ROWS ONLY`,
			`SELECT c FROM [s].[t] ORDER BY (SELECT NULL) OFFSET 20 ROWS FETCH NEXT 10 ROWS ONLY`},
	}
	for _, tt := range tests {
		name := reflect.TypeOf(tt.d).Name()
		if got := tt.d.QuoteIdent(tt.ident); got != tt.quoted {
			t.Errorf("%s.QuoteIdent(%q) = %s, want %s", name, tt.ident, got, tt.quoted)
		}
		from := tableName(tt.d, tbl)
		if got := tt.d.selectPage("c", from, []string{tt.d.QuoteIdent("id")}, 10, 20); got != tt.ordered {
			t.Errorf("%s ordered page = %s\nwant %s", name, got, tt.ordered)
		}
		if got := tt.d.selectPage("c", from, nil, 10, 20); got != tt.plain {
			t.Errorf("%s unordered page = %s\nwant %s", name, got, tt.plain)
		}
	}
}

func TestFor(t *testing.T) {
	for typ, want := range map[conn.DBType]Dialect{
		conn.SQLite: SQLite{}, conn.PostgreSQL: PostgreSQL{}, conn.MySQL: MySQL{}, conn.SQLServer: SQLServer{},
	} {
		if got, err := For(typ); err != nil || got != want {
			t.Errorf("For(%s) = %#v, %v", typ, got, err)
		}
	}
	if _, err := For("oracle"); !errors.Is(err, conn.ErrUnsupportedType) {
		t.Errorf("For(oracle) = %v, want ErrUnsupportedType", err)
	}
}
