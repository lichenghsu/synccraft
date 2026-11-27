package dialect

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits on a single cell so one huge value can't bloat a preview page.
const (
	maxCellRunes = 1000
	maxCellBytes = 32
)

// maxSafeInt is the largest integer a JavaScript number holds exactly.
const maxSafeInt = 1<<53 - 1

// RowPage is one page of rows from a table, for display.
type RowPage struct {
	Columns []string `json:"columns"`
	// Rows hold display values: null, bool, number or string. Binary values
	// are hex ("0x..."), times are RFC 3339, integers beyond 2^53 and
	// non-finite floats are strings. Long values are cut and end with "…".
	Rows [][]any `json:"rows"`
	// Ordered is true when rows are sorted by primary key, so pages don't
	// overlap. Tables without a primary key come back in server order.
	Ordered bool `json:"ordered"`
}

// ReadRows returns up to limit rows of t, skipping offset rows, sorted by
// primary key when t has one. Returns an error wrapping ErrTableNotFound if t
// does not exist.
func ReadRows(ctx context.Context, d Dialect, db *sql.DB, t Table, offset, limit int) (RowPage, error) {
	if offset < 0 || limit <= 0 {
		return RowPage{}, fmt.Errorf("invalid page: offset %d, limit %d", offset, limit)
	}
	cols, err := d.Columns(ctx, db, t)
	if err != nil {
		return RowPage{}, err
	}
	pk, err := d.PrimaryKey(ctx, db, t)
	if err != nil {
		return RowPage{}, err
	}

	page := RowPage{Columns: make([]string, len(cols)), Rows: [][]any{}, Ordered: len(pk) > 0}
	if len(cols) == 0 {
		return page, nil // PostgreSQL allows tables with no columns
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		page.Columns[i] = c.Name
		quoted[i] = d.QuoteIdent(c.Name)
	}
	orderBy := make([]string, len(pk))
	for i, name := range pk {
		orderBy[i] = d.QuoteIdent(name)
	}

	query := d.selectPage(strings.Join(quoted, ", "), tableName(d, t), orderBy, limit, offset)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return RowPage{}, err
	}
	defer rows.Close()

	types, err := rows.ColumnTypes()
	if err != nil {
		return RowPage{}, err
	}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return RowPage{}, err
		}
		for i, v := range vals {
			vals[i] = cellValue(v, types[i].DatabaseTypeName())
		}
		page.Rows = append(page.Rows, vals)
	}
	return page, rows.Err()
}

// CountRows returns the number of rows in t. It scans the whole table on
// most engines, so callers should let the user cancel it.
func CountRows(ctx context.Context, d Dialect, db *sql.DB, t Table) (int64, error) {
	// Columns doubles as the existence check: it fails for views too.
	if _, err := d.Columns(ctx, db, t); err != nil {
		return 0, err
	}
	var n int64
	err := db.QueryRowContext(ctx, `SELECT `+d.countExpr()+` FROM `+tableName(d, t)).Scan(&n)
	return n, err
}

func tableName(d Dialect, t Table) string {
	if t.Schema == "" {
		return d.QuoteIdent(t.Name)
	}
	return d.QuoteIdent(t.Schema) + "." + d.QuoteIdent(t.Name)
}

// limitOffset is the paging clause shared by SQLite, PostgreSQL and MySQL.
func limitOffset(cols, from string, orderBy []string, limit, offset int) string {
	q := `SELECT ` + cols + ` FROM ` + from
	if len(orderBy) > 0 {
		q += ` ORDER BY ` + strings.Join(orderBy, ", ")
	}
	return q + fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, offset)
}

func doubleQuote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// cellValue turns a scanned value into something JSON and JavaScript keep
// exactly. dbType is the driver's column type name (may be empty).
func cellValue(v any, dbType string) any {
	switch x := v.(type) {
	case nil, bool:
		return x
	case int64:
		if x > maxSafeInt || x < -maxSafeInt {
			return strconv.FormatInt(x, 10)
		}
		return x
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return strconv.FormatFloat(x, 'g', -1, 64)
		}
		return x
	case float32:
		return cellValue(float64(x), dbType)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case string:
		return cutString(x)
	case []byte:
		if dbType == "UNIQUEIDENTIFIER" && len(x) == 16 {
			return mssqlGUID(x)
		}
		if utf8.Valid(x) {
			return cutString(string(x))
		}
		if len(x) > maxCellBytes {
			return fmt.Sprintf("0x%s… (%d bytes)", hex.EncodeToString(x[:maxCellBytes]), len(x))
		}
		return "0x" + hex.EncodeToString(x)
	default:
		return cutString(fmt.Sprint(x))
	}
}

func cutString(s string) string {
	if utf8.RuneCountInString(s) <= maxCellRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxCellRunes]) + "…"
}

// mssqlGUID formats a SQL Server uniqueidentifier, whose first three groups
// are stored little-endian.
func mssqlGUID(b []byte) string {
	return fmt.Sprintf("%X-%X-%X-%X-%X",
		[]byte{b[3], b[2], b[1], b[0]},
		[]byte{b[5], b[4]},
		[]byte{b[7], b[6]},
		b[8:10], b[10:16])
}
