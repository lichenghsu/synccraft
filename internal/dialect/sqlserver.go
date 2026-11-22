package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

// SQLServer reads metadata from SQL Server. An empty Table.Schema means the
// user's default schema (SCHEMA_NAME(), usually dbo).
type SQLServer struct{}

var _ Dialect = SQLServer{}

// mssqlObjectID resolves @p1 (schema) and @p2 (table) to a user table's
// object_id, or NULL.
const mssqlObjectID = `OBJECT_ID(QUOTENAME(COALESCE(NULLIF(@p1, ''), SCHEMA_NAME())) + '.' + QUOTENAME(@p2), 'U')`

const mssqlListTablesSQL = `SELECT s.name, t.name
	FROM sys.tables t
	JOIN sys.schemas s ON s.schema_id = t.schema_id
	WHERE t.is_ms_shipped = 0
	ORDER BY s.name, t.name`

func (SQLServer) ListTables(ctx context.Context, db *sql.DB) ([]Table, error) {
	rows, err := db.QueryContext(ctx, mssqlListTablesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := []Table{}
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.Schema, &t.Name); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

const mssqlColumnsSQL = `SELECT c.name, ty.name, c.max_length, c.precision, c.scale, c.is_nullable,
		CASE WHEN c.is_computed = 0 THEN dc.definition END
	FROM sys.columns c
	JOIN sys.types ty ON ty.user_type_id = c.user_type_id
	LEFT JOIN sys.default_constraints dc ON dc.object_id = c.default_object_id
	WHERE c.object_id = ` + mssqlObjectID + `
	ORDER BY c.column_id`

func (SQLServer) Columns(ctx context.Context, db *sql.DB, t Table) ([]Column, error) {
	if err := mssqlTableExists(ctx, db, t); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, mssqlColumnsSQL, t.Schema, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := []Column{}
	for rows.Next() {
		var (
			c                        Column
			typeName                 string
			maxLen, precision, scale int
			dflt                     sql.NullString
		)
		if err := rows.Scan(&c.Name, &typeName, &maxLen, &precision, &scale, &c.Nullable, &dflt); err != nil {
			return nil, err
		}
		c.Type = mssqlType(typeName, maxLen, precision, scale)
		if dflt.Valid {
			s := unwrapParens(dflt.String)
			c.Default = &s
		}
		c.Position = len(cols) + 1
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// mssqlType rebuilds the declared type from sys.columns. max_length is in
// bytes (-1 for max), so n-types are halved.
func mssqlType(name string, maxLen, precision, scale int) string {
	length := func(n int) string {
		if maxLen == -1 {
			return name + "(max)"
		}
		return name + "(" + strconv.Itoa(n) + ")"
	}
	switch name {
	case "varchar", "char", "varbinary", "binary":
		return length(maxLen)
	case "nvarchar", "nchar":
		return length(maxLen / 2)
	case "decimal", "numeric":
		return fmt.Sprintf("%s(%d,%d)", name, precision, scale)
	case "datetime2", "time", "datetimeoffset":
		return fmt.Sprintf("%s(%d)", name, scale)
	}
	return name
}

// unwrapParens strips the parentheses SQL Server wraps around stored
// defaults: "('guest')" -> "'guest'", "((0))" -> "0". Parentheses inside
// string literals are ignored.
func unwrapParens(s string) string {
	for len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' && closingParen(s) == len(s)-1 {
		s = s[1 : len(s)-1]
	}
	return s
}

// closingParen returns the index of the ')' matching s[0], or -1.
func closingParen(s string) int {
	depth, inString := 0, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'':
			inString = !inString // '' inside a literal toggles twice
		case inString:
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

const mssqlPrimaryKeySQL = `SELECT col.name
	FROM sys.indexes i
	JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
	JOIN sys.columns col ON col.object_id = ic.object_id AND col.column_id = ic.column_id
	WHERE i.object_id = ` + mssqlObjectID + ` AND i.is_primary_key = 1
	ORDER BY ic.key_ordinal`

func (SQLServer) PrimaryKey(ctx context.Context, db *sql.DB, t Table) ([]string, error) {
	if err := mssqlTableExists(ctx, db, t); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, mssqlPrimaryKeySQL, t.Schema, t.Name)
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

// key_ordinal is 0 for INCLUDE columns.
const mssqlIndexesSQL = `SELECT i.name, i.is_unique, i.is_unique_constraint, col.name
	FROM sys.indexes i
	JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
	JOIN sys.columns col ON col.object_id = ic.object_id AND col.column_id = ic.column_id
	WHERE i.object_id = ` + mssqlObjectID + `
	  AND i.is_primary_key = 0
	  AND i.is_hypothetical = 0
	  AND ic.key_ordinal > 0
	ORDER BY i.name, ic.key_ordinal`

func (SQLServer) Indexes(ctx context.Context, db *sql.DB, t Table) ([]Index, error) {
	if err := mssqlTableExists(ctx, db, t); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, mssqlIndexesSQL, t.Schema, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexes := []Index{}
	for rows.Next() {
		var (
			name, column       string
			unique, constraint bool
		)
		if err := rows.Scan(&name, &unique, &constraint, &column); err != nil {
			return nil, err
		}
		indexes = appendIndexColumn(indexes, name, unique, constraint, column)
	}
	return indexes, rows.Err()
}

func mssqlTableExists(ctx context.Context, db *sql.DB, t Table) error {
	var ok bool
	err := db.QueryRowContext(ctx,
		`SELECT CAST(CASE WHEN `+mssqlObjectID+` IS NULL THEN 0 ELSE 1 END AS bit)`,
		t.Schema, t.Name).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s.%s", ErrTableNotFound, t.Schema, t.Name)
	}
	return nil
}
