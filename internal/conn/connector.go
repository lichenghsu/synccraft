package conn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	mssql "github.com/microsoft/go-mssqldb"
)

// dialer routes every database connection through a tunnel (SSH or a
// Kubernetes port-forward). The DSN keeps the real host, so TLS verifies the
// right name.
type dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	Close() error
}

type openOptions struct {
	dial  dialer // nil = connect directly
	caPEM []byte // PostgreSQL CA from memory (CNPG), instead of a file
}

// openDB builds a *sql.DB for c.
func openDB(c ConnectionConfig, o openOptions) (*sql.DB, error) {
	tun := o.dial
	switch c.Type {
	case MySQL:
		cfg := mysqlConfig(c)
		if c.TLS != nil {
			tc, err := loadTLSConfig(c.TLS, c.Host)
			if err != nil {
				return nil, err
			}
			cfg.TLS = tc
		}
		if tun != nil {
			cfg.DialFunc = tun.DialContext
		}
		connector, err := mysql.NewConnector(cfg)
		if err != nil {
			return nil, err
		}
		return sql.OpenDB(connector), nil

	case PostgreSQL:
		cfg, err := pgx.ParseConfig(postgresDSN(c))
		if err != nil {
			return nil, err
		}
		if tun != nil {
			cfg.DialFunc = tun.DialContext
			// pgx resolves hosts locally by default; the tunnel's far end does it.
			cfg.LookupFunc = func(_ context.Context, host string) ([]string, error) { return []string{host}, nil }
		}
		if o.caPEM != nil {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(o.caPEM) {
				return nil, errors.New("CA certificate contains no PEM certificates")
			}
			cfg.TLSConfig = &tls.Config{RootCAs: pool, ServerName: c.Host, MinVersion: tls.VersionTLS12}
			cfg.Fallbacks = nil
		}
		return stdlib.OpenDB(*cfg), nil

	case SQLServer:
		connector, err := mssql.NewConnector(sqlserverDSN(c))
		if err != nil {
			return nil, err
		}
		if tun != nil {
			connector.Dialer = hostDialer{tun, c.Host}
		}
		return sql.OpenDB(connector), nil
	}

	driver, dsn, err := buildDSN(c)
	if err != nil {
		return nil, err
	}
	return sql.Open(driver, dsn)
}

// loadTLSConfig reads PEM files into a tls.Config (used for MySQL; PostgreSQL
// and SQL Server take the file paths in their DSN).
func loadTLSConfig(files *TLSFiles, serverName string) (*tls.Config, error) {
	tc := &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12}

	if files.CACertPath != "" {
		pem, err := os.ReadFile(files.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("CA certificate file contains no PEM certificates")
		}
		tc.RootCAs = pool
	}

	if files.ClientCertPath != "" {
		cert, err := tls.LoadX509KeyPair(files.ClientCertPath, files.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	return tc, nil
}
