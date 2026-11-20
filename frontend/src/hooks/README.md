# src/hooks

## useAsync(fn)

包裝回傳 Promise 的函式，追蹤呼叫狀態。

```ts
const hello = useAsync(api.hello);
await hello.run("Miles");
hello.state  // { status: "idle" | "loading" | "success" | "error", data?, error? }
```

`state` 是判別聯集（`AsyncState<T>`，定義在 `src/types.ts`）：在 `status === "success"` 的分支裡 TS 才允許讀 `data`。錯誤不會往外丟，會放進 `state.error`。

## useCancellable(fn)

可取消的長時間呼叫，`fn` 必須回傳 `CancellablePromise`。

```ts
const count = useCancellable(api.count);
await count.run(50, 80);   // 再次呼叫 run 會先取消上一次
count.cancel();            // Go 端的 context.Context 會被取消
count.running              // 是否進行中
```

- 被取消時 `run` 會 reject，用 `isCancel(e)` 判斷。
- 元件卸載時自動取消進行中的呼叫，不會在背景繼續跑。

## useWailsEvent(name, handler)

訂閱 Go 端送出的事件。

```ts
import { CountTick } from "@events";
useWailsEvent(CountTick, (tick) => setTick(tick));   // tick 的型別自動推導
```

- 事件名稱用 `@events` 匯出的常數，payload 型別來自 Go 端 `application.RegisterEvent[T]` 註冊的型別。
- `handler` 用 ref 保存：每次 render 傳入新的函式也不會重新訂閱。
- 元件卸載時自動取消訂閱。
