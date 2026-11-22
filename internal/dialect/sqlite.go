package dialect

import (
	"context"
	"database/sql"
	"fmt"
)

// SQLite reads metadata from SQLite databases.
type SQLite struct{}

var _ Dialect = SQLite{}

const sqliteListTablesSQL = `SELECT name FROM sqlite_schema
	WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\'
	ORDER BY name`

func (SQLite) ListTables(ctx context.Context, db *sql.DB) ([]Table, error) {
	rows, err := db.QueryContext(ctx, sqliteListTablesSQL)
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

const sqliteColumnsSQL = `SELECT cid, name, type, "notnull", dflt_value
	FROM pragma_table_info(?)
	ORDER BY cid`

func (SQLite) Columns(ctx context.Context, db *sql.DB, t Table) ([]Column, error) {
	rows, err := db.QueryContext(ctx, sqliteColumnsSQL, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := []Column{}
	for rows.Next() {
		var (
			c       Column
			cid     int
			notNull bool
			dflt    sql.NullString
		)
		if err := rows.Scan(&cid, &c.Name, &c.Type, &notNull, &dflt); err != nil {
			return nil, err
		}
		c.Position = cid + 1
		c.Nullable = !notNull
		if dflt.Valid {
			s := dflt.String
			c.Default = &s
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrTableNotFound, t.Name)
	}
	return cols, nil
}
