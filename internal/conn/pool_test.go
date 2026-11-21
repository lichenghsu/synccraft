package conn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// SQLite needs no server, so it exercises the pool without docker.
func sqliteConfig(t *testing.T, name string) ConnectionConfig {
	t.Helper()
	return ConnectionConfig{Type: SQLite, FilePath: filepath.Join(t.TempDir(), name)}
}

func TestPoolCachesPerConfig(t *testing.T) {
	p := NewPool()
	defer p.CloseAll()
	ctx := context.Background()
	a, b := sqliteConfig(t, "a.db"), sqliteConfig(t, "b.db")

	dbA1, err := p.Get(ctx, a)
	if err != nil {
		t.Fatalf("Get(a) error = %v", err)
	}
	dbA2, _ := p.Get(ctx, a)
	dbB, _ := p.Get(ctx, b)

	if dbA1 != dbA2 {
		t.Error("same config should return the cached DB")
	}
	if dbA1 == dbB {
		t.Error("different configs must not share a DB")
	}
	if p.Len() != 2 {
		t.Errorf("Len() = %d, want 2", p.Len())
	}
	if got := dbA1.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("sqlite MaxOpenConnections = %d, want 1", got)
	}
}

func TestPoolConcurrentGet(t *testing.T) {
	p := NewPool()
	defer p.CloseAll()
	c := sqliteConfig(t, "race.db")

	const n = 16
	got := make([]any, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := p.Get(context.Background(), c)
			if err != nil {
				t.Error(err)
			}
			got[i] = db
		}()
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if got[i] != got[0] {
			t.Fatal("concurrent Get returned different DBs for one config")
		}
	}
	if p.Len() != 1 {
		t.Errorf("Len() = %d, want 1", p.Len())
	}
}

func TestPoolClose(t *testing.T) {
	p := NewPool()
	ctx := context.Background()
	a, b := sqliteConfig(t, "a.db"), sqliteConfig(t, "b.db")
	dbA, _ := p.Get(ctx, a)
	dbB, _ := p.Get(ctx, b)

	if err := p.Close(a); err != nil {
		t.Fatalf("Close(a) error = %v", err)
	}
	if err := dbA.Ping(); err == nil {
		t.Error("closed DB should fail to ping")
	}
	if err := p.Close(a); err != nil {
		t.Errorf("second Close(a) = %v, want nil", err)
	}

	if err := p.CloseAll(); err != nil {
		t.Fatalf("CloseAll() error = %v", err)
	}
	if err := dbB.Ping(); err == nil {
		t.Error("CloseAll should close every DB")
	}
	if p.Len() != 0 {
		t.Errorf("Len() after CloseAll = %d, want 0", p.Len())
	}
}

func TestPoolDoesNotCacheFailures(t *testing.T) {
	p := NewPool()
	defer p.CloseAll()
	bad := ConnectionConfig{Type: SQLite, FilePath: filepath.Join(t.TempDir(), "missing-dir", "x.db")}

	if _, err := p.Get(context.Background(), bad); err == nil {
		t.Fatal("Get(unopenable) should fail")
	}
	if p.Len() != 0 {
		t.Errorf("failed Get was cached: Len() = %d", p.Len())
	}
}

func TestPoolRejectsInvalidConfig(t *testing.T) {
	p := NewPool()
	if _, err := p.Get(context.Background(), ConnectionConfig{Type: MySQL}); !errors.Is(err, ErrMissingField) {
		t.Fatalf("Get(invalid) error = %v, want ErrMissingField", err)
	}
}

func TestPingDoesNotCache(t *testing.T) {
	c := sqliteConfig(t, "ping.db")
	if err := Ping(context.Background(), c); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestPingHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Ping(ctx, sqliteConfig(t, "cancel.db")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ping(cancelled) = %v, want context.Canceled", err)
	}
}

func TestListDatabasesSQLite(t *testing.T) {
	p := NewPool()
	defer p.CloseAll()
	db, err := p.Get(context.Background(), sqliteConfig(t, "list.db"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := ListDatabases(context.Background(), db, SQLite)
	if err != nil {
		t.Fatalf("ListDatabases() error = %v", err)
	}
	if len(got) != 1 || got[0] != "main" {
		t.Errorf("ListDatabases() = %v, want [main]", got)
	}

	if _, err := ListDatabases(context.Background(), db, "oracle"); !errors.Is(err, ErrUnsupportedType) {
		t.Errorf("unknown type error = %v", err)
	}
}

func TestExplainTLSError(t *testing.T) {
	flattened := errors.New("TLS Handshake failed: tls: failed to verify certificate: x509: cannot validate certificate for 127.0.0.1 because it doesn't contain any IP SANs")
	typed := &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}

	for name, in := range map[string]error{"flattened": flattened, "typed": typed} {
		got := explainTLSError(SQLServer, in)
		if !errors.Is(got, ErrUntrustedCertificate) || !errors.Is(got, in) {
			t.Errorf("%s: %v should wrap both ErrUntrustedCertificate and the original", name, got)
		}
		if !strings.Contains(got.Error(), "Trust server certificate") {
			t.Errorf("%s: SQL Server hint missing: %v", name, got)
		}
	}
	if got := explainTLSError(PostgreSQL, flattened); !strings.Contains(got.Error(), "SSL mode") {
		t.Errorf("postgres hint missing: %v", got)
	}

	other := errors.New("mssql: Login failed for user 'sa'.")
	if got := explainTLSError(SQLServer, other); got != other {
		t.Errorf("non-TLS error changed: %v", got)
	}
}

func TestExplainSocketAuth(t *testing.T) {
	denied := &mysql.MySQLError{Number: 1698, Message: "Access denied for user 'root'@'localhost'"}

	got := explain(ConnectionConfig{Type: MySQL}, denied)
	if !errors.Is(got, ErrSocketAuthOnly) || !strings.Contains(got.Error(), "Unix socket") {
		t.Errorf("tcp: %v", got)
	}
	var myErr *mysql.MySQLError
	if !errors.As(got, &myErr) {
		t.Error("original MySQL error lost")
	}

	got = explain(ConnectionConfig{Type: MySQL, SocketPath: "/run/mysqld/mysqld.sock"}, denied)
	if !strings.Contains(got.Error(), "OS user") {
		t.Errorf("socket: %v", got)
	}

	other := &mysql.MySQLError{Number: 1045, Message: "Access denied (using password: YES)"}
	if got := explain(ConnectionConfig{Type: MySQL}, other); got != error(other) {
		t.Errorf("1045 should pass through, got %v", got)
	}
}
