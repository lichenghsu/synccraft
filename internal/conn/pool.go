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
)

// PingTimeout bounds how long opening a connection may take.
const PingTimeout = 10 * time.Second

// ErrNotSupportedYet marks options that validate but cannot be connected yet.
// Failing loudly beats silently connecting without the tunnel or TLS.
var ErrNotSupportedYet = errors.New("not supported yet")

// ErrUntrustedCertificate wraps TLS verification failures with a hint on what to change.
var ErrUntrustedCertificate = errors.New("server certificate is not trusted")

// Pool caches one *sql.DB per distinct ConnectionConfig.
type Pool struct {
	mu  sync.Mutex
	dbs map[string]*sql.DB
}

func NewPool() *Pool {
	return &Pool{dbs: map[string]*sql.DB{}}
}

// Get returns a cached, pinged *sql.DB for c, opening one if needed.
// Do not Close the returned DB; use Pool.Close or Pool.CloseAll.
func (p *Pool) Get(ctx context.Context, c ConnectionConfig) (*sql.DB, error) {
	key := c.Key()

	p.mu.Lock()
	if db, ok := p.dbs[key]; ok {
		p.mu.Unlock()
		return db, nil
	}
	p.mu.Unlock()

	// Open without holding the lock so a slow server doesn't block other configs.
	db, err := open(ctx, c)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.dbs[key]; ok {
		db.Close() // lost a race with another Get for the same config
		return existing, nil
	}
	p.dbs[key] = db
	return db, nil
}

// Close closes and forgets the DB for c, if any.
func (p *Pool) Close(c ConnectionConfig) error {
	key := c.Key()
	p.mu.Lock()
	db, ok := p.dbs[key]
	delete(p.dbs, key)
	p.mu.Unlock()
	if !ok {
		return nil
	}
	return db.Close()
}

// CloseAll closes every cached DB.
func (p *Pool) CloseAll() error {
	p.mu.Lock()
	dbs := p.dbs
	p.dbs = map[string]*sql.DB{}
	p.mu.Unlock()

	var errs []error
	for _, db := range dbs {
		errs = append(errs, db.Close())
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
	db, err := open(ctx, c)
	if err != nil {
		return err
	}
	return db.Close()
}

func open(ctx context.Context, c ConnectionConfig) (*sql.DB, error) {
	if c.SSH != nil {
		return nil, fmt.Errorf("ssh tunnel: %w", ErrNotSupportedYet)
	}
	if c.Type == MySQL && c.TLS != nil {
		return nil, fmt.Errorf("mysql tls files: %w", ErrNotSupportedYet)
	}

	driver, dsn, err := buildDSN(c)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

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
		db.Close()
		return nil, explainTLSError(c.Type, err)
	}
	return db, nil
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
