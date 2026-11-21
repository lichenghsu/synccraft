# Development Progress

| Milestone | Scope | Status |
|---|---|---|
| M0 | Project scaffold: Wails v3 + React/TS, example Service, hooks, test databases, Renovate, CI | ✅ Done |
| M1 | Connection management: connection pool, four drivers, structured DSNs, passwords in the OS keychain | 🚧 In progress |
| M2 | Introspection and table browser: Dialect interface, metadata queries for all four databases | ⏳ Not started |
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
- [ ] SSH tunnel (`golang.org/x/crypto/ssh` + known_hosts) and MySQL TLS (`mysql.RegisterTLSConfig`)
- [x] `ConnectionService` (Wails): `TestConnection`, `ListDatabases` (both enforce `checkPaths`)
- [x] Store: `os.UserConfigDir` + OS keychain + atomic writes; saved-connection APIs in `ConnectionService`
- [x] React: connections page (saved list, `ConnectionForm`, test / load databases / save / delete), file fields via native dialogs only; `HelloService` removed
- [x] Manual check in the real window: GTK4 file dialogs, keychain, saved connections across restarts
- **Acceptance**: all four databases connect and list their databases; passwords never appear in the JSON file or DevTools
