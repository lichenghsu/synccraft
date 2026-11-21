package conn

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ServerInfo identifies the database server behind a connection. MariaDB is
// reported separately even though it shares the MySQL type, because its SQL
// dialect differs.
type ServerInfo struct {
	Product string `json:"product"` // MySQL, MariaDB, PostgreSQL, SQL Server, SQLite
	Version string `json:"version"` // e.g. 10.11.6
	Raw     string `json:"raw"`     // the server's full version string
}

const (
	ProductMySQL      = "MySQL"
	ProductMariaDB    = "MariaDB"
	ProductPostgreSQL = "PostgreSQL"
	ProductSQLServer  = "SQL Server"
	ProductSQLite     = "SQLite"
)

var serverVersionSQL = map[DBType]string{
	MySQL:      `SELECT VERSION()`,
	PostgreSQL: `SHOW server_version`,
	SQLServer:  `SELECT CAST(SERVERPROPERTY('ProductVersion') AS nvarchar(128)) + N' ' + CAST(SERVERPROPERTY('Edition') AS nvarchar(128))`,
	SQLite:     `SELECT sqlite_version()`,
}

// DetectServer asks the server what it is.
func DetectServer(ctx context.Context, db *sql.DB, t DBType) (ServerInfo, error) {
	q, ok := serverVersionSQL[t]
	if !ok {
		return ServerInfo{}, fmt.Errorf("%w: %q", ErrUnsupportedType, t)
	}
	var raw string
	if err := db.QueryRowContext(ctx, q).Scan(&raw); err != nil {
		return ServerInfo{}, err
	}
	return parseServerInfo(t, raw), nil
}

func parseServerInfo(t DBType, raw string) ServerInfo {
	info := ServerInfo{Raw: raw, Version: firstField(raw)}
	switch t {
	case MySQL:
		info.Product = ProductMySQL
		if strings.Contains(strings.ToLower(raw), "mariadb") {
			info.Product = ProductMariaDB
		}
		// "10.11.6-MariaDB-0+deb12u2", "8.4.3", "8.0.36-0ubuntu0.22.04.1"
		info.Version, _, _ = strings.Cut(info.Version, "-")
	case PostgreSQL:
		info.Product = ProductPostgreSQL // "17.12 (Debian 17.12-1.pgdg13+1)"
	case SQLServer:
		info.Product = ProductSQLServer // "16.0.4215.2 Developer Edition (64-bit)"
	case SQLite:
		info.Product = ProductSQLite
	}
	return info
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}
