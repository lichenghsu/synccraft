# src/hooks

## useAsync(fn)

Wraps a Promise-returning function and tracks the call state.

```ts
const load = useAsync(api.listSaved);
await load.run();
load.state  // { status: "idle" | "loading" | "success" | "error", data?, error? }
```

`state` is a discriminated union (`AsyncState<T>`, defined in `src/types.ts`): TS only lets you read `data` inside the `status === "success"` branch. Errors are not thrown; they go into `state.error`.

## useCancellable(fn)

A cancellable long-running call; `fn` must return a `CancellablePromise`.

```ts
const test = useCancellable(api.testConnection);
await test.run(draft);     // calling run again cancels the previous call first
test.cancel();             // cancels the Go-side context.Context
test.running               // whether a call is in progress
```

- When cancelled, `run` rejects; check with `isCancel(e)`.
- An in-progress call is cancelled automatically on unmount, so nothing keeps running in the background.

## useWailsEvent(name, handler)

Subscribes to an event emitted from Go.

```ts
import { SyncProgress } from "@events";             // hypothetical, once Go registers it
useWailsEvent(SyncProgress, (p) => setProgress(p)); // p's type is inferred
```

No custom events are registered right now (the M0 `count:tick` demo was removed); the hook is kept for M4's sync progress.

- Use the event name constants exported from `@events`; the payload type comes from the type registered with `application.RegisterEvent[T]` in Go.
- `handler` is kept in a ref: passing a new function on every render does not resubscribe.
- Unsubscribes automatically on unmount.
