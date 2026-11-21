package app

import (
	"errors"
	"testing"

	"github.com/lichenghsu/synccraft/internal/conn"
)

func newTestFileService(result string, err error) (*FileService, *[]dialogSpec) {
	var seen []dialogSpec
	s := NewFileService()
	s.prompt = func(spec dialogSpec) (string, error) {
		seen = append(seen, spec)
		return result, err
	}
	return s, &seen
}

func TestPickFileAllowsChosenPath(t *testing.T) {
	s, seen := newTestFileService("/data/shop.db", nil)

	got, err := s.PickFile(FileSQLiteOpen)
	if err != nil || got != "/data/shop.db" {
		t.Fatalf("PickFile() = %q, %v", got, err)
	}
	if len(*seen) != 1 || (*seen)[0].save {
		t.Errorf("expected one open dialog, got %+v", *seen)
	}

	c := conn.ConnectionConfig{Type: conn.SQLite, FilePath: got}
	if err := s.checkPaths(&c); err != nil {
		t.Errorf("checkPaths(picked) = %v, want nil", err)
	}
}

func TestPickFileCreateUsesSaveDialog(t *testing.T) {
	s, seen := newTestFileService("/data/new.db", nil)
	if _, err := s.PickFile(FileSQLiteCreate); err != nil {
		t.Fatal(err)
	}
	if !(*seen)[0].save {
		t.Error("sqliteCreate should use a save dialog")
	}
}

func TestPickFileCancelled(t *testing.T) {
	s, _ := newTestFileService("", nil)
	got, err := s.PickFile(FileCACert)
	if err != nil || got != "" {
		t.Fatalf("PickFile(cancelled) = %q, %v; want empty, nil", got, err)
	}
	if len(s.allowed) != 0 {
		t.Error("cancelled dialog must not allow anything")
	}
}

func TestPickFileErrors(t *testing.T) {
	s, seen := newTestFileService("/x", nil)
	if _, err := s.PickFile("passwd"); !errors.Is(err, ErrUnknownFileKind) {
		t.Errorf("unknown kind error = %v", err)
	}
	if len(*seen) != 0 {
		t.Error("unknown kind must not open a dialog")
	}

	boom := errors.New("boom")
	s, _ = newTestFileService("", boom)
	if _, err := s.PickFile(FileClientKey); !errors.Is(err, boom) {
		t.Errorf("dialog error = %v, want boom", err)
	}

	s, _ = newTestFileService("relative/key.pem", nil)
	if _, err := s.PickFile(FileClientKey); !errors.Is(err, conn.ErrInvalidPath) {
		t.Errorf("relative path error = %v, want ErrInvalidPath", err)
	}
}

func TestPickFileCleansPath(t *testing.T) {
	s, _ := newTestFileService("/home/u/.ssh/../.ssh/id_ed25519", nil)
	got, err := s.PickFile(FileSSHPrivateKey)
	if err != nil || got != "/home/u/.ssh/id_ed25519" {
		t.Fatalf("PickFile() = %q, %v", got, err)
	}
}

func TestCheckPathsRejectsUnpicked(t *testing.T) {
	s, _ := newTestFileService("/certs/ca.pem", nil)
	if _, err := s.PickFile(FileCACert); err != nil {
		t.Fatal(err)
	}

	c := conn.ConnectionConfig{
		Type: conn.PostgreSQL, Host: "h", User: "u",
		TLS: &conn.TLSFiles{CACertPath: "/certs/ca.pem", ClientCertPath: "/etc/shadow", ClientKeyPath: "/certs/c.key"},
	}
	if err := s.checkPaths(&c); !errors.Is(err, ErrPathNotPicked) {
		t.Fatalf("checkPaths() = %v, want ErrPathNotPicked", err)
	}

	s.allow("/etc/shadow", "/certs/c.key") // e.g. paths loaded from the saved store
	if err := s.checkPaths(&c); err != nil {
		t.Fatalf("checkPaths() after allow = %v, want nil", err)
	}
}

func TestCheckPathsNoFiles(t *testing.T) {
	s := NewFileService()
	c := conn.ConnectionConfig{Type: conn.MySQL, Host: "h", User: "u"}
	if err := s.checkPaths(&c); err != nil {
		t.Fatalf("checkPaths(no files) = %v, want nil", err)
	}
}

func TestEveryFileKindHasDialog(t *testing.T) {
	kinds := []FileKind{FileSQLiteOpen, FileSQLiteCreate, FileCACert, FileClientCert, FileClientKey, FileSSHPrivateKey, FileKnownHosts, FileKubeconfig}
	for _, k := range kinds {
		spec, ok := dialogSpecs[k]
		if !ok || spec.title == "" || len(spec.filters) == 0 {
			t.Errorf("FileKind %q has no complete dialog spec", k)
		}
	}
	if len(dialogSpecs) != len(kinds) {
		t.Errorf("dialogSpecs has %d kinds, test lists %d", len(dialogSpecs), len(kinds))
	}
}

func TestCheckFile(t *testing.T) {
	s, _ := newTestFileService("/home/u/.kube/config", nil)
	if err := s.checkFile(""); err != nil {
		t.Errorf("empty path = %v, want nil (default kubeconfig)", err)
	}
	if err := s.checkFile("/home/u/.kube/config"); !errors.Is(err, ErrPathNotPicked) {
		t.Errorf("unpicked = %v", err)
	}
	if _, err := s.PickFile(FileKubeconfig); err != nil {
		t.Fatal(err)
	}
	if err := s.checkFile("/home/u/.kube/config"); err != nil {
		t.Errorf("picked = %v", err)
	}
}
