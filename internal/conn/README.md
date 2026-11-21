# internal/conn

Database connection layer: connection config, DSN building, driver registration and the connection pool. Does not import Wails.

## Files

| File | Contents |
|---|---|
| `config.go` | `DBType`, `ConnectionConfig`, `SSHTunnel`, `TLSFiles`, `Validate()`, `Key()`, errors |
| `dsn.go` | `buildDSN(c) (driver, dsn, err)` and one DSN builder per engine |
| `drivers.go` | Blank imports that register the four drivers with `database/sql` |
| `pool.go` | `Pool` (one cached `*sql.DB` per config) and `Ping` (open, ping, close) |
| `databases.go` | `ListDatabases`: non-system databases per engine |

## Drivers

| DBType | Driver name | Package | Notes |
|---|---|---|---|
| `mysql` | `mysql` | `github.com/go-sql-driver/mysql` | Use `mysql.Config.FormatDSN()`; `ParseTime`, `utf8mb4`; **no** `multiStatements` |
| `postgresql` | `pgx` | `github.com/jackc/pgx/v5/stdlib` | Build with `url.URL`; `sslmode` defaults to `prefer` |
| `sqlserver` | `sqlserver` | `github.com/microsoft/go-mssqldb` | Build with `url.URL`; **always** write `TrustServerCertificate` explicitly (when omitted, the driver trusts any certificate) |
| `sqlite` | `sqlite` | `modernc.org/sqlite` | Pure Go, no CGO; DSN is a `file:` URI so `? # %` in file names are safe; `foreign_keys` enabled |

## Rules

- Always build DSNs with the driver's structured API or `net/url`, never by string concatenation. Passwords containing `@ / ; ' space %` must round-trip (see `trickyPasswords` in `dsn_test.go`).
- `Port` 0 means the engine default (`DefaultPort`).
- `Key()` is the pool cache key: it changes when any field changes (password included) and never contains the password in plain text.

## SSH tunnel and TLS certificates

Both are pointer fields on `ConnectionConfig`; `nil` means unused. `Validate` rejects either one for SQLite.

**`SSH *SSHTunnel`**: connect through a bastion host. `Port` 0 means 22.

| `Auth` | Required fields |
|---|---|
| `password` | `Password` |
| `privateKey` | `PrivateKeyPath`; `Passphrase` if the key is encrypted |
| `agent` | none (uses `SSH_AUTH_SOCK`) |

Host keys are always checked against known_hosts; an empty `KnownHostsPath` means `~/.ssh/known_hosts`. **There is no option to skip host key verification.**

**`TLS *TLSFiles`**: PEM file paths. Client certificate and key must be given together.

| DB | CA | Client cert | How it maps |
|---|---|---|---|
| MySQL | ✅ | ✅ | **Not in `mysqlDSN` yet**: build a `tls.Config`, register it with `mysql.RegisterTLSConfig`, set `tls=<name>` — done when the pool opens the connection |
| PostgreSQL | ✅ | ✅ | `sslrootcert` / `sslcert` / `sslkey` in the DSN; not allowed with `sslmode=disable` |
| SQL Server | ✅ | ❌ | `certificate=<CA path>` in the DSN; go-mssqldb does not support client certificates |

`Validate` only checks field combinations, not whether files exist; files are read when connecting.

`SSH.Password` and `SSH.Passphrase` are secrets like `Password`: when saving, they go to the OS keychain, never into the JSON file.

## Pool

- `Get(ctx, cfg)` returns a cached, pinged `*sql.DB`, keyed by `cfg.Key()`. Callers must not `Close` it.
- Opening happens **outside the lock**, so a slow server doesn't block other configs; if two `Get`s race for the same config, the loser closes its DB and returns the winner's.
- Failed opens are not cached. Ping is bounded by `PingTimeout` (10 s) and by the caller's ctx (frontend `.cancel()`).
- SQLite gets `MaxOpenConns(1)` to avoid `database is locked`; servers get 8 open / 4 idle / 5 min idle time.
- `Ping(ctx, cfg)` is for "Test connection": nothing is cached.
- `Close(cfg)` / `CloseAll()`; `ConnectionService.ServiceShutdown` calls `CloseAll`.

**`ErrUntrustedCertificate`**: TLS verification failures are wrapped with a hint per engine (choose the CA certificate; SQL Server: "Trust server certificate" for self-signed; PostgreSQL: lower SSL mode). The original error stays in the chain. go-mssqldb returns the TLS error as plain text, so `x509:` in the message is matched too.

**`ErrNotSupportedYet`**: SSH tunnels and MySQL TLS files pass `Validate` but `open` refuses them, rather than silently connecting without the tunnel or TLS.

## ListDatabases

| DB | Source | Hidden |
|---|---|---|
| MySQL | `information_schema.SCHEMATA` | `information_schema`, `mysql`, `performance_schema`, `sys` |
| PostgreSQL | `pg_database` | templates and databases that disallow connections |
| SQL Server | `sys.databases` | `master`, `tempdb`, `model`, `msdb` (`database_id <= 4`) |
| SQLite | `pragma_database_list` | none (normally just `main`) |

## Tests

```bash
go test ./internal/conn/...
```

DSN tests need no database: each generated DSN is parsed back with the driver's own parser (`mysql.ParseDSN`, `pgconn.ParseConfig`, `msdsn.Parse`) and compared. SQLite tests and the pool tests open real files in a temp directory.

Integration tests run against `dev/docker-compose.yml` (build tag `integration`):

```bash
podman compose -f dev/docker-compose.yml up -d
go test -tags integration ./internal/conn/...
```

They list databases on MySQL, PostgreSQL and SQL Server (creating `synccraft_it` on SQL Server, which has no seed), and log in to MySQL and PostgreSQL as a temporary user whose password is `` p@ss/w;rd 'x' ``.
