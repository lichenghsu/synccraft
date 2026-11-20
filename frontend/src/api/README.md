# src/api

包裝 Go 綁定。**元件只 import 這裡，不直接 import `@bindings`**：Go 端拆分或重新命名 Service 時，只需要改這一層。

## wails.ts

| 匯出 | 說明 |
|---|---|
| `api` | 各 Service 方法的薄包裝 |
| `arr(x)` | Go 的 nil slice 會序列化成 `null`，統一轉成 `[]` |
| `isCancel(e)` | 是否為使用者主動取消（`CancelError`） |
| `errMsg(e)` | 把任何錯誤轉成可顯示的字串；Go 回傳的 error 會變成帶 `message` 的 `Error` |

## 取消

Service 方法回傳的是 `CancellablePromise`。需要取消的呼叫（例如 `api.count`）**直接回傳原本的 promise，不要用 `async` 包**：`async` 函式回傳一般的 `Promise`，`.cancel()` 就不見了。

不需要取消、但要處理 nil slice 的呼叫，可以用 `async` 包：

```ts
getDatabases: async (c: ConnectionConfig) => arr(await ConnService.GetDatabases(c)),
```
