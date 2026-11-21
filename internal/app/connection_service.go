package app

import (
	"context"
	"slices"

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

// ConnectionDraft is what the connection form holds.
type ConnectionDraft struct {
	ID            string                `json:"id"` // saved connection being edited; "" if new
	Config        conn.ConnectionConfig `json:"config"`
	UpdateSecrets []string              `json:"updateSecrets"` // secret names typed in the form; others come from the keychain
}

// TestConnection connects with the draft's config, identifies the server and
// disconnects without caching.
func (s *ConnectionService) TestConnection(ctx context.Context, d ConnectionDraft) (conn.ServerInfo, error) {
	cfg, err := s.resolve(d)
	if err != nil {
		return conn.ServerInfo{}, err
	}
	return conn.Probe(ctx, cfg)
}

// ListDatabases returns the non-system databases on the draft's server.
func (s *ConnectionService) ListDatabases(ctx context.Context, d ConnectionDraft) ([]string, error) {
	cfg, err := s.resolve(d)
	if err != nil {
		return nil, err
	}
	return s.listDatabases(ctx, cfg)
}

// resolve checks the draft's paths and, when editing a saved connection
// without touching its passwords, fills them in from the keychain.
func (s *ConnectionService) resolve(d ConnectionDraft) (conn.ConnectionConfig, error) {
	cfg := d.Config
	if err := s.files.checkPaths(&cfg); err != nil {
		return conn.ConnectionConfig{}, err
	}
	if d.ID == "" {
		return cfg, nil
	}

	stored, err := s.store.Get(d.ID)
	if err != nil {
		return conn.ConnectionConfig{}, err
	}
	keep := func(name string) bool { return !slices.Contains(d.UpdateSecrets, name) }
	if keep(store.SecretPassword) {
		cfg.Password = stored.Password
	}
	if cfg.SSH != nil && stored.SSH != nil {
		ssh := *cfg.SSH
		if keep(store.SecretSSHPassword) {
			ssh.Password = stored.SSH.Password
		}
		if keep(store.SecretSSHPassphrase) {
			ssh.Passphrase = stored.SSH.Passphrase
		}
		cfg.SSH = &ssh
	}
	return cfg, nil
}

// DetectSocket finds the database's Unix socket on the draft's SSH server.
func (s *ConnectionService) DetectSocket(ctx context.Context, d ConnectionDraft) (string, error) {
	cfg, err := s.resolve(d)
	if err != nil {
		return "", err
	}
	return conn.DetectSocket(ctx, cfg)
}

// K8sContexts lists the contexts in a kubeconfig ("" = $KUBECONFIG or ~/.kube/config).
func (s *ConnectionService) K8sContexts(kubeconfigPath string) (conn.K8sContexts, error) {
	if err := s.files.checkFile(kubeconfigPath); err != nil {
		return conn.K8sContexts{}, err
	}
	return conn.ListK8sContexts(kubeconfigPath)
}

// K8sNamespaces lists namespaces; it fails where RBAC forbids listing them,
// in which case the namespace can still be typed in.
func (s *ConnectionService) K8sNamespaces(ctx context.Context, kubeconfigPath, kubeContext string) ([]string, error) {
	if err := s.files.checkFile(kubeconfigPath); err != nil {
		return nil, err
	}
	return conn.ListK8sNamespaces(ctx, kubeconfigPath, kubeContext)
}

// K8sTargets lists CloudNativePG clusters and services in a namespace.
func (s *ConnectionService) K8sTargets(ctx context.Context, kubeconfigPath, kubeContext, namespace string) (conn.K8sTargets, error) {
	if err := s.files.checkFile(kubeconfigPath); err != nil {
		return conn.K8sTargets{}, err
	}
	return conn.ListK8sTargets(ctx, kubeconfigPath, kubeContext, namespace)
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

// SaveConnection creates (empty id) or updates a saved connection. When updating,
// only the secrets named in updateSecrets are written; the others are kept.
func (s *ConnectionService) SaveConnection(c store.Connection, updateSecrets []string) (store.Connection, error) {
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
func (s *ConnectionService) TestSavedConnection(ctx context.Context, id string) (conn.ServerInfo, error) {
	cfg, err := s.store.Get(id)
	if err != nil {
		return conn.ServerInfo{}, err
	}
	return conn.Probe(ctx, cfg)
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
