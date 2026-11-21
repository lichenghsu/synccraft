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

### HelloService (M0 demo)

Exercises the three ways Go and React talk to each other; to be removed after M1.

| Method | Description |
|---|---|
| `Hello(name)` | Returns a greeting; a blank `name` returns `ErrEmptyName` and rejects the frontend Promise |
| `Count(ctx, total, intervalMs)` | Counts every `intervalMs` ms and emits a `count:tick` event (payload: `CountTick{current, total}`); returns `ctx.Err()` immediately when the frontend cancels |

The `emit` field lets tests replace the Wails event system; see `hello_service_test.go`.
