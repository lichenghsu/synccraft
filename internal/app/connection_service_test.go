package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/lichenghsu/synccraft/internal/conn"
	"github.com/lichenghsu/synccraft/internal/store"
)

func newTestConnectionService(t *testing.T) (*ConnectionService, string) {
	t.Helper()
	keyring.MockInit()
	dir := t.TempDir()
	path := filepath.Join(dir, "shop.db")
	files, _ := newTestFileService(path, nil)
	st := store.New(filepath.Join(dir, "config", "connections.json"), store.Keyring{Service: "synccraft-test"})
	return NewConnectionService(files, st), path
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

func TestSavedConnectionFlow(t *testing.T) {
	s, _ := newTestConnectionService(t)
	ctx := context.Background()

	path, _ := s.files.PickFile(FileSQLiteCreate)
	saved, err := s.SaveConnection(store.Connection{Name: "local", Config: conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}}, false)
	if err != nil {
		t.Fatalf("SaveConnection() = %v", err)
	}

	if err := s.TestSavedConnection(ctx, saved.ID); err != nil {
		t.Fatalf("TestSavedConnection() = %v", err)
	}
	dbs, err := s.ListSavedDatabases(ctx, saved.ID)
	if err != nil || len(dbs) != 1 {
		t.Fatalf("ListSavedDatabases() = %v, %v", dbs, err)
	}

	list, err := s.ListSaved()
	if err != nil || len(list) != 1 || list[0].Name != "local" {
		t.Fatalf("ListSaved() = %v, %v", list, err)
	}

	if err := s.DeleteConnection(saved.ID); err != nil {
		t.Fatalf("DeleteConnection() = %v", err)
	}
	if s.pool.Len() != 0 {
		t.Error("DeleteConnection should close the pooled connection")
	}
	if err := s.TestSavedConnection(ctx, saved.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("TestSavedConnection(deleted) = %v, want ErrNotFound", err)
	}
}

func TestListSavedAllowsStoredPaths(t *testing.T) {
	s, _ := newTestConnectionService(t)
	path, _ := s.files.PickFile(FileSQLiteOpen)
	cfg := conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}
	if _, err := s.SaveConnection(store.Connection{Name: "x", Config: cfg}, false); err != nil {
		t.Fatal(err)
	}

	// Simulate an app restart: same store, fresh allowlist.
	files, _ := newTestFileService("", nil)
	restarted := NewConnectionService(files, s.store)
	if err := restarted.TestConnection(context.Background(), cfg); !errors.Is(err, ErrPathNotPicked) {
		t.Fatalf("before ListSaved: %v, want ErrPathNotPicked", err)
	}
	if _, err := restarted.ListSaved(); err != nil {
		t.Fatal(err)
	}
	if err := restarted.TestConnection(context.Background(), cfg); err != nil {
		t.Fatalf("after ListSaved: %v, want nil", err)
	}
}

func TestSaveConnectionValidates(t *testing.T) {
	s, _ := newTestConnectionService(t)
	if _, err := s.SaveConnection(store.Connection{Name: "bad", Config: conn.ConnectionConfig{Type: conn.MySQL}}, true); !errors.Is(err, conn.ErrMissingField) {
		t.Errorf("SaveConnection(invalid) = %v, want ErrMissingField", err)
	}
	cfg := conn.ConnectionConfig{Type: conn.SQLite, FilePath: "/etc/passwd"}
	if _, err := s.SaveConnection(store.Connection{Name: "evil", Config: cfg}, true); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("SaveConnection(unpicked path) = %v, want ErrPathNotPicked", err)
	}
}

func TestSaveConnectionClosesOldPoolEntry(t *testing.T) {
	s, _ := newTestConnectionService(t)
	ctx := context.Background()
	path, _ := s.files.PickFile(FileSQLiteCreate)
	saved, _ := s.SaveConnection(store.Connection{Name: "a", Config: conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}}, false)
	if _, err := s.ListSavedDatabases(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}

	other := filepath.Join(filepath.Dir(path), "other.db")
	s.files.allow(other)
	saved.Config.FilePath = other
	if _, err := s.SaveConnection(saved, false); err != nil {
		t.Fatal(err)
	}
	if s.pool.Len() != 0 {
		t.Errorf("old pooled connection not closed: Len() = %d", s.pool.Len())
	}
}
