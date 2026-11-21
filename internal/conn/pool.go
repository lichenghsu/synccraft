package conn

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

// PingTimeout bounds how long opening a connection may take.
const PingTimeout = 10 * time.Second

// ErrUntrustedCertificate wraps TLS verification failures with a hint on what to change.
var ErrUntrustedCertificate = errors.New("server certificate is not trusted")

// ErrSocketAuthOnly wraps MySQL error 1698: the account authenticates by the
// OS user of a local socket (auth_socket / unix_socket), not by password.
var ErrSocketAuthOnly = errors.New("this account only accepts local socket logins")

// Pool caches one *sql.DB per distinct ConnectionConfig.
type Pool struct {
	mu  sync.Mutex
	dbs map[string]*opened
}

// opened is a DB plus the tunnel it dials through, closed together.
type opened struct {
	db     *sql.DB
	tunnel dialer
}

func (o *opened) Close() error {
	err := o.db.Close()
	if o.tunnel != nil {
		err = errors.Join(err, o.tunnel.Close())
	}
	return err
}

func NewPool() *Pool {
	return &Pool{dbs: map[string]*opened{}}
}

// Get returns a cached, pinged *sql.DB for c, opening one if needed.
// Do not Close the returned DB; use Pool.Close or Pool.CloseAll.
func (p *Pool) Get(ctx context.Context, c ConnectionConfig) (*sql.DB, error) {
	key := c.Key()

	p.mu.Lock()
	if o, ok := p.dbs[key]; ok {
		p.mu.Unlock()
		return o.db, nil
	}
	p.mu.Unlock()

	// Open without holding the lock so a slow server doesn't block other configs.
	o, err := open(ctx, c)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.dbs[key]; ok {
		o.Close() // lost a race with another Get for the same config
		return existing.db, nil
	}
	p.dbs[key] = o
	return o.db, nil
}

// Close closes and forgets the DB for c, if any.
func (p *Pool) Close(c ConnectionConfig) error {
	key := c.Key()
	p.mu.Lock()
	o, ok := p.dbs[key]
	delete(p.dbs, key)
	p.mu.Unlock()
	if !ok {
		return nil
	}
	return o.Close()
}

// CloseAll closes every cached DB.
func (p *Pool) CloseAll() error {
	p.mu.Lock()
	dbs := p.dbs
	p.dbs = map[string]*opened{}
	p.mu.Unlock()

	var errs []error
	for _, o := range dbs {
		errs = append(errs, o.Close())
	}
	return errors.Join(errs...)
}

// Len reports how many DBs are cached.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.dbs)
}

// Ping opens c, pings it and closes it again without caching.
func Ping(ctx context.Context, c ConnectionConfig) error {
	_, err := Probe(ctx, c)
	return err
}

// Probe opens c, identifies the server and closes it again without caching.
func Probe(ctx context.Context, c ConnectionConfig) (ServerInfo, error) {
	o, err := open(ctx, c)
	if err != nil {
		return ServerInfo{}, err
	}
	defer o.Close()
	return DetectServer(ctx, o.db, c.Type)
}

func open(ctx context.Context, c ConnectionConfig) (*opened, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	var opts openOptions
	switch {
	case c.SSH != nil:
		tun, err := newSSHTunnel(ctx, c.SSH)
		if err != nil {
			return nil, err
		}
		opts.dial = tun
	case c.K8s != nil:
		fw, err := newK8sForwarder(c)
		if err != nil {
			return nil, err
		}
		if c, err = prepareK8s(ctx, c, fw, &opts); err != nil {
			return nil, err
		}
		opts.dial = fw
	}

	db, err := openDB(c, opts)
	if err != nil {
		if opts.dial != nil {
			opts.dial.Close()
		}
		return nil, err
	}
	o := &opened{db: db, tunnel: opts.dial}

	if c.Type == SQLite {
		db.SetMaxOpenConns(1) // avoid "database is locked" between our own connections
	} else {
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(4)
		db.SetConnMaxIdleTime(5 * time.Minute)
	}

	pingCtx, cancel := context.WithTimeout(ctx, PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		o.Close()
		return nil, explain(c, err)
	}
	return o, nil
}

func explain(c ConnectionConfig, err error) error {
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == 1698 {
		hint := "connect through SSH and pick \"Unix socket\", or create a user with a password"
		if c.SocketPath != "" {
			hint = "log in over SSH as the OS user with the same name as the database user, or create a user with a password"
		}
		return fmt.Errorf("%w: %s (%w)", ErrSocketAuthOnly, hint, err)
	}
	return explainTLSError(c.Type, err)
}

func explainTLSError(t DBType, err error) error {
	var verr *tls.CertificateVerificationError
	// go-mssqldb flattens the error to a string, so also match the text.
	if !errors.As(err, &verr) && !strings.Contains(err.Error(), "x509:") {
		return err
	}
	hint := "choose the server's CA certificate"
	switch t {
	case SQLServer:
		hint += `, or enable "Trust server certificate" if it is self-signed on a server you control`
	case PostgreSQL:
		hint += ", or pick a lower SSL mode"
	}
	return fmt.Errorf("%w: %s (%w)", ErrUntrustedCertificate, hint, err)
}
