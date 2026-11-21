# src/hooks

## useAsync(fn)

Wraps a Promise-returning function and tracks the call state.

```ts
const hello = useAsync(api.hello);
await hello.run("Miles");
hello.state  // { status: "idle" | "loading" | "success" | "error", data?, error? }
```

`state` is a discriminated union (`AsyncState<T>`, defined in `src/types.ts`): TS only lets you read `data` inside the `status === "success"` branch. Errors are not thrown; they go into `state.error`.

## useCancellable(fn)

A cancellable long-running call; `fn` must return a `CancellablePromise`.

```ts
const count = useCancellable(api.count);
await count.run(50, 80);   // calling run again cancels the previous call first
count.cancel();            // cancels the Go-side context.Context
count.running              // whether a call is in progress
```

- When cancelled, `run` rejects; check with `isCancel(e)`.
- An in-progress call is cancelled automatically on unmount, so nothing keeps running in the background.

## useWailsEvent(name, handler)

Subscribes to an event emitted from Go.

```ts
import { CountTick } from "@events";
useWailsEvent(CountTick, (tick) => setTick(tick));   // tick's type is inferred
```

- Use the event name constants exported from `@events`; the payload type comes from the type registered with `application.RegisterEvent[T]` in Go.
- `handler` is kept in a ref: passing a new function on every render does not resubscribe.
- Unsubscribes automatically on unmount.
