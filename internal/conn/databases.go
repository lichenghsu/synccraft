package conn

import (
	"context"
	"database/sql"
	"fmt"
)

// System databases are hidden; users pick application databases.
var listDatabasesSQL = map[DBType]string{
	MySQL: `SELECT SCHEMA_NAME FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys')
		ORDER BY SCHEMA_NAME`,
	PostgreSQL: `SELECT datname FROM pg_database
		WHERE NOT datistemplate AND datallowconn
		ORDER BY datname`,
	SQLServer: `SELECT name FROM sys.databases
		WHERE database_id > 4
		ORDER BY name`,
	SQLite: `SELECT name FROM pragma_database_list ORDER BY seq`,
}

// ListDatabases returns the non-system databases visible on db. Never nil.
func ListDatabases(ctx context.Context, db *sql.DB, t DBType) ([]string, error) {
	q, ok := listDatabasesSQL[t]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, t)
	}

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
