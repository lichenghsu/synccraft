package app

import (
	"context"

	"github.com/lichenghsu/synccraft/internal/conn"
	"github.com/lichenghsu/synccraft/internal/store"
)

// ConnectionService tests connections, lists databases and manages saved connections.
type ConnectionService struct {
	files *FileService
	store *store.Store
	pool  *conn.Pool
}

func NewConnectionService(files *FileService, st *store.Store) *ConnectionService {
	return &ConnectionService{files: files, store: st, pool: conn.NewPool()}
}

// TestConnection opens cfg, pings it and closes it without caching.
func (s *ConnectionService) TestConnection(ctx context.Context, cfg conn.ConnectionConfig) error {
	if err := s.files.checkPaths(&cfg); err != nil {
		return err
	}
	return conn.Ping(ctx, cfg)
}

// ListDatabases returns the non-system databases on the server in cfg.
func (s *ConnectionService) ListDatabases(ctx context.Context, cfg conn.ConnectionConfig) ([]string, error) {
	if err := s.files.checkPaths(&cfg); err != nil {
		return nil, err
	}
	return s.listDatabases(ctx, cfg)
}

// ListSaved returns saved connections without passwords.
func (s *ConnectionService) ListSaved() ([]store.Connection, error) {
	list, err := s.store.List()
	if err != nil {
		return nil, err
	}
	// Paths from our own file are trusted, so the frontend may send them back.
	for _, c := range list {
		s.files.allow(c.Config.FilePaths()...)
	}
	return list, nil
}

// SaveConnection creates (empty id) or updates a saved connection.
// Passwords in c.Config are stored only when creating or when updateSecrets is true.
func (s *ConnectionService) SaveConnection(c store.Connection, updateSecrets bool) (store.Connection, error) {
	if err := s.files.checkPaths(&c.Config); err != nil {
		return store.Connection{}, err
	}
	if err := c.Config.Validate(); err != nil {
		return store.Connection{}, err
	}

	var old *conn.ConnectionConfig
	if c.ID != "" {
		if cfg, err := s.store.Get(c.ID); err == nil {
			old = &cfg
		}
	}

	saved, err := s.store.Save(c, updateSecrets)
	if err != nil {
		return store.Connection{}, err
	}
	if old != nil {
		s.pool.Close(*old) // drop the connection opened with the previous settings
	}
	return saved, nil
}

// DeleteConnection removes a saved connection and its passwords.
func (s *ConnectionService) DeleteConnection(id string) error {
	cfg, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if err := s.store.Delete(id); err != nil {
		return err
	}
	return s.pool.Close(cfg)
}

// TestSavedConnection tests a saved connection using its stored passwords.
func (s *ConnectionService) TestSavedConnection(ctx context.Context, id string) error {
	cfg, err := s.store.Get(id)
	if err != nil {
		return err
	}
	return conn.Ping(ctx, cfg)
}

// ListSavedDatabases lists databases on a saved connection.
func (s *ConnectionService) ListSavedDatabases(ctx context.Context, id string) ([]string, error) {
	cfg, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	return s.listDatabases(ctx, cfg)
}

func (s *ConnectionService) listDatabases(ctx context.Context, cfg conn.ConnectionConfig) ([]string, error) {
	db, err := s.pool.Get(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return conn.ListDatabases(ctx, db, cfg.Type)
}

// ServiceShutdown is called by Wails when the app exits.
func (s *ConnectionService) ServiceShutdown() error {
	return s.pool.CloseAll()
}
