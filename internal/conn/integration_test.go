//go:build integration

package conn

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"
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
			adminDB, err := open(ctx, admin)
			if err != nil {
				t.Fatalf("open(admin) = %v", err)
			}
			// Cleanups run LIFO after the test returns: drop the user, then close.
			t.Cleanup(func() { adminDB.Close() })
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
