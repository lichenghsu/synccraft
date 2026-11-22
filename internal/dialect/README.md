# internal/dialect

Database metadata (introspection): tables, columns, primary keys, indexes, one implementation per engine. Does not import Wails; callers pass an open `*sql.DB` (usually from `conn.Pool`).

## Files

| File | Contents |
|---|---|
| `dialect.go` | `Dialect` interface and shared types (`Table`, `Column`), `ErrTableNotFound` |
| `sqlite.go` | `SQLite` dialect (`sqlite_schema`, `pragma_table_info`) |

## Rules

- List methods return a non-nil, deterministically ordered slice.
- Pass `ctx` through to every query (`QueryContext`, not `Query`) so calls are cancellable.
- Exclude views and engine-internal objects (SQLite: names starting with `sqlite_`).

## Tests

SQLite tests run in plain `go test` against a temp file. Server engines will use `-tags integration` against `dev/docker-compose.yml`.
