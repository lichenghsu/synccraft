# internal/dialect

Database metadata (introspection): tables, columns, primary keys, indexes, one implementation per engine. Does not import Wails; callers pass an open `*sql.DB` (usually from `conn.Pool`).

## Files

| File | Contents |
|---|---|
| `dialect.go` | `Dialect` interface and shared types (`Table`, `Column`, `Index`), `ErrTableNotFound`, `appendIndexColumn` |
| `sqlite.go` | `SQLite` dialect (`sqlite_schema`, `pragma_table_info`, `pragma_index_list` + `pragma_index_info`) |
| `postgres.go` | `PostgreSQL` dialect (`pg_class`, `pg_attribute`, `pg_index`, `pg_constraint`) |
| `mysql.go` | `MySQL` dialect for MySQL and MariaDB (`information_schema.TABLES` / `COLUMNS` / `STATISTICS`) |
| `sqlserver.go` | `SQLServer` dialect (`sys.tables`, `sys.columns`, `sys.indexes`, `sys.default_constraints`) |

## Rules

- List methods return a non-nil, deterministically ordered slice.
- Pass `ctx` through to every query (`QueryContext`, not `Query`) so calls are cancellable.
- Exclude views and engine-internal objects (SQLite: names starting with `sqlite_`).
- Per-table methods return an error wrapping `ErrTableNotFound` for a missing table or a view.
- The primary key comes from `PrimaryKey`; `Indexes` never includes the index that backs it.
- `Column.Position` is renumbered 1..n (PostgreSQL `attnum` keeps gaps from dropped columns).
- `Column.Default` is a default *expression* (string literals quoted), or nil. Generated / computed columns and identity columns report no default.
- An expression key part in an index is reported as `""`.

## Schemas

| Engine | `ListTables` returns | Empty `Table.Schema` means |
|---|---|---|
| SQLite | `Schema: ""` | the main database |
| PostgreSQL | every non-system schema | `current_schema()` |
| MySQL / MariaDB | `Schema: ""` (current database only) | `DATABASE()` |
| SQL Server | every user schema | `SCHEMA_NAME()` (usually `dbo`) |

Names are passed as query parameters and quoted server-side (`format('%I')`, `QUOTENAME`), so quotes, spaces and mixed case are safe.

## Engine notes

**PostgreSQL**
- Partitioned tables are listed once (partitions are skipped).
- `Type` comes from `format_type` (`character varying(20)`, `numeric(10,2)`). `Default` is `pg_get_expr` output (`nextval('t_id_seq'::regclass)`, `'guest'::character varying`).
- Index key columns exclude `INCLUDE` columns. A table may have zero columns, so existence is checked separately.

**MySQL / MariaDB**
- `Type` is `COLUMN_TYPE` (`varchar(20)`, `int unsigned`).
- MySQL stores literal defaults unquoted and marks expressions with `DEFAULT_GENERATED`; MariaDB 10.2.7+ stores expressions and the string `NULL` for `DEFAULT NULL`. `mysqlDefault` normalizes both (MariaDB detected via `VERSION()`). Numeric literals come back quoted (`'0'`), which is still valid MySQL.
- `Index.Constraint` is always false: MySQL does not distinguish a `UNIQUE` constraint from a unique index. Prefix lengths are not reported.

**SQL Server**
- `Type` is rebuilt from `sys.columns` (`max_length` is in bytes and `-1` for `max`): `nvarchar(20)`, `varchar(max)`, `decimal(10,2)`, `datetime2(3)`.
- Stored defaults are wrapped in parentheses (`('guest')`, `((0))`); `unwrapParens` strips them.
- Index key columns exclude `INCLUDE` columns.

## Not covered yet

Partial / filtered indexes, descending key parts, collations, column comments, foreign keys, check constraints. Add them when M3 schema comparison needs them.

## Tests

SQLite tests and the pure helpers (`mysqlDefault`, `mssqlType`, `unwrapParens`) run in plain `go test`. Server engines use `-tags integration` against `dev/docker-compose.yml`; each test creates and drops its own database:

```sh
podman compose -f dev/docker-compose.yml up -d pg-src mysql-src mssql
go test -tags integration ./internal/dialect/...
```

The compose file has no MariaDB, so MariaDB default handling is covered only by `TestMySQLDefault`.
