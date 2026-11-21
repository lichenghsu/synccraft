# src/api

Wrappers around the Go bindings. **Components import only from here, never from `@bindings` directly**: when a Go Service is split or renamed, only this layer changes.

## wails.ts

| Export | Description |
|---|---|
| `api` | Thin wrappers around each Service method |
| `FileKind` | Re-exported enum for `api.pickFile` |
| `Connection`, `ConnectionConfig` | Re-exported types for the connection APIs |
| `arr(x)` | Go nil slices serialize to `null`; normalizes them to `[]` |
| `isCancel(e)` | Whether the error is a user-initiated cancel (`CancelError`) |
| `errMsg(e)` | Turns any error into a displayable string; errors returned from Go become an `Error` with `message` |

## Cancellation

Service methods return a `CancellablePromise`. For calls that need cancelling (e.g. `api.count`), **return the original promise directly; do not wrap it in `async`**: an `async` function returns a plain `Promise` and `.cancel()` is lost.

Calls that don't need cancelling but must handle nil slices can be wrapped in `async`:

```ts
getDatabases: async (c: ConnectionConfig) => arr(await ConnService.GetDatabases(c)),
```

## File paths

File paths (SQLite database, TLS certificates, SSH key, known_hosts) are **never typed in or built by the frontend**. Show a button that calls `api.pickFile(FileKind.X)`; it opens a native dialog in Go and resolves to the chosen path (or `""` when cancelled). Display the path read-only and send it back unchanged in the connection config.

The backend rejects any path that did not come from `pickFile` (or from the saved store). Don't use `Dialogs.OpenFile` from `@wailsio/runtime` for these fields: paths picked that way are not on the backend's allowlist.

## Connections

| Call | When |
|---|---|
| `testConnection(cfg)`, `listDatabases(cfg)` | An unsaved config typed into the form (password included) |
| `listSaved()` | Saved connections, **without passwords** |
| `saveConnection(c, updateSecrets)` | Create (`c.id === ""`) or update. Pass `updateSecrets: true` only when the user edited a password field; `false` keeps the stored passwords |
| `testSaved(id)`, `listSavedDatabases(id)`, `deleteConnection(id)` | Saved connections, by id; the backend fills in the passwords |

Call `listSaved()` before using a saved connection's file paths: it also puts those paths on the backend's allowlist.
