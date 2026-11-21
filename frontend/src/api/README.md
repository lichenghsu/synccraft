# src/api

Wrappers around the Go bindings. **Components import only from here, never from `@bindings` directly**: when a Go Service is split or renamed, only this layer changes.

## wails.ts

| Export | Description |
|---|---|
| `api` | Thin wrappers around each Service method |
| `arr(x)` | Go nil slices serialize to `null`; normalizes them to `[]` |
| `isCancel(e)` | Whether the error is a user-initiated cancel (`CancelError`) |
| `errMsg(e)` | Turns any error into a displayable string; errors returned from Go become an `Error` with `message` |

## Cancellation

Service methods return a `CancellablePromise`. For calls that need cancelling (e.g. `api.count`), **return the original promise directly; do not wrap it in `async`**: an `async` function returns a plain `Promise` and `.cancel()` is lost.

Calls that don't need cancelling but must handle nil slices can be wrapped in `async`:

```ts
getDatabases: async (c: ConnectionConfig) => arr(await ConnService.GetDatabases(c)),
```
