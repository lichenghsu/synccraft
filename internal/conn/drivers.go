package conn

import (
	_ "github.com/go-sql-driver/mysql"  // "mysql"
	_ "github.com/jackc/pgx/v5/stdlib"  // "pgx"
	_ "github.com/microsoft/go-mssqldb" // "sqlserver"
	_ "modernc.org/sqlite"              // "sqlite" (pure Go, no CGO)
)
