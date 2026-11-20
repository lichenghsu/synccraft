# internal/app

Wails 綁定層。這裡每個 Service 的**公開方法**都會自動產生前端可呼叫的 TypeScript 函式（輸出到 `frontend/bindings/github.com/lichenghsu/synccraft/internal/app/`）。

## 原則

- **只有這個套件 import Wails**。資料庫連線、比對演算法等核心邏輯放在 `internal/` 的其他套件，這裡只負責參數轉換和呼叫下層，方便用 `go test` 單獨測試核心邏輯。
- exported 識別字的 doc comment 會帶進產生的 TS 綁定，保持簡短的英文。
- 改了方法簽章後要重新產生綁定並提交：`wails3 generate bindings -clean=true -ts -i`。CI 會檢查綁定是否同步。

## Go → TypeScript 對應

| Go | TypeScript |
|---|---|
| `func X() (T, error)` | `X(): CancellablePromise<T>`；error 非 nil 時 Promise reject，訊息在 `e.message` |
| 第一個參數是 `context.Context` | 前端不用傳；前端對 promise 呼叫 `.cancel()` 時，這個 ctx 會被取消 |
| struct | `models.ts` 裡的 interface（依 `json` tag 命名） |
| nil slice | `null`（型別會標成 `T[] \| null`） |

## 事件

用 `application.RegisterEvent[T]("name")` 註冊後，產生器會輸出：

- `frontend/bindings/events/index.ts`：事件名稱常數（前端用 `@events` 匯入）
- `@wailsio/runtime` 的型別擴充：`Events.On(name, ev => ev.data)` 的 `ev.data` 型別自動推導

送出事件：`application.Get().Event.Emit(name, payload)`。

## 目前的 Service

### HelloService（M0 示範）

驗證 Go ↔ React 的三種串接方式，M1 之後會移除。

| 方法 | 說明 |
|---|---|
| `Hello(name)` | 回傳問候語；`name` 空白時回傳 `ErrEmptyName`，前端 Promise 會被 reject |
| `Count(ctx, total, intervalMs)` | 每 `intervalMs` 毫秒數一次並送出 `count:tick` 事件（payload：`CountTick{current, total}`）；前端取消時立即回傳 `ctx.Err()` |

`emit` 欄位是為了測試可以替換掉 Wails 的事件系統，見 `hello_service_test.go`。
