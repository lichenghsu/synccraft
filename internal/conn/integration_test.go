//go:build integration

package conn

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Matches dev/docker-compose.yml. Run:
//
//	podman compose -f dev/docker-compose.yml up -d
//	go test -tags integration ./internal/conn/...
var composeDBs = map[string]ConnectionConfig{
	"mysql":     {Type: MySQL, Host: "127.0.0.1", Port: 3306, User: "root", Password: "root", Database: "shop"},
	"postgres":  {Type: PostgreSQL, Host: "127.0.0.1", Port: 5432, User: "postgres", Password: "postgres", Database: "shop", SSLMode: "disable"},
	"sqlserver": {Type: SQLServer, Host: "127.0.0.1", Port: 1433, User: "sa", Password: "Passw0rd!", TrustServerCertificate: true},
}

const trickyPassword = `p@ss/w;rd 'x'`

func itContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestIntegrationListDatabases(t *testing.T) {
	want := map[string]string{"mysql": "shop", "postgres": "shop", "sqlserver": "synccraft_it"}

	p := NewPool()
	t.Cleanup(func() { p.CloseAll() })

	for name, c := range composeDBs {
		t.Run(name, func(t *testing.T) {
			ctx := itContext(t)
			if err := Ping(ctx, c); err != nil {
				t.Fatalf("Ping() = %v (is dev/docker-compose.yml up?)", err)
			}
			db, err := p.Get(ctx, c)
			if err != nil {
				t.Fatalf("Get() = %v", err)
			}
			if c.Type == SQLServer {
				mustExec(t, db, `IF DB_ID('synccraft_it') IS NULL CREATE DATABASE synccraft_it`)
			}

			got, err := ListDatabases(ctx, db, c.Type)
			if err != nil {
				t.Fatalf("ListDatabases() = %v", err)
			}
			if !slices.Contains(got, want[name]) {
				t.Errorf("ListDatabases() = %v, want it to contain %q", got, want[name])
			}
			for _, sys := range []string{"mysql", "information_schema", "template0", "master", "tempdb"} {
				if slices.Contains(got, sys) {
					t.Errorf("system database %q should be hidden", sys)
				}
			}
		})
	}
}

// Logs in with a password that breaks naive DSN concatenation.
func TestIntegrationTrickyPassword(t *testing.T) {
	setup := map[string][2]string{
		"mysql": {
			`CREATE USER IF NOT EXISTS 'synccraft_it'@'%' IDENTIFIED BY 'p@ss/w;rd ''x'''`,
			`DROP USER IF EXISTS 'synccraft_it'@'%'`,
		},
		"postgres": {
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'synccraft_it') THEN
					CREATE ROLE synccraft_it LOGIN PASSWORD 'p@ss/w;rd ''x''';
				END IF;
			END $$`,
			`DROP ROLE IF EXISTS synccraft_it`,
		},
	}

	for name, sqls := range setup {
		t.Run(name, func(t *testing.T) {
			ctx := itContext(t)
			admin := composeDBs[name]
			adminConn, err := open(ctx, admin)
			if err != nil {
				t.Fatalf("open(admin) = %v", err)
			}
			adminDB := adminConn.db
			// Cleanups run LIFO after the test returns: drop the user, then close.
			t.Cleanup(func() { adminConn.Close() })
			mustExec(t, adminDB, sqls[0])
			t.Cleanup(func() {
				if _, err := adminDB.Exec(sqls[1]); err != nil {
					t.Errorf("cleanup %q: %v", sqls[1], err)
				}
			})

			user := admin
			user.User, user.Password = "synccraft_it", trickyPassword
			// The temp user has no grants: MySQL then needs no default database,
			// PostgreSQL lets any role connect to "postgres".
			user.Database = ""
			if name == "postgres" {
				user.Database = "postgres"
			}
			if err := Ping(ctx, user); err != nil {
				t.Fatalf("Ping(tricky password) = %v", err)
			}
		})
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

const bastionAddr = "127.0.0.1:2222"

// bastionKnownHosts records the bastion's host key (trust on first use, test only).
func bastionKnownHosts(t *testing.T) string {
	t.Helper()
	var key ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User: "probe",
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			key = k
			return errors.New("captured")
		},
		Timeout: 5 * time.Second,
	}
	ssh.Dial("tcp", bastionAddr, cfg)
	if key == nil {
		t.Fatalf("no host key from %s (is ssh-bastion in dev/docker-compose.yml up?)", bastionAddr)
	}
	path := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(bastionAddr)}, key)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Reaches each database by its compose service name, which only resolves on the bastion.
func TestIntegrationSSHTunnel(t *testing.T) {
	kh := bastionKnownHosts(t)
	tunnel := &SSHTunnel{Host: "127.0.0.1", Port: 2222, User: "ops", Auth: SSHAuthPassword, Password: "ops-pass", KnownHostsPath: kh}

	behind := map[string]struct {
		host string
		port int
		want string
	}{
		"mysql":     {"mysql-src", 3306, "shop"},
		"postgres":  {"pg-src", 5432, "shop"},
		"sqlserver": {"mssql", 1433, "synccraft_it"},
	}

	p := NewPool()
	t.Cleanup(func() { p.CloseAll() })

	for name, b := range behind {
		t.Run(name, func(t *testing.T) {
			ctx := itContext(t)
			c := composeDBs[name]
			c.Host, c.Port, c.SSH = b.host, b.port, tunnel

			if err := Ping(ctx, c); err != nil {
				t.Fatalf("Ping() through tunnel = %v", err)
			}
			db, err := p.Get(ctx, c)
			if err != nil {
				t.Fatalf("Get() = %v", err)
			}
			if c.Type == SQLServer {
				mustExec(t, db, `IF DB_ID('synccraft_it') IS NULL CREATE DATABASE synccraft_it`)
			}
			got, err := ListDatabases(ctx, db, c.Type)
			if err != nil || !slices.Contains(got, b.want) {
				t.Fatalf("ListDatabases() = %v, %v; want %q", got, err, b.want)
			}
		})
	}

	t.Run("wrong password", func(t *testing.T) {
		c := composeDBs["mysql"]
		bad := *tunnel
		bad.Password = "nope"
		c.Host, c.SSH = "mysql-src", &bad
		if err := Ping(itContext(t), c); !errors.Is(err, ErrSSHAuthFailed) {
			t.Fatalf("err = %v, want ErrSSHAuthFailed", err)
		}
	})
}

// A CA that did not sign the server's certificate must fail the handshake,
// which proves the TLS files are actually used.
func TestIntegrationMySQLTLS(t *testing.T) {
	ca, _, _ := writeTestCerts(t)
	c := composeDBs["mysql"]
	c.TLS = &TLSFiles{CACertPath: ca}
	if err := Ping(itContext(t), c); !errors.Is(err, ErrUntrustedCertificate) {
		t.Fatalf("err = %v, want ErrUntrustedCertificate", err)
	}
}

// The bastion mounts the databases' socket directories (see dev/docker-compose.yml).
func TestIntegrationSocketOverSSH(t *testing.T) {
	kh := bastionKnownHosts(t)
	tunnel := &SSHTunnel{Host: "127.0.0.1", Port: 2222, User: "ops", Auth: SSHAuthPassword, Password: "ops-pass", KnownHostsPath: kh}

	cases := map[string]ConnectionConfig{
		// root@localhost: a socket login counts as localhost.
		"mysql": {Type: MySQL, User: "root", Password: "root", Database: "shop", SocketPath: "/run/mysql-src/mysqld.sock", SSH: tunnel},
		// The postgres image trusts local socket connections, so no password.
		"postgres": {Type: PostgreSQL, User: "postgres", Database: "shop", SocketPath: "/run/pg-src", SSH: tunnel},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := itContext(t)
			o, err := open(ctx, c)
			if err != nil {
				t.Fatalf("open() over socket = %v", err)
			}
			defer o.Close()
			got, err := ListDatabases(ctx, o.db, c.Type)
			if err != nil || !slices.Contains(got, "shop") {
				t.Fatalf("ListDatabases() = %v, %v", got, err)
			}
		})
	}
}

func TestIntegrationDetectServer(t *testing.T) {
	want := map[string]string{"mysql": ProductMySQL, "postgres": ProductPostgreSQL, "sqlserver": ProductSQLServer}
	for name, c := range composeDBs {
		t.Run(name, func(t *testing.T) {
			info, err := Probe(itContext(t), c)
			if err != nil || info.Product != want[name] || info.Version == "" {
				t.Fatalf("Probe() = %+v, %v; want %s", info, err, want[name])
			}
			t.Logf("%s %s (%s)", info.Product, info.Version, info.Raw)
		})
	}
}

func TestIntegrationDetectSocket(t *testing.T) {
	kh := bastionKnownHosts(t)
	tunnel := &SSHTunnel{Host: "127.0.0.1", Port: 2222, User: "ops", Auth: SSHAuthPassword, Password: "ops-pass", KnownHostsPath: kh}

	// The bastion mounts the sockets at non-standard paths; append them after the real defaults.
	saved := socketCandidates
	t.Cleanup(func() { socketCandidates = saved })
	socketCandidates = map[DBType][]string{
		MySQL:      append(slices.Clone(saved[MySQL]), "/run/mysql-src/mysqld.sock"),
		PostgreSQL: append(slices.Clone(saved[PostgreSQL]), "/run/pg-src"),
	}

	for typ, want := range map[DBType]string{MySQL: "/run/mysql-src/mysqld.sock", PostgreSQL: "/run/pg-src"} {
		got, err := DetectSocket(itContext(t), ConnectionConfig{Type: typ, SSH: tunnel})
		if err != nil || got != want {
			t.Errorf("%s: DetectSocket() = %q, %v; want %q", typ, got, err, want)
		}
	}
}
