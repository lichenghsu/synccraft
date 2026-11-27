// Package dialect reads database metadata (tables, columns, keys, indexes)
// for each supported engine.
package dialect

import (
	"context"
	"database/sql"
	"errors"
)

// ErrTableNotFound is returned when the requested table does not exist.
var ErrTableNotFound = errors.New("table not found")

// Table is a base table. Views and engine-internal tables are excluded.
type Table struct {
	// Schema is empty for engines without schemas (SQLite).
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

// Column describes one table column.
type Column struct {
	Name string `json:"name"`
	// Type is the declared type as written, e.g. "VARCHAR(20)". Empty if none.
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	// Default is the raw default expression, e.g. "'guest'" or
	// "CURRENT_TIMESTAMP". Nil means no default.
	Default *string `json:"default"`
	// Position is 1-based and contiguous, in table definition order.
	Position int `json:"position"`
	// AutoIncrement marks a column the engine numbers itself: MySQL
	// AUTO_INCREMENT, PostgreSQL identity, SQL Server IDENTITY. PostgreSQL
	// serial shows as a nextval default instead; SQLite never sets it.
	AutoIncrement bool `json:"autoIncrement"`
}

// Index is a secondary index. The primary key is reported by PrimaryKey, not
// as an Index.
type Index struct {
	Name string `json:"name"`
	// Columns are in key order. An expression key part is an empty string.
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	// Constraint is true when the index backs a UNIQUE constraint rather than
	// a CREATE INDEX statement.
	Constraint bool `json:"constraint"`
}

// Dialect reads metadata from one database engine.
type Dialect interface {
	// ListTables returns the user tables in the current database, ordered by
	// schema then name. Never nil.
	ListTables(ctx context.Context, db *sql.DB) ([]Table, error)

	// Columns returns the columns of t ordered by Position. Returns an error
	// wrapping ErrTableNotFound if t does not exist.
	Columns(ctx context.Context, db *sql.DB, t Table) ([]Column, error)

	// PrimaryKey returns the primary key column names in key order, or an
	// empty slice if t has none. Returns an error wrapping ErrTableNotFound
	// if t does not exist.
	PrimaryKey(ctx context.Context, db *sql.DB, t Table) ([]string, error)

	// Indexes returns the secondary indexes of t ordered by name. Never nil.
	// Returns an error wrapping ErrTableNotFound if t does not exist.
	Indexes(ctx context.Context, db *sql.DB, t Table) ([]Index, error)

	// QuoteIdent quotes one identifier for use in SQL text.
	QuoteIdent(name string) string

	// selectPage builds a SELECT of cols from a quoted table name, sorted by
	// orderBy (quoted, may be empty) and limited to one page.
	selectPage(cols, from string, orderBy []string, limit, offset int) string

	// countExpr is the row-count aggregate, e.g. COUNT(*).
	countExpr() string
}

// appendIndexColumn adds column to the last index if it is named name,
// otherwise starts a new index. Rows must arrive ordered by index name, then
// key position.
func appendIndexColumn(indexes []Index, name string, unique, constraint bool, column string) []Index {
	if n := len(indexes); n == 0 || indexes[n-1].Name != name {
		indexes = append(indexes, Index{
			Name:       name,
			Columns:    []string{},
			Unique:     unique,
			Constraint: constraint,
		})
	}
	last := &indexes[len(indexes)-1]
	last.Columns = append(last.Columns, column)
	return indexes
}
