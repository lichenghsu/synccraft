package conn

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
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

func TestNotSupportedYet(t *testing.T) {
	ssh := ConnectionConfig{Type: PostgreSQL, Host: "h", User: "u",
		SSH: &SSHTunnel{Host: "bastion", User: "ops", Auth: SSHAuthAgent}}
	mysqlTLS := ConnectionConfig{Type: MySQL, Host: "h", User: "u", TLS: &TLSFiles{CACertPath: "/ca.pem"}}

	for name, c := range map[string]ConnectionConfig{"ssh": ssh, "mysql tls": mysqlTLS} {
		if err := Ping(context.Background(), c); !errors.Is(err, ErrNotSupportedYet) {
			t.Errorf("%s: Ping() = %v, want ErrNotSupportedYet", name, err)
		}
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
