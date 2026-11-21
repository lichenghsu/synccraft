package app

import (
	"context"

	"github.com/lichenghsu/synccraft/internal/conn"
)

// ConnectionService tests connections and lists databases.
type ConnectionService struct {
	files *FileService
	pool  *conn.Pool
}

func NewConnectionService(files *FileService) *ConnectionService {
	return &ConnectionService{files: files, pool: conn.NewPool()}
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
