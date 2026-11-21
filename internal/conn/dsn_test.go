package conn

import (
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/microsoft/go-mssqldb/msdsn"
)

// Passwords that break naive string concatenation in at least one DSN format.
var trickyPasswords = []string{
	"simple",
	"p@ss/w;rd",
	"with space",
	`quote'"`,
	"colon:percent%20",
	"=&?#",
	`back\slash`,
	"中文密碼",
}

func TestBuildDSNDriverNames(t *testing.T) {
	want := map[string]string{
		"mysql":     driverMySQL,
		"postgres":  driverPgx,
		"sqlserver": driverSQLServer,
		"sqlite":    driverSQLite,
	}
	for name, c := range validConfigs() {
		t.Run(name, func(t *testing.T) {
			driver, dsn, err := buildDSN(c)
			if err != nil {
				t.Fatalf("buildDSN() error = %v", err)
			}
			if driver != want[name] {
				t.Errorf("driver = %q, want %q", driver, want[name])
			}
			if dsn == "" {
				t.Error("dsn is empty")
			}
		})
	}
}

func TestBuildDSNValidates(t *testing.T) {
	_, _, err := buildDSN(ConnectionConfig{Type: MySQL})
	if !errors.Is(err, ErrMissingField) {
		t.Fatalf("buildDSN(invalid) error = %v, want ErrMissingField", err)
	}
}

func TestMySQLDSN(t *testing.T) {
	for _, pw := range trickyPasswords {
		t.Run(pw, func(t *testing.T) {
			c := ConnectionConfig{Type: MySQL, Host: "db.local", Port: 3307, User: "app", Password: pw, Database: "shop"}
			dsn := mysqlDSN(c)

			got, err := mysql.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("ParseDSN(%q) error = %v", dsn, err)
			}
			if got.User != c.User || got.Passwd != pw || got.DBName != c.Database {
				t.Errorf("round trip = user %q pass %q db %q", got.User, got.Passwd, got.DBName)
			}
			if got.Net != "tcp" || got.Addr != "db.local:3307" {
				t.Errorf("net/addr = %q %q", got.Net, got.Addr)
			}
			if !got.ParseTime {
				t.Error("ParseTime should be true")
			}
			if got.MultiStatements {
				t.Error("MultiStatements must stay off")
			}
			if !strings.Contains(dsn, "charset=utf8mb4") {
				t.Errorf("dsn %q missing charset=utf8mb4", dsn)
			}
		})
	}
}

func TestPostgresDSN(t *testing.T) {
	for _, pw := range trickyPasswords {
		t.Run(pw, func(t *testing.T) {
			c := ConnectionConfig{Type: PostgreSQL, Host: "db.local", Port: 5433, User: "app", Password: pw, Database: "shop"}
			dsn := postgresDSN(c)

			got, err := pgconn.ParseConfig(dsn)
			if err != nil {
				t.Fatalf("ParseConfig(%q) error = %v", dsn, err)
			}
			if got.User != c.User || got.Password != pw || got.Database != c.Database {
				t.Errorf("round trip = user %q pass %q db %q", got.User, got.Password, got.Database)
			}
			if got.Host != c.Host || got.Port != 5433 {
				t.Errorf("host/port = %q %d", got.Host, got.Port)
			}
		})
	}
}

func TestPostgresSSLMode(t *testing.T) {
	for in, want := range map[string]string{"": "prefer", "disable": "disable", "verify-full": "verify-full"} {
		c := ConnectionConfig{Type: PostgreSQL, Host: "h", User: "u", SSLMode: in}
		u, err := url.Parse(postgresDSN(c))
		if err != nil {
			t.Fatalf("url.Parse error = %v", err)
		}
		if got := u.Query().Get("sslmode"); got != want {
			t.Errorf("SSLMode %q -> sslmode %q, want %q", in, got, want)
		}
	}
}

func TestSQLServerDSN(t *testing.T) {
	for _, pw := range trickyPasswords {
		t.Run(pw, func(t *testing.T) {
			c := ConnectionConfig{Type: SQLServer, Host: "db.local", Port: 1434, User: "sa", Password: pw, Database: "shop"}
			dsn := sqlserverDSN(c)

			got, err := msdsn.Parse(dsn)
			if err != nil {
				t.Fatalf("msdsn.Parse(%q) error = %v", dsn, err)
			}
			if got.User != c.User || got.Password != pw || got.Database != c.Database {
				t.Errorf("round trip = user %q pass %q db %q", got.User, got.Password, got.Database)
			}
			if got.Host != c.Host || got.Port != 1434 {
				t.Errorf("host/port = %q %d", got.Host, got.Port)
			}
		})
	}
}

// go-mssqldb defaults to trusting any certificate when the DSN omits it,
// so false must be written out explicitly too.
func TestSQLServerTrustServerCertificate(t *testing.T) {
	for _, trust := range []bool{false, true} {
		c := ConnectionConfig{Type: SQLServer, Host: "h", User: "sa", TrustServerCertificate: trust}
		got, err := msdsn.Parse(sqlserverDSN(c))
		if err != nil {
			t.Fatalf("msdsn.Parse error = %v", err)
		}
		if got.TrustServerCertificate != trust {
			t.Errorf("TrustServerCertificate = %v, want %v", got.TrustServerCertificate, trust)
		}
	}
}

func TestDefaultPorts(t *testing.T) {
	my, _ := mysql.ParseDSN(mysqlDSN(ConnectionConfig{Type: MySQL, Host: "h", User: "u"}))
	if my == nil || !strings.HasSuffix(my.Addr, ":3306") {
		t.Errorf("mysql default addr = %v", my)
	}
	pg, _ := pgconn.ParseConfig(postgresDSN(ConnectionConfig{Type: PostgreSQL, Host: "h", User: "u"}))
	if pg == nil || pg.Port != 5432 {
		t.Errorf("postgres default port = %v", pg)
	}
	ms, _ := msdsn.Parse(sqlserverDSN(ConnectionConfig{Type: SQLServer, Host: "h", User: "u"}))
	if ms.Port != 1433 {
		t.Errorf("sqlserver default port = %d", ms.Port)
	}
}

func TestIPv6Host(t *testing.T) {
	my, err := mysql.ParseDSN(mysqlDSN(ConnectionConfig{Type: MySQL, Host: "::1", Port: 3306, User: "u"}))
	if err != nil || my.Addr != "[::1]:3306" {
		t.Errorf("mysql IPv6 addr = %v, %v", my, err)
	}
	pg, err := pgconn.ParseConfig(postgresDSN(ConnectionConfig{Type: PostgreSQL, Host: "::1", Port: 5432, User: "u"}))
	if err != nil || pg.Host != "::1" {
		t.Errorf("postgres IPv6 host = %v, %v", pg, err)
	}
}

// Opens a real file, so it also checks the driver name, pragma syntax and
// that characters meaningful in URLs survive in the file name.
func TestSQLiteDSN(t *testing.T) {
	for _, name := range []string{"shop.db", "my shop.db", "what?.db", "hash#1.db", "100%.db", "中文.db"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			driver, dsn, err := buildDSN(ConnectionConfig{Type: SQLite, FilePath: path})
			if err != nil {
				t.Fatalf("buildDSN() error = %v", err)
			}

			db, err := sql.Open(driver, dsn)
			if err != nil {
				t.Fatalf("sql.Open error = %v", err)
			}
			defer db.Close()

			var fk int
			if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
				t.Fatalf("PRAGMA foreign_keys error = %v (dsn %q)", err, dsn)
			}
			if fk != 1 {
				t.Errorf("foreign_keys = %d, want 1", fk)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("database file not created at %q (dsn %q): %v", path, dsn, err)
			}
		})
	}
}

func TestPostgresTLSFiles(t *testing.T) {
	c := ConnectionConfig{Type: PostgreSQL, Host: "h", User: "u", SSLMode: "verify-full",
		TLS: &TLSFiles{CACertPath: "/certs/ca 1.pem", ClientCertPath: "/certs/client.pem", ClientKeyPath: "/certs/client.key"}}
	u, err := url.Parse(postgresDSN(c))
	if err != nil {
		t.Fatalf("url.Parse error = %v", err)
	}
	q := u.Query()
	want := map[string]string{
		"sslmode":     "verify-full",
		"sslrootcert": "/certs/ca 1.pem",
		"sslcert":     "/certs/client.pem",
		"sslkey":      "/certs/client.key",
	}
	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}

	plain, _ := url.Parse(postgresDSN(ConnectionConfig{Type: PostgreSQL, Host: "h", User: "u"}))
	for _, k := range []string{"sslrootcert", "sslcert", "sslkey"} {
		if plain.Query().Has(k) {
			t.Errorf("%s set without TLS files", k)
		}
	}
}

func TestSQLServerCACert(t *testing.T) {
	c := ConnectionConfig{Type: SQLServer, Host: "h", User: "sa", TLS: &TLSFiles{CACertPath: "/certs/ca.pem"}}
	u, err := url.Parse(sqlserverDSN(c))
	if err != nil {
		t.Fatalf("url.Parse error = %v", err)
	}
	if got := u.Query().Get("certificate"); got != "/certs/ca.pem" {
		t.Errorf("certificate = %q, want /certs/ca.pem", got)
	}
}
