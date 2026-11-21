// Package store persists saved connections: settings in a JSON file,
// secrets in the OS keychain.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/lichenghsu/synccraft/internal/conn"
)

const fileVersion = 1

var (
	ErrNotFound           = errors.New("saved connection not found")
	ErrMissingName        = errors.New("connection name is required")
	ErrUnsupportedVersion = errors.New("unsupported connections file version")
)

// Connection is a saved connection. Config never carries secrets outside this package.
type Connection struct {
	ID     string                `json:"id"`
	Name   string                `json:"name"`
	Config conn.ConnectionConfig `json:"config"`
}

type fileData struct {
	Version     int          `json:"version"`
	Connections []Connection `json:"connections"`
}

// Store reads and writes the connections file. Safe for concurrent use.
type Store struct {
	mu      sync.Mutex
	path    string
	secrets Secrets
}

func New(path string, secrets Secrets) *Store {
	return &Store{path: path, secrets: secrets}
}

// DefaultPath returns <user config dir>/synccraft/connections.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "synccraft", "connections.json"), nil
}

// List returns all saved connections without secrets. Never nil.
func (s *Store) List() ([]Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(data.Connections))
	for _, c := range data.Connections {
		c.Config = withoutSecrets(c.Config)
		out = append(out, c)
	}
	return out, nil
}

// Get returns the full config for id, secrets included. Backend use only.
func (s *Store) Get(id string) (conn.ConnectionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return conn.ConnectionConfig{}, err
	}
	i := indexOf(data.Connections, id)
	if i < 0 {
		return conn.ConnectionConfig{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}

	cfg := withoutSecrets(data.Connections[i].Config)
	for _, f := range secretFields(&cfg) {
		v, err := s.secrets.Get(secretKey(id, f.name))
		if err != nil {
			return conn.ConnectionConfig{}, fmt.Errorf("read %s from keychain: %w", f.name, err)
		}
		*f.ptr = v
	}
	return cfg, nil
}

// Save creates (empty ID) or updates a connection and returns it without secrets.
// When creating, every secret in c.Config is written. When updating, only the
// secrets named in update are written (an empty value deletes it); the others
// keep their stored value. SSH secrets are always deleted when the tunnel is removed.
func (s *Store) Save(c Connection, update []string) (Connection, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return Connection{}, ErrMissingName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return Connection{}, err
	}

	i := -1
	if c.ID == "" {
		c.ID = newID()
		update = secretNames
	} else if i = indexOf(data.Connections, c.ID); i < 0 {
		return Connection{}, fmt.Errorf("%w: %q", ErrNotFound, c.ID)
	}

	if err := s.writeSecrets(c.ID, c.Config, update); err != nil {
		return Connection{}, err
	}

	saved := Connection{ID: c.ID, Name: c.Name, Config: withoutSecrets(c.Config)}
	if i < 0 {
		data.Connections = append(data.Connections, saved)
	} else {
		data.Connections[i] = saved
	}
	if err := s.write(data); err != nil {
		return Connection{}, err
	}
	return saved, nil
}

// Delete removes a connection and its secrets.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load()
	if err != nil {
		return err
	}
	i := indexOf(data.Connections, id)
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	data.Connections = append(data.Connections[:i], data.Connections[i+1:]...)
	if err := s.write(data); err != nil {
		return err
	}
	return s.deleteSecrets(id)
}

func (s *Store) writeSecrets(id string, cfg conn.ConnectionConfig, update []string) error {
	cfg = copyConfig(cfg)
	for _, f := range allSecretFields(&cfg) {
		orphaned := cfg.SSH == nil && f.name != SecretPassword
		if !orphaned && !slices.Contains(update, f.name) {
			continue
		}
		key := secretKey(id, f.name)
		var err error
		if *f.ptr == "" {
			err = s.secrets.Delete(key)
		} else {
			err = s.secrets.Set(key, *f.ptr)
		}
		if err != nil {
			return fmt.Errorf("write %s to keychain: %w", f.name, err)
		}
	}
	return nil
}

func (s *Store) deleteSecrets(id string) error {
	var errs []error
	for _, name := range secretNames {
		errs = append(errs, s.secrets.Delete(secretKey(id, name)))
	}
	return errors.Join(errs...)
}

func (s *Store) load() (fileData, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return fileData{Version: fileVersion}, nil
	}
	if err != nil {
		return fileData{}, err
	}
	var data fileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return fileData{}, fmt.Errorf("parse %s: %w", s.path, err)
	}
	if data.Version != fileVersion {
		return fileData{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, data.Version)
	}
	return data, nil
}

// write replaces the file atomically: temp file in the same dir, fsync, rename.
func (s *Store) write(data fileData) error {
	data.Version = fileVersion
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".connections-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func indexOf(cs []Connection, id string) int {
	for i, c := range cs {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b) // never fails (crypto/rand panics on failure since Go 1.24)
	return hex.EncodeToString(b)
}
