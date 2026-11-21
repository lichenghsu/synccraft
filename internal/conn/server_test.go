package conn

import (
	"context"
	"testing"
)

func TestParseServerInfo(t *testing.T) {
	tests := []struct {
		typ              DBType
		raw              string
		product, version string
	}{
		{MySQL, "8.4.3", ProductMySQL, "8.4.3"},
		{MySQL, "8.0.36-0ubuntu0.22.04.1", ProductMySQL, "8.0.36"},
		{MySQL, "10.11.6-MariaDB-0+deb12u2", ProductMariaDB, "10.11.6"},
		{MySQL, "11.4.2-MariaDB-ubu2404-log", ProductMariaDB, "11.4.2"},
		{PostgreSQL, "17.12 (Debian 17.12-1.pgdg13+1)", ProductPostgreSQL, "17.12"},
		{SQLServer, "16.0.4215.2 Developer Edition (64-bit)", ProductSQLServer, "16.0.4215.2"},
		{SQLite, "3.50.4", ProductSQLite, "3.50.4"},
	}
	for _, tt := range tests {
		got := parseServerInfo(tt.typ, tt.raw)
		if got.Product != tt.product || got.Version != tt.version || got.Raw != tt.raw {
			t.Errorf("parseServerInfo(%s, %q) = %+v, want %s %s", tt.typ, tt.raw, got, tt.product, tt.version)
		}
	}
}

func TestProbeSQLite(t *testing.T) {
	info, err := Probe(context.Background(), sqliteConfig(t, "probe.db"))
	if err != nil {
		t.Fatalf("Probe() = %v", err)
	}
	if info.Product != ProductSQLite || info.Version == "" {
		t.Errorf("Probe() = %+v", info)
	}
}
