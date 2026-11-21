package conn

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// DBType identifies a database engine.
type DBType string

const (
	MySQL      DBType = "mysql"
	PostgreSQL DBType = "postgresql"
	SQLServer  DBType = "sqlserver"
	SQLite     DBType = "sqlite"
)

// ConnectionConfig describes how to reach one database.
type ConnectionConfig struct {
	Type     DBType `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	FilePath string `json:"filePath,omitempty"`

	// SocketPath reaches MySQL / PostgreSQL through a Unix socket on the SSH
	// server instead of Host:Port (PostgreSQL: the socket directory). It is a
	// path on the remote machine, so it is not part of FilePaths.
	SocketPath string `json:"socketPath,omitempty"`

	SSLMode                string `json:"sslMode,omitempty"`
	TrustServerCertificate bool   `json:"trustServerCertificate,omitempty"`

	SSH *SSHTunnel `json:"ssh,omitempty"` // nil = connect directly
	TLS *TLSFiles  `json:"tls,omitempty"` // nil = driver defaults
}

// SSHAuthMethod selects how to authenticate to the SSH server.
type SSHAuthMethod string

const (
	SSHAuthPassword   SSHAuthMethod = "password"
	SSHAuthPrivateKey SSHAuthMethod = "privateKey"
	SSHAuthAgent      SSHAuthMethod = "agent"
)

// SSHTunnel forwards the database connection through an SSH server.
type SSHTunnel struct {
	Host           string        `json:"host"`
	Port           int           `json:"port"` // 0 = 22
	User           string        `json:"user"`
	Auth           SSHAuthMethod `json:"auth"`
	Password       string        `json:"password,omitempty"`
	PrivateKeyPath string        `json:"privateKeyPath,omitempty"`
	Passphrase     string        `json:"passphrase,omitempty"`     // for an encrypted private key
	KnownHostsPath string        `json:"knownHostsPath,omitempty"` // "" = ~/.ssh/known_hosts
}

// TLSFiles are PEM files for verifying the server and authenticating the client.
type TLSFiles struct {
	CACertPath     string `json:"caCertPath,omitempty"`
	ClientCertPath string `json:"clientCertPath,omitempty"`
	ClientKeyPath  string `json:"clientKeyPath,omitempty"`
}

var (
	ErrUnsupportedType   = errors.New("unsupported database type")
	ErrMissingField      = errors.New("missing required field")
	ErrInvalidPort       = errors.New("port must be between 0 and 65535")
	ErrInvalidSSLMode    = errors.New("invalid sslmode")
	ErrInvalidSSHAuth    = errors.New("invalid ssh auth method")
	ErrUnsupportedOption = errors.New("option not supported for this database type")
	ErrInvalidPath       = errors.New("file path must be absolute and clean")
)

// DefaultSSHPort is used when SSHTunnel.Port is 0.
const DefaultSSHPort = 22

var validSSLModes = map[string]struct{}{
	"disable":     {},
	"allow":       {},
	"prefer":      {},
	"require":     {},
	"verify-ca":   {},
	"verify-full": {},
}

// DefaultPort returns the engine's standard port.
func DefaultPort(t DBType) int {
	switch t {
	case MySQL:
		return 3306
	case PostgreSQL:
		return 5432
	case SQLServer:
		return 1433
	default:
		return 0
	}
}

// Validate checks whether the configuration is valid.
func (c *ConnectionConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: config", ErrMissingField)
	}

	switch c.Type {
	case MySQL, PostgreSQL, SQLServer, SQLite:
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedType, c.Type)
	}

	for _, p := range c.FilePaths() {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return fmt.Errorf("%w: %q", ErrInvalidPath, p)
		}
	}

	if c.Type == SQLite {
		if strings.TrimSpace(c.FilePath) == "" {
			return fmt.Errorf("%w: filePath", ErrMissingField)
		}
		if c.SSH != nil || c.TLS != nil {
			return fmt.Errorf("%w: ssh/tls with sqlite", ErrUnsupportedOption)
		}
		return nil
	}

	if c.SocketPath != "" {
		if err := c.validateSocket(); err != nil {
			return err
		}
	} else if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("%w: host", ErrMissingField)
	}

	if strings.TrimSpace(c.User) == "" {
		return fmt.Errorf("%w: user (database user)", ErrMissingField)
	}

	if c.Port < 0 || c.Port > 65535 {
		return ErrInvalidPort
	}

	if c.Type == PostgreSQL {
		if _, ok := validSSLModes[c.SSLMode]; !ok && c.SSLMode != "" {
			return ErrInvalidSSLMode
		}
	}

	if c.SSH != nil {
		if err := c.SSH.validate(); err != nil {
			return err
		}
	}

	if c.TLS != nil {
		if err := c.validateTLS(); err != nil {
			return err
		}
	}

	return nil
}

func (c *ConnectionConfig) validateSocket() error {
	if c.SSH == nil {
		return fmt.Errorf("%w: a socket path needs an ssh tunnel", ErrUnsupportedOption)
	}
	if c.Type != MySQL && c.Type != PostgreSQL {
		return fmt.Errorf("%w: unix socket with %s", ErrUnsupportedOption, c.Type)
	}
	if !strings.HasPrefix(c.SocketPath, "/") || path.Clean(c.SocketPath) != c.SocketPath {
		return fmt.Errorf("%w: %q", ErrInvalidPath, c.SocketPath)
	}
	if c.TLS != nil {
		return fmt.Errorf("%w: tls over a unix socket", ErrUnsupportedOption)
	}
	return nil
}

func (s *SSHTunnel) validate() error {
	if strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("%w: ssh.host (SSH host)", ErrMissingField)
	}
	if strings.TrimSpace(s.User) == "" {
		return fmt.Errorf("%w: ssh.user (SSH user)", ErrMissingField)
	}
	if s.Port < 0 || s.Port > 65535 {
		return fmt.Errorf("%w: ssh.port", ErrInvalidPort)
	}

	switch s.Auth {
	case SSHAuthPassword, SSHAuthAgent:
	case SSHAuthPrivateKey:
		if strings.TrimSpace(s.PrivateKeyPath) == "" {
			return fmt.Errorf("%w: ssh.privateKeyPath", ErrMissingField)
		}
	default:
		return fmt.Errorf("%w: %q", ErrInvalidSSHAuth, s.Auth)
	}
	return nil
}

func (c *ConnectionConfig) validateTLS() error {
	t := c.TLS
	hasCert := strings.TrimSpace(t.ClientCertPath) != ""
	hasKey := strings.TrimSpace(t.ClientKeyPath) != ""
	if hasCert && !hasKey {
		return fmt.Errorf("%w: tls.clientKeyPath", ErrMissingField)
	}
	if hasKey && !hasCert {
		return fmt.Errorf("%w: tls.clientCertPath", ErrMissingField)
	}

	// go-mssqldb only accepts a server CA ("certificate"), not client certificates.
	if c.Type == SQLServer && hasCert {
		return fmt.Errorf("%w: tls client certificate with sqlserver", ErrUnsupportedOption)
	}
	if c.Type == PostgreSQL && c.SSLMode == "disable" {
		return fmt.Errorf("%w: disable conflicts with tls files", ErrInvalidSSLMode)
	}
	return nil
}

// FilePaths returns every non-empty file path in c, so callers can check
// that each one was chosen through a native file dialog.
func (c *ConnectionConfig) FilePaths() []string {
	if c == nil {
		return nil
	}
	candidates := []string{c.FilePath}
	if c.TLS != nil {
		candidates = append(candidates, c.TLS.CACertPath, c.TLS.ClientCertPath, c.TLS.ClientKeyPath)
	}
	if c.SSH != nil {
		candidates = append(candidates, c.SSH.PrivateKeyPath, c.SSH.KnownHostsPath)
	}

	var paths []string
	for _, p := range candidates {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// Key generates a stable, non-reversible cache key.
func (c *ConnectionConfig) Key() string {
	if c == nil {
		return ""
	}

	payload, err := json.Marshal(c)
	if err != nil {
		return ""
	}

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
