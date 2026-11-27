# internal/dialect

Database metadata (introspection): tables, columns, primary keys, indexes, one implementation per engine; plus paginated row reads for the table browser. Does not import Wails; callers pass an open `*sql.DB` (usually from `conn.Pool`).

## Files

| File | Contents |
|---|---|
| `dialect.go` | `Dialect` interface and shared types (`Table`, `Column`, `Index`), `ErrTableNotFound`, `appendIndexColumn` |
| `for.go` | `For(conn.DBType)`: the dialect for a connection type |
| `rows.go` | `ReadRows` (one page of rows, `RowPage`), `CountRows`, `cellValue` (display normalization) |
| `sqlite.go` | `SQLite` dialect (`sqlite_schema`, `pragma_table_info`, `pragma_index_list` + `pragma_index_info`) |
| `postgres.go` | `PostgreSQL` dialect (`pg_class`, `pg_attribute`, `pg_index`, `pg_constraint`) |
| `mysql.go` | `MySQL` dialect for MySQL and MariaDB (`information_schema.TABLES` / `COLUMNS` / `STATISTICS`) |
| `sqlserver.go` | `SQLServer` dialect (`sys.tables`, `sys.columns`, `sys.indexes`, `sys.default_constraints`) |

## Rules

- List methods return a non-nil, deterministically ordered slice.
- Pass `ctx` through to every query (`QueryContext`, not `Query`) so calls are cancellable.
- Exclude views and engine-internal objects (SQLite: names starting with `sqlite_`).
- Per-table methods (and `ReadRows` / `CountRows`) return an error wrapping `ErrTableNotFound` for a missing table or a view.
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

## Reading rows

`ReadRows(ctx, d, db, t, offset, limit)` is the only place that puts identifiers into SQL text, so every name goes through `Dialect.QuoteIdent` (`"a""b"`, `` `a``b` ``, `[a]]b]`). It reads `Columns` and `PrimaryKey` first (which also rejects missing tables and views), then selects the columns in definition order:

| Engine | Paging |
|---|---|
| SQLite, PostgreSQL, MySQL | `ORDER BY <pk> LIMIT n OFFSET m` |
| SQL Server | `ORDER BY <pk> OFFSET m ROWS FETCH NEXT n ROWS ONLY`; `ORDER BY (SELECT NULL)` without a primary key |

With a primary key pages are stable (`RowPage.Ordered`); without one rows come in server order and may shift between pages. Offset paging gets slower deep into big tables; that is fine for a preview, M4 sync uses key ranges instead.

`cellValue` makes each value safe for JSON and JavaScript:

| Scanned value | Sent as |
|---|---|
| `int64` within ±(2^53−1) | number |
| `int64` beyond that, NaN / ±Inf | string (JS numbers would lose digits; JSON has no NaN) |
| `time.Time` | RFC 3339 string |
| `[]byte`, valid UTF-8 | string (MySQL returns most types this way, e.g. `DECIMAL`) |
| `[]byte`, binary | `0x…` hex, first 32 bytes then `… (N bytes)` |
| SQL Server `uniqueidentifier` | canonical GUID (its first three groups are stored little-endian) |
| strings over 1000 characters | cut, ending with `…` |

`CountRows` uses `COUNT(*)` (`COUNT_BIG(*)` on SQL Server, whose `COUNT` is a 32-bit int). It scans the table on most engines, so the UI only runs it on request and lets the user cancel.

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

SQLite tests and the pure helpers (`mysqlDefault`, `mssqlType`, `unwrapParens`, `cellValue`, quoting and paging SQL) run in plain `go test`. Server engines use `-tags integration` against `dev/docker-compose.yml`; each test creates and drops its own database:

```sh
podman compose -f dev/docker-compose.yml up -d pg-src mysql-src mssql
go test -tags integration ./internal/dialect/...
```

The compose file has no MariaDB, so MariaDB default handling is covered only by `TestMySQLDefault`.
