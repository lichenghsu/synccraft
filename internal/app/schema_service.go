package app

import (
	"context"
	"database/sql"

	"github.com/lichenghsu/synccraft/internal/conn"
	"github.com/lichenghsu/synccraft/internal/dialect"
	"github.com/lichenghsu/synccraft/internal/store"
)

// MaxPageSize caps how many rows PreviewRows returns at once.
const MaxPageSize = 500

// SchemaService reads tables, their structure and their rows from saved connections.
type SchemaService struct {
	store *store.Store
	pool  *conn.Pool
}

// NewSchemaService shares pool with ConnectionService, which closes it on shutdown.
func NewSchemaService(st *store.Store, pool *conn.Pool) *SchemaService {
	return &SchemaService{store: st, pool: pool}
}

// TableInfo is the structure of one table.
type TableInfo struct {
	Columns    []dialect.Column `json:"columns"`
	PrimaryKey []string         `json:"primaryKey"`
	Indexes    []dialect.Index  `json:"indexes"`
}

// ListTables returns the tables in database ("" = the connection's default)
// on a saved connection.
func (s *SchemaService) ListTables(ctx context.Context, id, database string) ([]dialect.Table, error) {
	d, db, err := s.open(ctx, id, database)
	if err != nil {
		return nil, err
	}
	return d.ListTables(ctx, db)
}

// DescribeTable returns the columns, primary key and indexes of t.
func (s *SchemaService) DescribeTable(ctx context.Context, id, database string, t dialect.Table) (TableInfo, error) {
	d, db, err := s.open(ctx, id, database)
	if err != nil {
		return TableInfo{}, err
	}
	var info TableInfo
	if info.Columns, err = d.Columns(ctx, db, t); err != nil {
		return TableInfo{}, err
	}
	if info.PrimaryKey, err = d.PrimaryKey(ctx, db, t); err != nil {
		return TableInfo{}, err
	}
	if info.Indexes, err = d.Indexes(ctx, db, t); err != nil {
		return TableInfo{}, err
	}
	return info, nil
}

// PreviewRows returns up to limit (at most MaxPageSize) rows of t starting at offset.
func (s *SchemaService) PreviewRows(ctx context.Context, id, database string, t dialect.Table, offset, limit int) (dialect.RowPage, error) {
	d, db, err := s.open(ctx, id, database)
	if err != nil {
		return dialect.RowPage{}, err
	}
	return dialect.ReadRows(ctx, d, db, t, offset, min(limit, MaxPageSize))
}

// CountRows counts the rows of t. It may scan the whole table; cancel it if slow.
func (s *SchemaService) CountRows(ctx context.Context, id, database string, t dialect.Table) (int64, error) {
	d, db, err := s.open(ctx, id, database)
	if err != nil {
		return 0, err
	}
	return dialect.CountRows(ctx, d, db, t)
}

// open returns the dialect and a pooled DB for a saved connection.
func (s *SchemaService) open(ctx context.Context, id, database string) (dialect.Dialect, *sql.DB, error) {
	_, d, db, err := openSaved(ctx, s.store, s.pool, id, database)
	return d, db, err
}

// openSaved returns the config, dialect and a pooled DB for a saved
// connection, switched to database when it is not empty. SQLite has a single
// database per file.
func openSaved(ctx context.Context, st *store.Store, pool *conn.Pool, id, database string) (conn.ConnectionConfig, dialect.Dialect, *sql.DB, error) {
	cfg, err := st.Get(id)
	if err != nil {
		return cfg, nil, nil, err
	}
	if database != "" && cfg.Type != conn.SQLite {
		cfg.Database = database
	}
	d, err := dialect.For(cfg.Type)
	if err != nil {
		return cfg, nil, nil, err
	}
	db, err := pool.Get(ctx, cfg)
	if err != nil {
		return cfg, nil, nil, err
	}
	return cfg, d, db, nil
}
