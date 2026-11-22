package dialect

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// assertNotFound checks that every per-table method reports ErrTableNotFound.
func assertNotFound(t *testing.T, d Dialect, db *sql.DB, tbl Table) {
	t.Helper()
	ctx := context.Background()
	if _, err := d.Columns(ctx, db, tbl); !errors.Is(err, ErrTableNotFound) {
		t.Errorf("Columns(%+v): err = %v, want ErrTableNotFound", tbl, err)
	}
	if _, err := d.PrimaryKey(ctx, db, tbl); !errors.Is(err, ErrTableNotFound) {
		t.Errorf("PrimaryKey(%+v): err = %v, want ErrTableNotFound", tbl, err)
	}
	if _, err := d.Indexes(ctx, db, tbl); !errors.Is(err, ErrTableNotFound) {
		t.Errorf("Indexes(%+v): err = %v, want ErrTableNotFound", tbl, err)
	}
}

// assertCanceled checks that every method honors a canceled context.
func assertCanceled(t *testing.T, d Dialect, db *sql.DB, tbl Table) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.ListTables(ctx, db); !errors.Is(err, context.Canceled) {
		t.Errorf("ListTables: err = %v, want context.Canceled", err)
	}
	if _, err := d.Columns(ctx, db, tbl); !errors.Is(err, context.Canceled) {
		t.Errorf("Columns: err = %v, want context.Canceled", err)
	}
	if _, err := d.PrimaryKey(ctx, db, tbl); !errors.Is(err, context.Canceled) {
		t.Errorf("PrimaryKey: err = %v, want context.Canceled", err)
	}
	if _, err := d.Indexes(ctx, db, tbl); !errors.Is(err, context.Canceled) {
		t.Errorf("Indexes: err = %v, want context.Canceled", err)
	}
}

func TestMySQLDefault(t *testing.T) {
	null := sql.NullString{}
	str := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

	tests := []struct {
		name    string
		raw     sql.NullString
		extra   string
		mariaDB bool
		want    *string
	}{
		{"no default", null, "", false, nil},
		{"mysql literal is quoted", str("guest"), "", false, ptr("'guest'")},
		{"mysql quote in literal is doubled", str("it's"), "", false, ptr("'it''s'")},
		{"mysql number literal", str("0"), "", false, ptr("'0'")},
		{"mysql expression", str("CURRENT_TIMESTAMP"), "DEFAULT_GENERATED", false, ptr("CURRENT_TIMESTAMP")},
		{"mysql expression with on update", str("CURRENT_TIMESTAMP"), "DEFAULT_GENERATED on update CURRENT_TIMESTAMP", false, ptr("CURRENT_TIMESTAMP")},
		{"mysql generated column", null, "STORED GENERATED", false, nil},
		{"mariadb literal kept", str("'guest'"), "", true, ptr("'guest'")},
		{"mariadb expression kept", str("current_timestamp()"), "", true, ptr("current_timestamp()")},
		{"mariadb DEFAULT NULL", str("NULL"), "", true, nil},
		{"mariadb generated column", str("NULL"), "STORED GENERATED", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mysqlDefault(tt.raw, tt.extra, tt.mariaDB)
			if (got == nil) != (tt.want == nil) || got != nil && *got != *tt.want {
				t.Errorf("mysqlDefault = %s, want %s", show(got), show(tt.want))
			}
		})
	}
}

func show(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestMSSQLType(t *testing.T) {
	tests := []struct {
		name                     string
		maxLen, precision, scale int
		want                     string
	}{
		{"int", 4, 10, 0, "int"},
		{"varchar", 50, 0, 0, "varchar(50)"},
		{"varchar", -1, 0, 0, "varchar(max)"},
		{"nvarchar", 40, 0, 0, "nvarchar(20)"},
		{"nvarchar", -1, 0, 0, "nvarchar(max)"},
		{"nchar", 8, 0, 0, "nchar(4)"},
		{"varbinary", 16, 0, 0, "varbinary(16)"},
		{"decimal", 9, 10, 2, "decimal(10,2)"},
		{"datetime2", 7, 23, 3, "datetime2(3)"},
		{"time", 3, 8, 0, "time(0)"},
	}
	for _, tt := range tests {
		if got := mssqlType(tt.name, tt.maxLen, tt.precision, tt.scale); got != tt.want {
			t.Errorf("mssqlType(%q, %d, %d, %d) = %q, want %q", tt.name, tt.maxLen, tt.precision, tt.scale, got, tt.want)
		}
	}
}

func TestUnwrapParens(t *testing.T) {
	tests := map[string]string{
		"('guest')":           "'guest'",
		"((0))":               "0",
		"(getdate())":         "getdate()",
		"((1)+(2))":           "(1)+(2)",
		"('(x)')":             "'(x)'",
		"('a)(b')":            "'a)(b'",
		"('it''s)')":          "'it''s)'",
		"(1)+(2)":             "(1)+(2)",
		"":                    "",
		"(":                   "(",
		"(newsequentialid())": "newsequentialid()",
	}
	for in, want := range tests {
		if got := unwrapParens(in); got != want {
			t.Errorf("unwrapParens(%q) = %q, want %q", in, got, want)
		}
	}
}
