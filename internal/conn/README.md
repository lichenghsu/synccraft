# internal/conn

Database connection layer: connection config, DSN building, driver registration and the connection pool. Does not import Wails.

## Files

| File | Contents |
|---|---|
| `config.go` | `DBType`, `ConnectionConfig`, `SSHTunnel`, `TLSFiles`, `Validate()`, `Key()`, errors |
| `dsn.go` | `buildDSN(c) (driver, dsn, err)` and one DSN builder per engine |
| `drivers.go` | Blank imports that register the four drivers with `database/sql` |
| `pool.go` | `Pool` (one cached `*sql.DB` per config) and `Ping` (open, ping, close) |
| `connector.go` | `openDB`: per-engine connectors (MySQL TLS, SSH dialers), `loadTLSConfig` |
| `ssh.go` | SSH tunnel: auth methods, strict known_hosts, reconnect |
| `databases.go` | `ListDatabases`: non-system databases per engine |
| `socket.go` | `DetectSocket`: finds the database's Unix socket on the SSH server |
| `server.go` | `DetectServer` / `ServerInfo`: product and version, MariaDB told apart from MySQL |

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
| MySQL | ✅ | ✅ | `loadTLSConfig` builds a `tls.Config` set on `mysql.Config.TLS` (no global registration) |
| PostgreSQL | ✅ | ✅ | `sslrootcert` / `sslcert` / `sslkey` in the DSN; not allowed with `sslmode=disable` |
| SQL Server | ✅ | ❌ | `certificate=<CA path>` in the DSN; go-mssqldb does not support client certificates |

`Validate` only checks field combinations, not whether files exist; files are read when connecting.

`SSH.Password` and `SSH.Passphrase` are secrets like `Password`: when saving, they go to the OS keychain, never into the JSON file.

## Pool

- `Get(ctx, cfg)` returns a cached, pinged `*sql.DB`, keyed by `cfg.Key()`. Callers must not `Close` it.
- Opening happens **outside the lock**, so a slow server doesn't block other configs; if two `Get`s race for the same config, the loser closes its DB and returns the winner's.
- Failed opens are not cached. Ping is bounded by `PingTimeout` (10 s) and by the caller's ctx (frontend `.cancel()`).
- SQLite gets `MaxOpenConns(1)` to avoid `database is locked`; servers get 8 open / 4 idle / 5 min idle time.
- `Probe(ctx, cfg)` is for "Test connection": connects, identifies the server, disconnects; nothing is cached. `Ping` is the same without the result.
- `Close(cfg)` / `CloseAll()`; `ConnectionService.ServiceShutdown` calls `CloseAll`.

**`ErrUntrustedCertificate`**: TLS verification failures are wrapped with a hint per engine (choose the CA certificate; SQL Server: "Trust server certificate" for self-signed; PostgreSQL: lower SSL mode). The original error stays in the chain. go-mssqldb returns the TLS error as plain text, so `x509:` in the message is matched too.

### How a connection is opened

`openDB` builds a connector per engine instead of `sql.Open(dsn)`, so it can attach things a DSN can't hold:

| DB | Connector | With an SSH tunnel |
|---|---|---|
| MySQL | `mysql.NewConnector(cfg)`, `cfg.TLS` from `loadTLSConfig` | `cfg.DialFunc` |
| PostgreSQL | `pgx.ParseConfig` + `stdlib.OpenDB` | `DialFunc`, and `LookupFunc` returns the name unchanged so the SSH server resolves it |
| SQL Server | `mssql.NewConnector` | `Dialer` implementing `HostDialer`, so DNS resolves on the SSH server |
| SQLite | `sql.Open` | not allowed |

The DSN keeps the real database host; only the dial goes through SSH. TLS therefore verifies the database's own name (e.g. PostgreSQL `verify-full`), not `127.0.0.1` as a local port forward would.

## SSH tunnel (`ssh.go`)

- One `ssh.Client` per pooled DB, closed together with it. Connected eagerly so auth and host key errors show up on "Test connection".
- If dialing fails (laptop slept, network changed), the client is dropped and reconnected once.
- **Host keys**: strict `knownhosts` check. Unknown host → `ErrSSHHostUnknown`, changed key → `ErrSSHHostKeyMismatch`, missing file → `ErrSSHKnownHostsMissing`; each message includes the presented key's fingerprint. The client only negotiates host key algorithms already recorded for that host, otherwise a server offering ed25519 when known_hosts holds its ECDSA key would look like a mismatch.
- **Unix socket over SSH**: with `SocketPath` set (MySQL: socket file, PostgreSQL: socket directory), the connection is forwarded to that socket on the SSH server (`direct-streamlocal`, allowed by OpenSSH's default `AllowStreamLocalForwarding yes`). The database then sees a local socket login by the SSH user's OS account, so `auth_socket` / `unix_socket` accounts (root on Debian/Ubuntu MariaDB and MySQL) and PostgreSQL `peer` work without a password when the names match. Requires a tunnel; no TLS; not for SQL Server. `SocketPath` is a remote path, so it is not in `FilePaths` and not on the local allowlist.
- **Socket detection** (`DetectSocket`): over one SSH connection, tries the usual locations until one accepts a connection. MySQL / MariaDB: `/run/mysqld/mysqld.sock`, `/var/run/mysqld/mysqld.sock` (Debian, Ubuntu), `/var/lib/mysql/mysql.sock` (RHEL family), `/var/run/mysql/mysql.sock` (openSUSE), `/tmp/mysql.sock`. PostgreSQL directories: `/var/run/postgresql`, `/run/postgresql`, `/tmp` (checking `.s.PGSQL.<port>`). Refused forwards don't drop the SSH connection, so trying several is cheap. `ErrSocketNotFound` lists what was tried and how to look it up on the server.
- **Refused forwards** (`ErrSSHForwardRejected`): the message names the target and the likely fix: wrong socket path (`SELECT @@socket`), wrong host/port as seen from the SSH server, or `AllowTcpForwarding` / `AllowStreamLocalForwarding` disabled in sshd. A refused forward does not trigger a reconnect, since the SSH connection itself is fine.
- **MySQL error 1698** (account only accepts socket logins) is wrapped in `ErrSocketAuthOnly` with a hint to use socket mode or create a password user.
- **Auth**: password (also answers keyboard-interactive prompts), private key (`ErrSSHPassphraseRequired` / `ErrSSHWrongPassphrase` for encrypted keys), agent via `SSH_AUTH_SOCK` (`ErrSSHAgentUnavailable`). Failed logins return `ErrSSHAuthFailed`, including OpenSSH-without-PAM's bare failure to a keyboard-interactive attempt.

## Server detection

MariaDB uses the `mysql` type (same protocol and driver) but its SQL dialect differs, so `DetectServer` reports the actual product. Later dialect code should branch on `ServerInfo.Product`, not on `DBType` alone.

| DB | Query | Example raw → product, version |
|---|---|---|
| MySQL / MariaDB | `SELECT VERSION()` | `10.11.6-MariaDB-0+deb12u2` → MariaDB 10.11.6; `8.4.3` → MySQL 8.4.3 |
| PostgreSQL | `SHOW server_version` | `17.12 (Debian …)` → PostgreSQL 17.12 |
| SQL Server | `SERVERPROPERTY('ProductVersion')` + `Edition` | `16.0.4215.2 Developer Edition (64-bit)` → SQL Server 16.0.4215.2 |
| SQLite | `sqlite_version()` | `3.50.4` → SQLite 3.50.4 |

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

They:

- list databases on MySQL, PostgreSQL and SQL Server (creating `synccraft_it` on SQL Server, which has no seed);
- log in to MySQL and PostgreSQL as a temporary user whose password is `` p@ss/w;rd 'x' ``;
- reach all three through the `ssh-bastion` container by their compose service names (which only resolve on the bastion), and check a wrong SSH password;
- reach MySQL and PostgreSQL through their Unix sockets, which the bastion mounts from shared volumes;
- give MySQL a random CA and expect `ErrUntrustedCertificate`, proving the TLS files are used.

Unit tests in `ssh_test.go` run an in-process SSH server (no docker) covering every auth method, host key failures, mismatched key types and reconnecting.
