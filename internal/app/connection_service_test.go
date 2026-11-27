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
	return NewConnectionService(files, st, conn.NewPool()), path
}

func TestConnectionServiceRejectsUnpickedPath(t *testing.T) {
	s, path := newTestConnectionService(t)
	cfg := conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}
	ctx := context.Background()

	if _, err := s.TestConnection(ctx, ConnectionDraft{Config: cfg}); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("TestConnection(unpicked) = %v, want ErrPathNotPicked", err)
	}
	if _, err := s.ListDatabases(ctx, ConnectionDraft{Config: cfg}); !errors.Is(err, ErrPathNotPicked) {
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

	if _, err := s.TestConnection(ctx, ConnectionDraft{Config: cfg}); err != nil {
		t.Fatalf("TestConnection() = %v", err)
	}
	if s.pool.Len() != 0 {
		t.Error("TestConnection must not cache")
	}

	dbs, err := s.ListDatabases(ctx, ConnectionDraft{Config: cfg})
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
	_, err := s.TestConnection(context.Background(), ConnectionDraft{Config: conn.ConnectionConfig{Type: conn.MySQL}})
	if !errors.Is(err, conn.ErrMissingField) {
		t.Fatalf("TestConnection(invalid) = %v, want ErrMissingField", err)
	}
}

func TestSavedConnectionFlow(t *testing.T) {
	s, _ := newTestConnectionService(t)
	ctx := context.Background()

	path, _ := s.files.PickFile(FileSQLiteCreate)
	saved, err := s.SaveConnection(store.Connection{Name: "local", Config: conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}}, nil)
	if err != nil {
		t.Fatalf("SaveConnection() = %v", err)
	}

	if _, err := s.TestSavedConnection(ctx, saved.ID); err != nil {
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
	if _, err := s.TestSavedConnection(ctx, saved.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("TestSavedConnection(deleted) = %v, want ErrNotFound", err)
	}
}

func TestListSavedAllowsStoredPaths(t *testing.T) {
	s, _ := newTestConnectionService(t)
	path, _ := s.files.PickFile(FileSQLiteOpen)
	cfg := conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}
	if _, err := s.SaveConnection(store.Connection{Name: "x", Config: cfg}, nil); err != nil {
		t.Fatal(err)
	}

	// Simulate an app restart: same store, fresh allowlist.
	files, _ := newTestFileService("", nil)
	restarted := NewConnectionService(files, s.store, conn.NewPool())
	if _, err := restarted.TestConnection(context.Background(), ConnectionDraft{Config: cfg}); !errors.Is(err, ErrPathNotPicked) {
		t.Fatalf("before ListSaved: %v, want ErrPathNotPicked", err)
	}
	if _, err := restarted.ListSaved(); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.TestConnection(context.Background(), ConnectionDraft{Config: cfg}); err != nil {
		t.Fatalf("after ListSaved: %v, want nil", err)
	}
}

func TestSaveConnectionValidates(t *testing.T) {
	s, _ := newTestConnectionService(t)
	if _, err := s.SaveConnection(store.Connection{Name: "bad", Config: conn.ConnectionConfig{Type: conn.MySQL}}, store.AllSecrets()); !errors.Is(err, conn.ErrMissingField) {
		t.Errorf("SaveConnection(invalid) = %v, want ErrMissingField", err)
	}
	cfg := conn.ConnectionConfig{Type: conn.SQLite, FilePath: "/etc/passwd"}
	if _, err := s.SaveConnection(store.Connection{Name: "evil", Config: cfg}, store.AllSecrets()); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("SaveConnection(unpicked path) = %v, want ErrPathNotPicked", err)
	}
}

func TestSaveConnectionClosesOldPoolEntry(t *testing.T) {
	s, _ := newTestConnectionService(t)
	ctx := context.Background()
	path, _ := s.files.PickFile(FileSQLiteCreate)
	saved, _ := s.SaveConnection(store.Connection{Name: "a", Config: conn.ConnectionConfig{Type: conn.SQLite, FilePath: path}}, nil)
	if _, err := s.ListSavedDatabases(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}

	other := filepath.Join(filepath.Dir(path), "other.db")
	s.files.allow(other)
	saved.Config.FilePath = other
	if _, err := s.SaveConnection(saved, nil); err != nil {
		t.Fatal(err)
	}
	if s.pool.Len() != 0 {
		t.Errorf("old pooled connection not closed: Len() = %d", s.pool.Len())
	}
}

func TestDraftUsesStoredSecrets(t *testing.T) {
	s, _ := newTestConnectionService(t)
	stored := conn.ConnectionConfig{Type: conn.MySQL, Host: "db", User: "app", Password: "stored-pw",
		SSH: &conn.SSHTunnel{Host: "b", User: "u", Auth: conn.SSHAuthPassword, Password: "stored-ssh"}}
	saved, err := s.SaveConnection(store.Connection{Name: "x", Config: stored}, store.AllSecrets())
	if err != nil {
		t.Fatal(err)
	}

	// The form only knows the stripped config and edits the host.
	edited := saved.Config
	edited.Host = "db2"

	cfg, err := s.resolve(ConnectionDraft{ID: saved.ID, Config: edited})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "db2" || cfg.Password != "stored-pw" || cfg.SSH.Password != "stored-ssh" {
		t.Errorf("resolve() = host %q pw %q ssh %q", cfg.Host, cfg.Password, cfg.SSH.Password)
	}
	if edited.SSH.Password != "" {
		t.Error("resolve must not mutate the draft's SSH struct")
	}

	edited.Password = "typed-pw"
	cfg, _ = s.resolve(ConnectionDraft{ID: saved.ID, Config: edited, UpdateSecrets: []string{store.SecretPassword}})
	if cfg.Password != "typed-pw" || cfg.SSH.Password != "stored-ssh" {
		t.Errorf("only the password was typed: pw %q ssh %q", cfg.Password, cfg.SSH.Password)
	}

	if _, err := s.resolve(ConnectionDraft{ID: "ghost", Config: edited}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown id = %v, want ErrNotFound", err)
	}
}

func TestK8sDiscoveryChecksKubeconfigPath(t *testing.T) {
	s, _ := newTestConnectionService(t)
	ctx := context.Background()
	evil := "/etc/kubernetes/admin.conf"
	if _, err := s.K8sContexts(evil); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("K8sContexts = %v", err)
	}
	if _, err := s.K8sNamespaces(ctx, evil, ""); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("K8sNamespaces = %v", err)
	}
	if _, err := s.K8sTargets(ctx, evil, "", "db"); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("K8sTargets = %v", err)
	}
}
