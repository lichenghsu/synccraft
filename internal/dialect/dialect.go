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
	// Position is 1-based, in table definition order.
	Position int `json:"position"`
}

// Dialect reads metadata from one database engine.
type Dialect interface {
	// ListTables returns the user tables in the current database, ordered by
	// schema then name. Never nil.
	ListTables(ctx context.Context, db *sql.DB) ([]Table, error)

	// Columns returns the columns of t ordered by Position. Returns an error
	// wrapping ErrTableNotFound if t does not exist.
	Columns(ctx context.Context, db *sql.DB, t Table) ([]Column, error)
}
