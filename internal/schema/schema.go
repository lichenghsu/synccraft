// Package schema compares the structure of two databases and lists the
// changes that would make the target match the source.
package schema

import (
	"context"
	"database/sql"

	"github.com/lichenghsu/synccraft/internal/dialect"
)

// Table is the full structure of one table.
type Table struct {
	Table      dialect.Table    `json:"table"`
	Columns    []dialect.Column `json:"columns"`
	PrimaryKey []string         `json:"primaryKey"`
	Indexes    []dialect.Index  `json:"indexes"`
}

// Load reads every table in the current database. The result is ordered like
// Dialect.ListTables and is never nil.
func Load(ctx context.Context, d dialect.Dialect, db *sql.DB) ([]Table, error) {
	tables, err := d.ListTables(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]Table, 0, len(tables))
	for _, t := range tables {
		st := Table{Table: t}
		if st.Columns, err = d.Columns(ctx, db, t); err != nil {
			return nil, err
		}
		if st.PrimaryKey, err = d.PrimaryKey(ctx, db, t); err != nil {
			return nil, err
		}
		if st.Indexes, err = d.Indexes(ctx, db, t); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}
