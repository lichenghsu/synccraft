# Development Progress

| Milestone | Scope | Status |
|---|---|---|
| M0 | Project scaffold: Wails v3 + React/TS, example Service, hooks, test databases, Renovate, CI | ✅ Done |
| M1 | Connection management: connection pool, four drivers, structured DSNs, passwords in the OS keychain, SSH tunnels, Kubernetes | ✅ Done |
| M2 | Introspection and table browser: Dialect interface, metadata queries for all four databases | ✅ Done |
| M3 | Schema comparison: diff algorithm, DDL in the target dialect, transactional execution | ⏳ Not started |
| M4 | Data sync: parameterized DML, value normalization, progress events and cancellation, streaming diff for large tables | ⏳ Not started |
| M5 | Side-by-side table comparison: row counts, column mapping, row diff, column statistics, CSV export | ⏳ Not started |
| M6 | Packaging and release: installers for three platforms, release workflow, code signing, auto-update | ⏳ Not started |

## M0 checklist

- [x] `wails3 init -t react`, upgraded to React 19 / TypeScript 7 / Vite 8
- [x] `@bindings` and `@events` path aliases
- [x] `internal/app/HelloService`: calls and errors, cancellable calls, typed events (with unit tests)
- [x] Hooks: `useAsync`, `useCancellable`, `useWailsEvent`
- [x] `dev/docker-compose.yml` and seed data
- [x] `renovate.json`, `.github/workflows/ci.yml`
- [x] LICENSE (MIT), README (English / Traditional Chinese)
- [ ] GitHub: install the Renovate App, set up branch protection on `main`

## M1 checklist

- [x] `internal/conn`: `ConnectionConfig`, `Validate()`, `Key()`, including SSH tunnel and TLS certificate fields
- [x] Drivers registered: `go-sql-driver/mysql`, `pgx/v5`, `microsoft/go-mssqldb`, `modernc.org/sqlite`
- [x] Structured DSN builders for all four drivers (`mysql.Config`, `net/url`), PG / SQL Server TLS file parameters
- [x] `FileService`: file paths only via native dialogs (backend allowlist, `PickFile`), `Validate` rejects relative / unclean paths
- [x] `Pool`: cache `*sql.DB`, ping timeout, close on `ServiceShutdown`; `Ping` without caching; integration tests against `dev/docker-compose.yml`
- [x] SSH tunnel (password / private key / agent, strict known_hosts, reconnect), Unix socket over SSH (with socket auto-detection), server detection (MariaDB vs MySQL), MySQL TLS; per-secret updates (`updateSecrets` list)
- [x] Kubernetes connections (built-in port-forward dialer; CloudNativePG with credentials from cluster secrets; service / pod targets)
- [x] `ConnectionService` (Wails): `TestConnection`, `ListDatabases` (both enforce `checkPaths`)
- [x] Store: `os.UserConfigDir` + OS keychain + atomic writes; saved-connection APIs in `ConnectionService`
- [x] React: connections page (saved list, `ConnectionForm`, test / load databases / save / delete), file fields via native dialogs only; `HelloService` removed
- [x] Manual check in the real window: GTK4 file dialogs, keychain, saved connections across restarts
- [x] Manual check of Kubernetes / CloudNativePG connections in the real window (kind cluster via `dev/kind/setup.sh`)
- **Acceptance**: all four databases connect and list their databases; passwords never appear in the JSON file or DevTools

## M2 checklist

- [x] `internal/dialect`: `Dialect` interface; tables, columns, primary keys, indexes for SQLite, PostgreSQL, MySQL / MariaDB, SQL Server
- [x] Integration tests per server engine (`-tags integration`, throwaway databases)
- [x] `ReadRows` / `CountRows`: quoted identifiers, primary-key order, per-engine paging, JSON-safe cell values
- [x] `SchemaService` (Wails): `ListTables`, `DescribeTable`, `PreviewRows`, `CountRows` on saved connections; pool shared with `ConnectionService`
- [x] React: Browse page (connection / database pickers, table filter, Structure and Data tabs, cancellable loads)
- [x] Manual check in the real window against all four databases
- **Acceptance**: every table in the dev databases shows its structure and pages through its rows; switching tables quickly never shows stale data
