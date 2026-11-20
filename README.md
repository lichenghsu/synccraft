# SyncCraft

**English** | [繁體中文](README.zh-TW.md)

A cross-platform desktop tool for **database schema comparison, data synchronization, and side-by-side table comparison**.
Supports MySQL, PostgreSQL, SQL Server and SQLite; runs on macOS (Apple Silicon / Intel), Windows and Linux.

> SyncCraft is a rewrite of [nanablast/syncforge](https://github.com/nanablast/syncforge) (MIT License),
> rebuilt with **Wails v3 + React + TypeScript**, with a redesigned SQL dialect layer, connection pooling,
> OS-keychain credential storage and streaming comparison for large tables.

## Features

- **Connection management** — four database engines, passwords stored in the system keychain
- **Table browser** — structure, indexes, `CREATE` statements and paginated data
- **Schema comparison** — DDL generated in the *target* database's dialect
- **Data synchronization** — row-level diff by primary key, parameterized DML, transactions
- **Side-by-side table comparison** — any two tables across hosts or engines: row counts, column mapping, row diff, column statistics

Development progress is tracked in [`docs/progress.md`](docs/progress.md).

## Development

### Requirements

- Go ≥ 1.26 and Node.js (even-numbered LTS)
- [mise](https://mise.jdx.dev) — installs the pinned `wails3` CLI for this project
- Platform dependencies:
  - **Linux**: `gtk4-devel webkitgtk6.0-devel` (Fedora) / `libgtk-4-dev libwebkitgtk-6.0-dev` (Ubuntu 24.04+)
  - **macOS**: `xcode-select --install`
  - **Windows**: WebView2 Runtime (built into Windows 11)

### Getting started

```bash
mise install          # install wails3 (version pinned in mise.toml)
wails3 doctor         # check system dependencies
wails3 dev            # dev mode: frontend hot reload, Go auto-rebuild
```

### Common commands

```bash
wails3 build                                   # production build → bin/synccraft
wails3 package                                 # installer for the current platform
GOOS=windows wails3 build                      # cross-compile for Windows
wails3 generate bindings -clean=true -ts -i    # regenerate frontend bindings
go test ./...                                  # Go tests
podman compose -f dev/docker-compose.yml up -d # local test databases
```

### Project layout

```
main.go               # Wails entry point
internal/app/         # Wails services (the only package importing Wails)
frontend/             # React + TypeScript + Vite
  bindings/           # generated from Go (do not edit, commit it)
  src/api/            # wrappers around the Go bindings
  src/hooks/          # shared React hooks
build/                # per-platform build config, icons, Taskfiles
dev/                  # local test databases and seed data
docs/                 # development progress
```

Each module folder has its own `README.md` describing what it does (e.g. [`internal/app`](internal/app/README.md), [`frontend/src/hooks`](frontend/src/hooks/README.md)). Code comments are kept minimal and in English.

## Maintenance

- Dependency updates are opened automatically by [Renovate](https://docs.renovatebot.com) (`renovate.json`)
- Every PR is checked by GitHub Actions: bindings in sync, type check, tests, vulnerability scan, builds on all three platforms (`.github/workflows/ci.yml`)

## License

[MIT](LICENSE). The copyright notice of the original SyncForge project is retained.
