# 第 2 部　Wails v3 核心：Go 與前端如何串接

## 2.1 先看原版（v2）怎麼做

```go
//go:embed all:frontend/dist
var assets embed.FS

func main() {
    app := NewApp()
    err := wails.Run(&options.App{
        Title: "SyncForge", Width: 1280, Height: 800,
        AssetServer: &assetserver.Options{Assets: assets},
        OnStartup:   app.startup,
        Bind:        []interface{}{app},   // App 的所有公開方法暴露給 JS
    })
}
```

v2 的特點：只有一個視窗、一個 `App` struct 塞滿所有方法、要靠 `ctx` 呼叫 `runtime.XXX(ctx, ...)`。

## 2.2 v3 的進入點

```go
package main

import (
    "embed"
    "log"

    "github.com/wailsapp/wails/v3/pkg/application"
    "github.com/<you>/synccraft/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
    pool := app.NewPool()
    store := app.MustNewStore()

    a := application.New(application.Options{
        Name:        "SyncCraft",
        Description: "Database schema & data compare",
        Services: []application.Service{
            application.NewService(app.NewConnService(pool, store)),
            application.NewService(app.NewSchemaService(pool)),
            application.NewService(app.NewDataService(pool)),
            application.NewService(app.NewCompareService(pool)),
        },
        Assets: application.AssetOptions{
            Handler: application.AssetFileServerFS(assets),
        },
        Mac: application.MacOptions{
            ApplicationShouldTerminateAfterLastWindowClosed: true,
        },
    })

    a.Window.NewWithOptions(application.WebviewWindowOptions{
        Title:            "SyncCraft",
        Width:            1280,
        Height:           800,
        BackgroundColour: application.NewRGB(27, 38, 54),
        URL:              "/",
    })

    if err := a.Run(); err != nil {
        log.Fatal(err)
    }
}
```

三個重點：

1. **`//go:embed all:frontend/dist`**：Vite 打包好的前端整包嵌進執行檔，最後只有一個檔案。
2. **`Services`**：每個 Service 是一個普通的 Go struct，**公開方法**會自動變成前端可呼叫的函式。
3. **視窗是明確建立的物件**：`a.Window.NewWithOptions(...)` 可以呼叫很多次 → 多視窗（第 9 部的雙表比對可以開第二個視窗）。

### v2 → v3 對照表

| 概念 | v2（原版） | v3（SyncCraft） |
|---|---|---|
| 啟動 | `wails.Run(&options.App{...})` | `application.New(...)` + `app.Run()` |
| 暴露給前端 | `Bind: []interface{}{app}` | `Services: []application.Service{application.NewService(x)}` |
| 生命週期 | `OnStartup(ctx)`、`OnShutdown(ctx)` | Service 實作 `ServiceStartup` / `ServiceShutdown` 介面 |
| 執行期 API | `runtime.EventsEmit(ctx, ...)` | `app.Event.Emit(...)`（manager 物件） |
| 對話框 | `runtime.OpenFileDialog(ctx, ...)` | `app.Dialog.OpenFile().AddFilter(...).PromptForSingleSelection()` |
| 前端綁定 | `wailsjs/go/main/App.js` | `bindings/<module>/<pkg>/<service>.ts` |
| 前端 runtime | `wailsjs/runtime` | npm `@wailsio/runtime` |
| 取消呼叫 | 不支援 | 方法第一個參數放 `context.Context`，前端 `promise.cancel()` |
| 視窗 | 只有一個 | 任意多個 |
| 建置 | `wails build`（黑盒子） | Taskfile（看得到、可修改） |

## 2.3 Service 的規則

```go
package app

type SchemaService struct {
    pool *Pool
}

func NewSchemaService(p *Pool) *SchemaService { return &SchemaService{pool: p} }

// CompareSchemas 比較兩個資料庫的結構。
// 這段註解會一起出現在產生的 TypeScript 裡。
func (s *SchemaService) CompareSchemas(ctx context.Context, src, dst ConnectionRef) ([]Change, error) {
    ...
}

func (s *SchemaService) helper() {}    // 小寫 → 不會暴露
```

| Go 簽章 | TS 端 |
|---|---|
| `func X() T` | `X(): CancellablePromise<T>` |
| `func X() (T, error)` | error 非 nil → Promise **reject**，錯誤物件是 `RuntimeError`，訊息在 `.message` |
| `func X() error` | `CancellablePromise<void>` |
| `func X(ctx context.Context, a A)` | 前端只要傳 `a`；**`ctx` 由 Wails 自動注入，前端取消呼叫時這個 ctx 會被 cancel** |
| 參數或回傳是 struct | 走 JSON，依 `json:"..."` tag 對應；會產生對應的 TS 型別 |

JSON 序列化的注意事項（v2、v3 都一樣）：

- `map[string]any` → `{ [_: string]: any }`
- `*string` → `string | null`
- `time.Time` → ISO 字串
- `[]byte` → base64 字串
- **nil slice → `null`**：v3 產生的型別會老實標成 `T[] | null`，TS 會逼你處理（v2 只標 `T[]`，所以原版前端到處寫 `|| []`）。比較好的做法是 Go 端回傳 `make([]T, 0)`。
- `int64` 超過 2^53 在 JS 會失真 → 大 ID 在 Go 端轉成字串。

### 生命週期

```go
// 實作這兩個介面（選用），Wails 會在適當時機呼叫
func (s *ConnService) ServiceStartup(ctx context.Context, opts application.ServiceOptions) error {
    return s.store.Load()        // 讀取已存的連線
}

func (s *ConnService) ServiceShutdown() error {
    s.pool.CloseAll()            // 關閉所有連線池
    return nil
}
```

### 在 Service 裡取得 App

```go
app := application.Get()        // 取得目前的 *application.App
app.Event.Emit("sync:progress", p)
```

## 2.4 自動產生的綁定

`wails3 dev` / `wails3 build` 會用**靜態分析**讀 Go 原始碼，產生：

```
frontend/bindings/
├── events/index.ts                      ← RegisterEvent 註冊的事件名稱常數
└── github.com/<you>/synccraft/
    └── internal/app/
        ├── index.ts
        ├── schemaservice.ts             ← 每個 Service 一個檔案
        └── models.ts                    ← 用到的 struct 型別
```

手動產生（例如 CI 檢查綁定有沒有更新）：

```bash
wails3 generate bindings -clean=true -ts -i
```

> ⚠️ 一定要帶參數。不帶參數時預設產生 **JS 檔**（型別寫在 JSDoc 裡）；`-ts` 產生 TypeScript、`-i` 產生 interface、`-clean=true` 先清掉舊檔。這組參數與 `build/Taskfile.yml` 的 `generate:bindings` 任務一致，`wails3 dev` / `build` 產出的結果會完全相同（v3.1.0 實測）。

產生的程式碼長這樣（示意）：

```ts
// schemaservice.ts
import { Call, CancellablePromise } from "@wailsio/runtime";
import * as $models from "./models.js";

/**
 * CompareSchemas 比較兩個資料庫的結構。
 */
export function CompareSchemas(src: $models.ConnectionRef, dst: $models.ConnectionRef):
    CancellablePromise<$models.Change[] | null> { ... }
```

- Go 的 doc comment 和**真正的參數名稱**都會帶過來（v2 只有 `arg1, arg2`）。
- `wails3 init` 預設產生 **interface**（不是 class），所以傳參數直接用物件字面值即可。
- **不要手改 `bindings/`**。要不要提交到 git 見第 12 部（SyncCraft 選擇提交）。

## 2.5 前端呼叫（React）

```tsx
import { SchemaService } from "@bindings/internal/app";

async function onCompare(src: ConnectionRef, dst: ConnectionRef) {
  try {
    const changes = (await SchemaService.CompareSchemas(src, dst)) ?? [];
    setResults(changes);
  } catch (e) {
    setError(e instanceof Error ? e.message : String(e));
  }
}
```

### 取消長時間的呼叫

```tsx
const p = DataService.CompareTables(src, dst, tables);   // CancellablePromise
setRunning(p);
// 使用者按「取消」：
p.cancel();       // Go 端的 ctx.Done() 會被觸發
```

Go 端只要把 ctx 一路往下傳：

```go
func (s *DataService) CompareTables(ctx context.Context, src, dst ConnectionRef, tables []string) error {
    for _, t := range tables {
        if err := ctx.Err(); err != nil { return err }     // 已取消就提早結束
        rows, err := db.QueryContext(ctx, q)                // 查詢也會被中斷
        ...
    }
}
```

v2 要自己管 `cancelFunc` 和 mutex（原版沒有實作取消）；v3 直接內建。

## 2.6 事件（Events）

原版沒有用事件，資料同步的進度條是前端用 `delay()` 假裝的。SyncCraft 改用真事件。

**Go 端：註冊事件型別並發送**

```go
type SyncProgress struct {
    Table string `json:"table"`
    Done  int    `json:"done"`
    Total int    `json:"total"`
}

func init() {
    // 註冊後，綁定產生器會為這個事件產生有型別的 TS API
    application.RegisterEvent[SyncProgress]("sync:progress")
}

func (s *DataService) CompareTables(ctx context.Context, ...) error {
    app := application.Get()
    for i, t := range tables {
        app.Event.Emit("sync:progress", SyncProgress{Table: t, Done: i, Total: len(tables)})
        ...
    }
    return nil
}
```

**React 端：訂閱**

```tsx
import { Events } from "@wailsio/runtime";
import { SyncProgress } from "@events";      // 產生器輸出的事件名稱常數 = "sync:progress"

useEffect(() => {
  const off = Events.On(SyncProgress, (ev) => {
    setProgress(ev.data);         // ev.data 的型別自動推導為 SyncProgress struct
  });
  return off;                     // Events.On 回傳取消訂閱函式，unmount 時呼叫
}, []);
```

原理：產生器輸出一個 `.d.ts`，用 module augmentation 把事件名稱和 payload 型別加進 `@wailsio/runtime` 的 `Events.CustomEvents` 介面。SyncCraft 的 `useWailsEvent` hook 就是利用這點做出完全型別安全的訂閱（見 `frontend/src/hooks/useWailsEvent.ts`）。

## 2.7 原生對話框

SQLite 選檔（原版是讓使用者手打路徑）：

```go
func (s *ConnService) PickSQLiteFile() (string, error) {
    return application.Get().Dialog.OpenFile().
        SetTitle("選擇 SQLite 檔案").
        AddFilter("SQLite", "*.db;*.sqlite;*.sqlite3").
        PromptForSingleSelection()
}
```

匯出 CSV 時的另存新檔：`app.Dialog.SaveFile().SetFilename("diff.csv").PromptForSingleSelection()`。

確認對話框（例如執行 DROP 前）：`app.Dialog.Question().SetTitle(...).SetMessage(...)`，加上按鈕後 `.Show()`。

## 2.8 SyncCraft 的 Go 專案結構

```
SyncCraft/
├── main.go                    # application.New + 視窗
├── internal/
│   ├── app/                   # Service 層（Wails 綁定的入口）：只做參數轉換、呼叫下層
│   │   ├── conn_service.go
│   │   ├── schema_service.go
│   │   ├── data_service.go
│   │   └── compare_service.go
│   ├── db/                    # Pool、DSN（第 3 部）
│   ├── dialect/               # 每種 DB 的 SQL（第 4、5 部）
│   ├── schema/                # 結構比對演算法（純函式，好測）
│   ├── datasync/              # 資料比對（第 6 部）
│   ├── pair/                  # 雙表並排比對（第 9 部）
│   └── store/                 # 連線儲存（第 7 部）
└── frontend/
```

原則：**`internal/app` 以外的套件完全不 import Wails**。這樣比對邏輯可以用 `go test` 單獨測試，未來就算換框架也不用改核心。

## 練習

1. 把範本的 `GreetService` 改名成 `HelloService`，加一個 `Hello(name string) (string, error)`，name 為空時回傳 error；前端分別顯示成功與錯誤。
2. 觀察 `frontend/bindings/` 的變化，確認 doc comment 有帶過去。
3. 寫一個 `Count(ctx context.Context, n int) error`：每 100ms 發一次 `count:tick` 事件；前端顯示數字並提供「取消」按鈕呼叫 `.cancel()`。
