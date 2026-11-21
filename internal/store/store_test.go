package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/lichenghsu/synccraft/internal/conn"
)

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) Set(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

func (s *memSecrets) Get(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[k], nil
}

func (s *memSecrets) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, k)
	return nil
}

func newTestStore(t *testing.T) (*Store, *memSecrets, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synccraft", "connections.json")
	sec := &memSecrets{m: map[string]string{}}
	return New(path, sec), sec, path
}

func pgWithSSH() conn.ConnectionConfig {
	return conn.ConnectionConfig{
		Type: conn.PostgreSQL, Host: "db", User: "app", Password: "db-secret", Database: "shop",
		SSH: &conn.SSHTunnel{Host: "bastion", User: "ops", Auth: conn.SSHAuthPrivateKey,
			PrivateKeyPath: "/home/ops/.ssh/id", Passphrase: "key-secret", Password: "ssh-secret"},
	}
}

func TestSaveCreatesAndKeepsSecretsOutOfFile(t *testing.T) {
	s, sec, path := newTestStore(t)
	cfg := pgWithSSH()

	saved, err := s.Save(Connection{Name: "  prod  ", Config: cfg}, nil)
	if err != nil {
		t.Fatalf("Save() = %v", err)
	}
	if saved.ID == "" || saved.Name != "prod" {
		t.Errorf("saved = %+v, want generated ID and trimmed name", saved)
	}
	if saved.Config.Password != "" || saved.Config.SSH.Password != "" || saved.Config.SSH.Passphrase != "" {
		t.Error("Save must return the connection without secrets")
	}
	if cfg.Password != "db-secret" || cfg.SSH.Passphrase != "key-secret" {
		t.Error("Save must not mutate the caller's config")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"db-secret", "key-secret", "ssh-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("connections file contains %q", secret)
		}
	}
	if sec.m[saved.ID+"/password"] != "db-secret" || sec.m[saved.ID+"/ssh-passphrase"] != "key-secret" {
		t.Errorf("secrets not in keychain: %v", sec.m)
	}
}

func TestFilePermissions(t *testing.T) {
	s, _, path := newTestStore(t)
	if _, err := s.Save(Connection{Name: "x", Config: pgWithSSH()}, AllSecrets()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o, want 600", perm)
	}
	di, _ := os.Stat(filepath.Dir(path))
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir mode = %o, want 700", perm)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".connections-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestListAndGet(t *testing.T) {
	s, _, _ := newTestStore(t)

	empty, err := s.List()
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("List() on missing file = %v, %v; want empty non-nil", empty, err)
	}

	saved, _ := s.Save(Connection{Name: "prod", Config: pgWithSSH()}, nil)
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List() = %v, %v", list, err)
	}
	if list[0].Config.Password != "" || list[0].Config.SSH.Passphrase != "" {
		t.Error("List must not return secrets")
	}

	full, err := s.Get(saved.ID)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if full.Password != "db-secret" || full.SSH.Password != "ssh-secret" || full.SSH.Passphrase != "key-secret" {
		t.Errorf("Get() secrets = %q %q %q", full.Password, full.SSH.Password, full.SSH.Passphrase)
	}
	if full.SSH.PrivateKeyPath != "/home/ops/.ssh/id" {
		t.Error("Get() lost non-secret fields")
	}

	if _, err := s.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(unknown) = %v, want ErrNotFound", err)
	}
}

func TestSaveUpdatesOnlyNamedSecrets(t *testing.T) {
	s, _, _ := newTestStore(t)
	saved, _ := s.Save(Connection{Name: "prod", Config: pgWithSSH()}, nil)

	// Frontend edits the name only: it has no secrets to send back.
	edit := saved
	edit.Name = "production"
	if _, err := s.Save(edit, nil); err != nil {
		t.Fatal(err)
	}
	full, _ := s.Get(saved.ID)
	if full.Password != "db-secret" || full.SSH.Password != "ssh-secret" {
		t.Errorf("empty update list lost secrets: %q %q", full.Password, full.SSH.Password)
	}

	// Only the DB password was retyped; the SSH secrets must survive.
	edit.Config.Password = "new-db"
	if _, err := s.Save(edit, []string{SecretPassword}); err != nil {
		t.Fatal(err)
	}
	full, _ = s.Get(saved.ID)
	if full.Password != "new-db" || full.SSH.Password != "ssh-secret" || full.SSH.Passphrase != "key-secret" {
		t.Errorf("after password update: %q %q %q", full.Password, full.SSH.Password, full.SSH.Passphrase)
	}

	// Explicitly clear the passphrase.
	if _, err := s.Save(edit, []string{SecretSSHPassphrase}); err != nil {
		t.Fatal(err)
	}
	full, _ = s.Get(saved.ID)
	if full.SSH.Passphrase != "" || full.Password != "new-db" {
		t.Errorf("after clearing passphrase: %q %q", full.SSH.Passphrase, full.Password)
	}
}

func TestRemovingTunnelDeletesSSHSecrets(t *testing.T) {
	s, sec, _ := newTestStore(t)
	saved, _ := s.Save(Connection{Name: "prod", Config: pgWithSSH()}, nil)

	edit := saved
	edit.Config.SSH = nil
	edit.Config.Password = "db-secret"
	if _, err := s.Save(edit, AllSecrets()); err != nil {
		t.Fatal(err)
	}
	if _, ok := sec.m[saved.ID+"/ssh-passphrase"]; ok {
		t.Error("ssh secrets should be deleted when the tunnel is removed")
	}
}

func TestSaveErrors(t *testing.T) {
	s, _, _ := newTestStore(t)
	if _, err := s.Save(Connection{Name: "  ", Config: pgWithSSH()}, nil); !errors.Is(err, ErrMissingName) {
		t.Errorf("blank name = %v, want ErrMissingName", err)
	}
	if _, err := s.Save(Connection{ID: "ghost", Name: "x", Config: pgWithSSH()}, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown ID = %v, want ErrNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	s, sec, _ := newTestStore(t)
	a, _ := s.Save(Connection{Name: "a", Config: pgWithSSH()}, nil)
	b, _ := s.Save(Connection{Name: "b", Config: pgWithSSH()}, nil)

	if err := s.Delete(a.ID); err != nil {
		t.Fatalf("Delete() = %v", err)
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].ID != b.ID {
		t.Errorf("List() after delete = %v", list)
	}
	for k := range sec.m {
		if strings.HasPrefix(k, a.ID+"/") {
			t.Errorf("secret %q not deleted", k)
		}
	}
	if err := s.Delete(a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete = %v, want ErrNotFound", err)
	}
}

func TestLoadErrorsDoNotOverwrite(t *testing.T) {
	s, _, path := newTestStore(t)
	os.MkdirAll(filepath.Dir(path), 0o700)

	for name, content := range map[string]string{
		"corrupt":       "{not json",
		"future format": `{"version": 2, "connections": []}`,
	} {
		t.Run(name, func(t *testing.T) {
			os.WriteFile(path, []byte(content), 0o600)
			if _, err := s.List(); err == nil {
				t.Fatal("List() should fail")
			}
			if _, err := s.Save(Connection{Name: "x", Config: pgWithSSH()}, AllSecrets()); err == nil {
				t.Fatal("Save() should fail instead of overwriting the file")
			}
			raw, _ := os.ReadFile(path)
			if string(raw) != content {
				t.Error("file was overwritten")
			}
		})
	}
	if _, err := s.List(); !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("future format error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestKeyringAdapter(t *testing.T) {
	keyring.MockInit()
	k := Keyring{Service: "synccraft-test"}

	if v, err := k.Get("missing"); err != nil || v != "" {
		t.Errorf("Get(missing) = %q, %v; want empty, nil", v, err)
	}
	if err := k.Delete("missing"); err != nil {
		t.Errorf("Delete(missing) = %v, want nil", err)
	}
	if err := k.Set("id/password", "pw"); err != nil {
		t.Fatal(err)
	}
	if v, _ := k.Get("id/password"); v != "pw" {
		t.Errorf("Get() = %q, want pw", v)
	}
}
