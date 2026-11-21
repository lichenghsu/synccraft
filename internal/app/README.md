# internal/app

Wails binding layer. Every **exported method** of a Service here gets a generated TypeScript function the frontend can call (output to `frontend/bindings/github.com/lichenghsu/synccraft/internal/app/`).

## Principles

- **This is the only package that imports Wails.** Core logic such as database connections and comparison algorithms lives in other `internal/` packages; this layer only converts parameters and calls down, so core logic can be tested on its own with `go test`.
- Doc comments on exported identifiers are copied into the generated TS bindings; keep them short and in English.
- After changing a method signature, regenerate the bindings and commit them: `wails3 generate bindings -clean=true -ts -i`. CI checks that bindings are in sync.

## Go → TypeScript mapping

| Go | TypeScript |
|---|---|
| `func X() (T, error)` | `X(): CancellablePromise<T>`; a non-nil error rejects the Promise, message in `e.message` |
| First parameter is `context.Context` | Not passed by the frontend; calling `.cancel()` on the promise cancels this ctx |
| struct | An interface in `models.ts` (named by `json` tags) |
| nil slice | `null` (typed as `T[] \| null`) |

## Events

After registering with `application.RegisterEvent[T]("name")`, the generator outputs:

- `frontend/bindings/events/index.ts`: event name constants (imported via `@events` in the frontend)
- A type augmentation for `@wailsio/runtime`: `ev.data` in `Events.On(name, ev => ev.data)` is inferred automatically

Emit an event: `application.Get().Event.Emit(name, payload)`.

## Current services

### FileService

Opens native file dialogs in Go and keeps an allowlist of the paths the user chose. This prevents path attacks: even if the WebView were compromised (e.g. XSS from rendered table data), it could not make the backend open or create arbitrary files.

| Method | Description |
|---|---|
| `PickFile(kind)` | Opens the dialog for `kind` (`FileKind` enum), adds the chosen path to the allowlist and returns it; `""` when cancelled |

| `FileKind` | Dialog |
|---|---|
| `sqliteOpen` / `sqliteCreate` | Open / save dialog for SQLite databases |
| `caCert`, `clientCert`, `clientKey` | Open dialog filtered to PEM files |
| `sshPrivateKey`, `knownHosts` | Open dialog starting in `~/.ssh` with hidden files shown |
| `kubeconfig` | Open dialog starting in `~/.kube` with hidden files shown |

Unexported helpers used by other services in this package:

- `checkFile(path)`: the same check for one optional path (`""` passes), used by the Kubernetes discovery methods.
- `checkPaths(cfg)`: fails with `ErrPathNotPicked` unless every path in `cfg.FilePaths()` is on the allowlist. Services must call it before using a config from the frontend.
- `allow(paths...)`: adds paths the backend itself loaded (e.g. from the saved store).

**These must stay unexported**: every exported method becomes a frontend binding, and an exported `allow` would let the frontend whitelist any path.

`main.go` creates one `FileService` and shares it with the services that need `checkPaths`. Tests replace `prompt` instead of opening real dialogs; `showDialog` itself needs a running app and is not unit tested.

### ConnectionService

| Method | Description |
|---|---|
| `TestConnection(ctx, draft)` | `checkPaths`, then connect, detect the server and disconnect (`conn.Probe`); returns `ServerInfo` (e.g. MariaDB 10.11.6); nothing is cached |
| `ListDatabases(ctx, draft)` | `checkPaths`, then a pooled connection and `conn.ListDatabases` |
| `DetectSocket(ctx, draft)` | Finds the database's Unix socket on the draft's SSH server (`conn.DetectSocket`) |
| `K8sContexts(kubeconfig)`, `K8sNamespaces(ctx, kubeconfig, context)`, `K8sTargets(ctx, kubeconfig, context, namespace)` | Kubernetes discovery for the form; an explicit kubeconfig path must come from `PickFile` |
| `ListSaved()` | Saved connections without passwords; their file paths are added to the allowlist |
| `SaveConnection(c, updateSecrets)` | `checkPaths` + `Validate`, then `store.Save` (only the named secrets are written); closes the pooled connection of the previous settings |
| `DeleteConnection(id)` | Removes the connection, its keychain entries and its pooled connection |
| `TestSavedConnection(ctx, id)`, `ListSavedDatabases(ctx, id)` | Same as above for a saved connection (Test also returns `ServerInfo`); passwords come from the keychain |
| `ServiceShutdown()` | Called by Wails on exit; closes every pooled connection |

Methods taking `ctx` can be cancelled from the frontend. List methods never return `null` from Go, but the generated types still say `T[] | null`; wrap them with `arr()`.

Saved connections are addressed by id so passwords never travel back to the frontend.

`ConnectionDraft{id, config, updateSecrets}` is the connection form's content. When `id` is set, every secret not named in `updateSecrets` is filled in from the keychain before connecting, so "Test connection" works on an edited saved connection whose passwords the frontend never had.
