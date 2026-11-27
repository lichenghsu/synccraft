package dialect

import (
	"context"
	"database/sql"
	"fmt"
)

// PostgreSQL reads metadata from PostgreSQL databases. An empty Table.Schema
// means the connection's current schema.
type PostgreSQL struct{}

var _ Dialect = PostgreSQL{}

// pgRegclass resolves $1 (schema) and $2 (table) to an OID. %I quotes
// mixed-case names; NULL if the relation does not exist.
const pgRegclass = `to_regclass(format('%I.%I', coalesce(nullif($1::text, ''), current_schema()), $2::text))`

// Ordinary and partitioned tables; partitions are listed through their parent.
const pgListTablesSQL = `SELECT n.nspname, c.relname
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE c.relkind IN ('r', 'p')
	  AND NOT c.relispartition
	  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
	  AND n.nspname NOT LIKE 'pg\_toast%'
	  AND n.nspname NOT LIKE 'pg\_temp\_%'
	ORDER BY n.nspname, c.relname`

func (PostgreSQL) ListTables(ctx context.Context, db *sql.DB) ([]Table, error) {
	rows, err := db.QueryContext(ctx, pgListTablesSQL)
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

// Generated columns keep their expression in pg_attrdef; it is not a default.
const pgColumnsSQL = `SELECT a.attname,
		format_type(a.atttypid, a.atttypmod),
		NOT a.attnotnull,
		CASE WHEN a.attgenerated = '' THEN pg_get_expr(d.adbin, d.adrelid) END
	FROM pg_attribute a
	LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
	WHERE a.attrelid = ` + pgRegclass + `
	  AND a.attnum > 0
	  AND NOT a.attisdropped
	ORDER BY a.attnum`

func (PostgreSQL) Columns(ctx context.Context, db *sql.DB, t Table) ([]Column, error) {
	// A table may legally have zero columns, so existence is checked separately.
	if err := pgTableExists(ctx, db, t); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, pgColumnsSQL, t.Schema, t.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := []Column{}
	for rows.Next() {
		var (
			c    Column
			dflt sql.NullString
		)
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &dflt); err != nil {
			return nil, err
		}
		// attnum keeps gaps from dropped columns; renumber so positions
		// compare across engines.
		c.Position = len(cols) + 1
		if dflt.Valid {
			s := dflt.String
			c.Default = &s
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

const pgPrimaryKeySQL = `SELECT a.attname
	FROM pg_index i
	JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
	WHERE i.indrelid = ` + pgRegclass + `
	  AND i.indisprimary
	ORDER BY array_position(i.indkey::int2[], a.attnum)`

func (PostgreSQL) PrimaryKey(ctx context.Context, db *sql.DB, t Table) ([]string, error) {
	if err := pgTableExists(ctx, db, t); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, pgPrimaryKeySQL, t.Schema, t.Name)
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

// indkey holds key columns then INCLUDE columns; only the first indnkeyatts
// are key columns. attnum 0 is an expression, which has no pg_attribute row.
const pgIndexesSQL = `SELECT ic.relname,
		i.indisunique,
		EXISTS (SELECT 1 FROM pg_constraint con
			WHERE con.conindid = i.indexrelid AND con.contype = 'u'),
		coalesce(a.attname, '')
	FROM pg_index i
	JOIN pg_class ic ON ic.oid = i.indexrelid
	CROSS JOIN LATERAL unnest(i.indkey::int2[]) WITH ORDINALITY AS k(attnum, ord)
	LEFT JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
	WHERE i.indrelid = ` + pgRegclass + `
	  AND NOT i.indisprimary
	  AND k.ord <= i.indnkeyatts
	ORDER BY ic.relname, k.ord`

func (PostgreSQL) Indexes(ctx context.Context, db *sql.DB, t Table) ([]Index, error) {
	if err := pgTableExists(ctx, db, t); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, pgIndexesSQL, t.Schema, t.Name)
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

// pgTableExists reports ErrTableNotFound unless t is an ordinary or
// partitioned table (to_regclass also resolves views, sequences and indexes).
func pgTableExists(ctx context.Context, db *sql.DB, t Table) error {
	var ok bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_class WHERE oid = `+pgRegclass+` AND relkind IN ('r', 'p'))`,
		t.Schema, t.Name).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s.%s", ErrTableNotFound, t.Schema, t.Name)
	}
	return nil
}

func (PostgreSQL) QuoteIdent(name string) string { return doubleQuote(name) }

func (PostgreSQL) selectPage(cols, from string, orderBy []string, limit, offset int) string {
	return limitOffset(cols, from, orderBy, limit, offset)
}

func (PostgreSQL) countExpr() string { return "COUNT(*)" }
