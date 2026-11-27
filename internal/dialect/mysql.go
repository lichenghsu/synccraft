package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// MySQL reads metadata from MySQL and MariaDB. It only sees the current
// database: Table.Schema is empty in results, and an empty Schema in
// arguments means DATABASE().
type MySQL struct{}

var _ Dialect = MySQL{}

// mysqlTableFilter binds two arguments: schema (may be empty), then table.
const mysqlTableFilter = `TABLE_SCHEMA = COALESCE(NULLIF(?, ''), DATABASE()) AND TABLE_NAME = ?`

const mysqlListTablesSQL = `SELECT TABLE_NAME FROM information_schema.TABLES
	WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'
	ORDER BY TABLE_NAME`

func (MySQL) ListTables(ctx context.Context, db *sql.DB) ([]Table, error) {
	rows, err := db.QueryContext(ctx, mysqlListTablesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := []Table{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, Table{Name: name})
	}
	return tables, rows.Err()
}

const mysqlColumnsSQL = `SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE = 'YES', COLUMN_DEFAULT, EXTRA
	FROM information_schema.COLUMNS
	WHERE ` + mysqlTableFilter + `
	ORDER BY ORDINAL_POSITION`

func (MySQL) Columns(ctx context.Context, db *sql.DB, t Table) ([]Column, error) {
	if err := mysqlTableExists(ctx, db, t); err != nil {
		return nil, err
	}
	var version string
	if err := db.QueryRowContext(ctx, `SELECT VERSION()`).Scan(&version); err != nil {
		return nil, err
	}
	mariaDB := strings.Contains(version, "MariaDB")

	rows, err := db.QueryContext(ctx, mysqlColumnsSQL, t.Schema, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := []Column{}
	for rows.Next() {
		var (
			c     Column
			dflt  sql.NullString
			extra string
		)
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &dflt, &extra); err != nil {
			return nil, err
		}
		c.Default = mysqlDefault(dflt, extra, mariaDB)
		c.AutoIncrement = strings.Contains(strings.ToLower(extra), "auto_increment")
		c.Position = len(cols) + 1
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// mysqlDefault turns COLUMN_DEFAULT into a default expression.
//
// MySQL stores literals unquoted ("guest") and marks expressions with
// DEFAULT_GENERATED in EXTRA. MariaDB 10.2.7+ already stores expressions
// ("'guest'", "current_timestamp()") and the string "NULL" for DEFAULT NULL.
func mysqlDefault(raw sql.NullString, extra string, mariaDB bool) *string {
	extra = strings.ToUpper(extra)
	if !raw.Valid || strings.Contains(extra, "GENERATED") && !strings.Contains(extra, "DEFAULT_GENERATED") {
		return nil
	}
	s := raw.String
	switch {
	case mariaDB:
		if s == "NULL" {
			return nil
		}
	case !strings.Contains(extra, "DEFAULT_GENERATED"):
		s = "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return &s
}

const mysqlPrimaryKeySQL = `SELECT COLUMN_NAME FROM information_schema.STATISTICS
	WHERE ` + mysqlTableFilter + ` AND INDEX_NAME = 'PRIMARY'
	ORDER BY SEQ_IN_INDEX`

func (MySQL) PrimaryKey(ctx context.Context, db *sql.DB, t Table) ([]string, error) {
	if err := mysqlTableExists(ctx, db, t); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, mysqlPrimaryKeySQL, t.Schema, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	key := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		key = append(key, name)
	}
	return key, rows.Err()
}

// COLUMN_NAME is NULL for functional key parts (MySQL 8.0.13+).
const mysqlIndexesSQL = `SELECT INDEX_NAME, NON_UNIQUE = 0, COALESCE(COLUMN_NAME, '')
	FROM information_schema.STATISTICS
	WHERE ` + mysqlTableFilter + ` AND INDEX_NAME <> 'PRIMARY'
	ORDER BY INDEX_NAME, SEQ_IN_INDEX`

// Indexes reports Constraint as false: MySQL does not distinguish a UNIQUE
// constraint from a unique index.
func (MySQL) Indexes(ctx context.Context, db *sql.DB, t Table) ([]Index, error) {
	if err := mysqlTableExists(ctx, db, t); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, mysqlIndexesSQL, t.Schema, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexes := []Index{}
	for rows.Next() {
		var (
			name, column string
			unique       bool
		)
		if err := rows.Scan(&name, &unique, &column); err != nil {
			return nil, err
		}
		indexes = appendIndexColumn(indexes, name, unique, false, column)
	}
	return indexes, rows.Err()
}

func mysqlTableExists(ctx context.Context, db *sql.DB, t Table) error {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.TABLES WHERE `+mysqlTableFilter+` AND TABLE_TYPE = 'BASE TABLE'`,
		t.Schema, t.Name).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrTableNotFound, t.Name)
	}
	return nil
}

func (MySQL) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func (MySQL) selectPage(cols, from string, orderBy []string, limit, offset int) string {
	return limitOffset(cols, from, orderBy, limit, offset)
}

func (MySQL) countExpr() string { return "COUNT(*)" }
