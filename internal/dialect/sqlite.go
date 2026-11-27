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
	// pragma_table_info also answers for views, so check first.
	if err := sqliteTableExists(ctx, db, t.Name); err != nil {
		return nil, err
	}

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
	return cols, rows.Err()
}

// pk is 0 for non-key columns, otherwise the 1-based position in the key.
const sqlitePrimaryKeySQL = `SELECT pk, name FROM pragma_table_info(?) ORDER BY pk`

func (SQLite) PrimaryKey(ctx context.Context, db *sql.DB, t Table) ([]string, error) {
	if err := sqliteTableExists(ctx, db, t.Name); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlitePrimaryKeySQL, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	key := []string{}
	for rows.Next() {
		var (
			pk   int
			name string
		)
		if err := rows.Scan(&pk, &name); err != nil {
			return nil, err
		}
		if pk > 0 {
			key = append(key, name)
		}
	}
	return key, rows.Err()
}

// origin: 'c' = CREATE INDEX, 'u' = UNIQUE constraint, 'pk' = PRIMARY KEY.
// ii.name is NULL for expression key parts.
const sqliteIndexesSQL = `SELECT il.name, il."unique", il.origin, ii.name
	FROM pragma_index_list(?) AS il
	JOIN pragma_index_info(il.name) AS ii
	WHERE il.origin <> 'pk'
	ORDER BY il.name, ii.seqno`

func (SQLite) Indexes(ctx context.Context, db *sql.DB, t Table) ([]Index, error) {
	// pragma_index_list returns no rows for a missing table or a view.
	if err := sqliteTableExists(ctx, db, t.Name); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqliteIndexesSQL, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexes := []Index{}
	for rows.Next() {
		var (
			name, origin string
			unique       bool
			column       sql.NullString
		)
		if err := rows.Scan(&name, &unique, &origin, &column); err != nil {
			return nil, err
		}
		indexes = appendIndexColumn(indexes, name, unique, origin == "u", column.String)
	}
	return indexes, rows.Err()
}

func sqliteTableExists(ctx context.Context, db *sql.DB, name string) error {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`, name).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrTableNotFound, name)
	}
	return nil
}

func (SQLite) QuoteIdent(name string) string { return doubleQuote(name) }

func (SQLite) selectPage(cols, from string, orderBy []string, limit, offset int) string {
	return limitOffset(cols, from, orderBy, limit, offset)
}

func (SQLite) countExpr() string { return "COUNT(*)" }
