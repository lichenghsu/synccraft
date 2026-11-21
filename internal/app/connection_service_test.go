package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lichenghsu/synccraft/internal/conn"
)

func newTestConnectionService(t *testing.T) (*ConnectionService, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	files, _ := newTestFileService(path, nil)
	return NewConnectionService(files), path
}

func TestConnectionServiceRejectsUnpickedPath(t *testing.T) {
	s, path := newTestConnectionService(t)
	cfg := conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}
	ctx := context.Background()

	if err := s.TestConnection(ctx, cfg); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("TestConnection(unpicked) = %v, want ErrPathNotPicked", err)
	}
	if _, err := s.ListDatabases(ctx, cfg); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("ListDatabases(unpicked) = %v, want ErrPathNotPicked", err)
	}
	if s.pool.Len() != 0 {
		t.Error("rejected config must not reach the pool")
	}
}

func TestConnectionServiceSQLite(t *testing.T) {
	s, _ := newTestConnectionService(t)
	ctx := context.Background()

	path, err := s.files.PickFile(FileSQLiteCreate)
	if err != nil {
		t.Fatal(err)
	}
	cfg := conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}

	if err := s.TestConnection(ctx, cfg); err != nil {
		t.Fatalf("TestConnection() = %v", err)
	}
	if s.pool.Len() != 0 {
		t.Error("TestConnection must not cache")
	}

	dbs, err := s.ListDatabases(ctx, cfg)
	if err != nil || len(dbs) != 1 || dbs[0] != "main" {
		t.Fatalf("ListDatabases() = %v, %v; want [main]", dbs, err)
	}
	if s.pool.Len() != 1 {
		t.Errorf("ListDatabases should cache: Len() = %d", s.pool.Len())
	}

	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown() = %v", err)
	}
	if s.pool.Len() != 0 {
		t.Error("ServiceShutdown should close the pool")
	}
}

func TestConnectionServiceNoFilesSkipsCheck(t *testing.T) {
	s, _ := newTestConnectionService(t)
	// No file paths: the path check passes and the error comes from validation.
	err := s.TestConnection(context.Background(), conn.ConnectionConfig{Type: conn.MySQL})
	if !errors.Is(err, conn.ErrMissingField) {
		t.Fatalf("TestConnection(invalid) = %v, want ErrMissingField", err)
	}
}
