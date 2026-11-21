package conn

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

var ErrSocketNotFound = errors.New("no database socket found on the SSH server")

// Common locations, most likely first. PostgreSQL entries are directories.
var socketCandidates = map[DBType][]string{
	MySQL: {
		"/run/mysqld/mysqld.sock",     // Debian, Ubuntu, Arch
		"/var/run/mysqld/mysqld.sock", // same, older layout
		"/var/lib/mysql/mysql.sock",   // RHEL, Rocky, Alma, Fedora
		"/var/run/mysql/mysql.sock",   // openSUSE
		"/tmp/mysql.sock",             // macOS, source builds
	},
	PostgreSQL: {
		"/var/run/postgresql", // Debian, Ubuntu, RHEL packages
		"/run/postgresql",
		"/tmp", // source builds, Homebrew
	},
}

// DetectSocket finds the database's Unix socket on c's SSH server by trying
// the usual locations over one SSH connection. It returns the value for
// SocketPath (for PostgreSQL, the directory).
func DetectSocket(ctx context.Context, c ConnectionConfig) (string, error) {
	if c.SSH == nil {
		return "", fmt.Errorf("%w: socket detection needs an ssh tunnel", ErrUnsupportedOption)
	}
	candidates, ok := socketCandidates[c.Type]
	if !ok {
		return "", fmt.Errorf("%w: unix socket with %s", ErrUnsupportedOption, c.Type)
	}
	if err := c.SSH.validate(); err != nil {
		return "", err
	}

	tun, err := newSSHTunnel(ctx, c.SSH)
	if err != nil {
		return "", err
	}
	defer tun.Close()

	for _, candidate := range candidates {
		target := candidate
		if c.Type == PostgreSQL {
			target = path.Join(candidate, ".s.PGSQL."+strconv.Itoa(c.port()))
		}
		conn, err := tun.dial(ctx, "unix", target)
		if err == nil {
			conn.Close()
			return candidate, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("%w: tried %s; run SELECT @@socket (MySQL) or SHOW unix_socket_directories (PostgreSQL) on the server",
		ErrSocketNotFound, strings.Join(candidates, ", "))
}
