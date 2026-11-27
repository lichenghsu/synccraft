package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/lichenghsu/synccraft/internal/conn"
	"github.com/lichenghsu/synccraft/internal/dialect"
	"github.com/lichenghsu/synccraft/internal/schema"
	"github.com/lichenghsu/synccraft/internal/store"
)

var (
	// ErrEngineMismatch is returned when source and target are different engines.
	ErrEngineMismatch = errors.New("source and target must be the same database engine")
	// ErrComparisonExpired is returned for a token that is not the latest comparison.
	ErrComparisonExpired = errors.New("comparison expired; compare again")
	// ErrSchemaChanged is returned by Apply when either side changed after Compare.
	ErrSchemaChanged = errors.New("a schema changed since the comparison; compare again")
	// ErrUnknownChange is returned for a change id not in the comparison.
	ErrUnknownChange = errors.New("unknown change")
)

// Side is one end of a comparison: a saved connection and a database on it
// ("" = the connection's default; ignored for SQLite).
type Side struct {
	ConnectionID string `json:"connectionId"`
	Database     string `json:"database"`
}

// Comparison is the result of Compare. Steps holds every change with the SQL
// it would run if all changes were applied.
type Comparison struct {
	Token string        `json:"token"`
	Steps []schema.Step `json:"steps"`
	// TransactionalDDL is false for MySQL / MariaDB, where each DDL statement
	// commits on its own and a failed Apply leaves earlier statements applied.
	TransactionalDDL bool `json:"transactionalDdl"`
}

// CompareService compares the structure of two saved connections and applies
// selected changes to the target.
type CompareService struct {
	store *store.Store
	pool  *conn.Pool

	mu   sync.Mutex
	last *comparison
}

// comparison is the state Compare keeps for Preview and Apply.
type comparison struct {
	token          string
	source, target Side
	d              dialect.Dialect
	src, dst       []schema.Table
	changes        []schema.Change
	fingerprint    string
}

// NewCompareService shares pool with ConnectionService, which closes it on shutdown.
func NewCompareService(st *store.Store, pool *conn.Pool) *CompareService {
	return &CompareService{store: st, pool: pool}
}

// Compare loads both schemas and lists the changes that make target match
// source. It replaces any earlier comparison.
func (s *CompareService) Compare(ctx context.Context, source, target Side) (Comparison, error) {
	c, err := s.load(ctx, source, target)
	if err != nil {
		return Comparison{}, err
	}
	c.changes = schema.Diff(c.src, c.dst)
	steps, err := schema.Script(c.d, c.dst, c.changes)
	if err != nil {
		return Comparison{}, err
	}
	if c.token, err = newToken(); err != nil {
		return Comparison{}, err
	}

	s.mu.Lock()
	s.last = c
	s.mu.Unlock()
	return Comparison{Token: c.token, Steps: steps, TransactionalDDL: schema.TransactionalDDL(c.d)}, nil
}

// Preview returns the SQL for the selected changes of the latest comparison,
// in execution order. Later steps can depend on which earlier ones are
// selected (SQLite table rebuilds).
func (s *CompareService) Preview(token string, ids []string) ([]schema.Step, error) {
	c, err := s.get(token)
	if err != nil {
		return nil, err
	}
	selected, err := selectChanges(c.changes, ids)
	if err != nil {
		return nil, err
	}
	return schema.Script(c.d, c.dst, selected)
}

// Apply reloads both schemas, refuses if either changed since Compare, and
// runs the selected changes on the target in one transaction. Compare again
// afterwards to see what is left.
func (s *CompareService) Apply(ctx context.Context, token string, ids []string) error {
	c, err := s.get(token)
	if err != nil {
		return err
	}
	selected, err := selectChanges(c.changes, ids)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return errors.New("no changes selected")
	}
	// Claim the comparison: whatever happens next, it no longer describes
	// the target, and a second Apply with the same token must not run.
	s.mu.Lock()
	if s.last != c {
		s.mu.Unlock()
		return ErrComparisonExpired
	}
	s.last = nil
	s.mu.Unlock()

	fresh, err := s.load(ctx, c.source, c.target)
	if err != nil {
		return err
	}
	if fresh.fingerprint != c.fingerprint {
		return ErrSchemaChanged
	}
	steps, err := schema.Script(c.d, c.dst, selected)
	if err != nil {
		return err
	}
	_, _, db, err := openSaved(ctx, s.store, s.pool, c.target.ConnectionID, c.target.Database)
	if err != nil {
		return err
	}
	return schema.Apply(ctx, db, steps)
}

func (s *CompareService) load(ctx context.Context, source, target Side) (*comparison, error) {
	// Check the engines before connecting to anything.
	srcCfg, err := s.store.Get(source.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	dstCfg, err := s.store.Get(target.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	if srcCfg.Type != dstCfg.Type {
		return nil, fmt.Errorf("%w (%s vs %s)", ErrEngineMismatch, srcCfg.Type, dstCfg.Type)
	}
	d, srcDB, err := s.openSide(ctx, "source", source)
	if err != nil {
		return nil, err
	}
	_, dstDB, err := s.openSide(ctx, "target", target)
	if err != nil {
		return nil, err
	}
	c := &comparison{source: source, target: target, d: d}
	if c.src, err = loadSide(ctx, "source", d, srcDB); err != nil {
		return nil, err
	}
	if c.dst, err = loadSide(ctx, "target", d, dstDB); err != nil {
		return nil, err
	}
	sum, err := json.Marshal([2][]schema.Table{c.src, c.dst})
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(sum)
	c.fingerprint = hex.EncodeToString(h[:])
	return c, nil
}

func (s *CompareService) openSide(ctx context.Context, name string, side Side) (dialect.Dialect, *sql.DB, error) {
	_, d, db, err := openSaved(ctx, s.store, s.pool, side.ConnectionID, side.Database)
	if err != nil {
		err = fmt.Errorf("%s: %w", name, err)
	}
	return d, db, err
}

func loadSide(ctx context.Context, name string, d dialect.Dialect, db *sql.DB) ([]schema.Table, error) {
	ts, err := schema.Load(ctx, d, db)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return ts, nil
}

func (s *CompareService) get(token string) (*comparison, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil || s.last.token != token {
		return nil, ErrComparisonExpired
	}
	return s.last, nil
}

// selectChanges returns the changes named by ids, in Diff order.
func selectChanges(changes []schema.Change, ids []string) ([]schema.Change, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := []schema.Change{}
	for _, c := range changes {
		if want[c.ID] {
			out = append(out, c)
			delete(want, c.ID)
		}
	}
	for id := range want {
		return nil, fmt.Errorf("%w: %s", ErrUnknownChange, id)
	}
	return out, nil
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
