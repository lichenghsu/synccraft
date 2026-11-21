package conn

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/go-sql-driver/mysql"
)

// Driver names registered by the imports in drivers.go.
const (
	driverMySQL     = "mysql"
	driverPgx       = "pgx"
	driverSQLServer = "sqlserver"
	driverSQLite    = "sqlite"
)

// buildDSN validates c and returns the database/sql driver name and DSN.
func buildDSN(c ConnectionConfig) (driver, dsn string, err error) {
	if err := c.Validate(); err != nil {
		return "", "", err
	}

	switch c.Type {
	case MySQL:
		return driverMySQL, mysqlDSN(c), nil
	case PostgreSQL:
		return driverPgx, postgresDSN(c), nil
	case SQLServer:
		return driverSQLServer, sqlserverDSN(c), nil
	case SQLite:
		return driverSQLite, sqliteDSN(c), nil
	}
	return "", "", fmt.Errorf("%w: %q", ErrUnsupportedType, c.Type)
}

// port returns c.Port, falling back to the engine default.
func (c ConnectionConfig) port() int {
	if c.Port == 0 {
		return DefaultPort(c.Type)
	}
	return c.Port
}

func (c ConnectionConfig) hostPort() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.port()))
}

// TLS files are not applied here: MySQL needs a registered tls.Config,
// which is done when the pool opens the connection.
func mysqlDSN(c ConnectionConfig) string {
	cfg := mysql.NewConfig()
	cfg.User = c.User
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = c.hostPort()
	cfg.DBName = c.Database
	cfg.ParseTime = true
	_ = cfg.Apply(mysql.Charset("utf8mb4", "")) // only fails on unsafe collation + interpolateParams
	return cfg.FormatDSN()
}

func postgresDSN(c ConnectionConfig) string {
	mode := c.SSLMode
	if mode == "" {
		mode = "prefer"
	}

	q := url.Values{}
	q.Set("sslmode", mode)
	if t := c.TLS; t != nil {
		setIf(q, "sslrootcert", t.CACertPath)
		setIf(q, "sslcert", t.ClientCertPath)
		setIf(q, "sslkey", t.ClientKeyPath)
	}

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     c.hostPort(),
		Path:     "/" + c.Database,
		RawQuery: q.Encode(),
	}
	return u.String()
}

func sqlserverDSN(c ConnectionConfig) string {
	q := url.Values{}
	q.Set("database", c.Database)
	// Always explicit: go-mssqldb trusts any certificate when this is omitted.
	q.Set("TrustServerCertificate", strconv.FormatBool(c.TrustServerCertificate))
	if c.TLS != nil {
		setIf(q, "certificate", c.TLS.CACertPath)
	}

	u := url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(c.User, c.Password),
		Host:     c.hostPort(),
		RawQuery: q.Encode(),
	}
	return u.String()
}

// A "file:" URI keeps '?', '#' and '%' in the path from being read as query syntax.
func sqliteDSN(c ConnectionConfig) string {
	p := filepath.ToSlash(c.FilePath)
	if filepath.VolumeName(c.FilePath) != "" {
		p = "/" + p // file:///C:/data.db
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "_pragma=foreign_keys(1)"}
	return u.String()
}

func setIf(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}
