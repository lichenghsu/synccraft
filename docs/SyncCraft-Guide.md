# 第 0 部　總覽與學習路線

> **SyncCraft** 是 [nanablast/syncforge](https://github.com/nanablast/syncforge) 的複刻版。
> 原版：Go + Wails **v2** + **Vue 3**；SyncCraft：Go + Wails **v3** + **React + TypeScript**。
> 本書以原版原始碼（`~/Lab/syncforge/source`）為教材，逐部拆解 SQL、Go ↔ 前端串接、React/TS 寫法與跨平台打包。

## 0.1 SyncForge 在做什麼？

一個「資料庫比對 + 同步」的桌面工具（Navicat 的 Data/Structure Synchronization 免費版）：

| 功能 | 說明 | 原版檔案 |
|---|---|---|
| 連線管理 | MySQL / PostgreSQL / SQLite / SQL Server，可存檔 | `database/connections.go` |
| Schema 比對 | 比兩個 DB 的表、欄位、索引，產生 `CREATE / ALTER / DROP` | `database/database.go` |
| 資料同步 | 依主鍵逐列比對，產生 `INSERT / UPDATE / DELETE` | `database/data_sync.go` |
| 資料表瀏覽 | 看結構、看資料（分頁）、看 `CREATE TABLE` | `database/table_browser.go` |
| 自動更新 | 查 GitHub Releases、下載、替換執行檔 | `updater/updater.go` |

## 0.2 整體架構

```
┌──────────────────────── 桌面視窗 (Wails) ─────────────────────────┐
│                                                                   │
│  WebView（macOS: WKWebView / Windows: WebView2 / Linux: WebKitGTK）│
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │ React + TypeScript (Vite 打包成 frontend/dist)              │  │
│  │   import { SchemaService } from '@bindings/internal/app'    │  │
│  └───────────────▲──────────────────────────┬──────────────────┘  │
│                  │ Promise 結果 (JSON)       │ 呼叫 (JSON)        │
│  ┌───────────────┴──────────────────────────▼──────────────────┐  │
│  │ Go：各 Service 的公開方法（application.NewService）          │  │
│  │   → database 套件 → database/sql → 各 driver                │  │
│  └─────────────────────────────┬───────────────────────────────┘  │
└────────────────────────────────┼──────────────────────────────────┘
                                 │ TCP / 檔案
             MySQL · PostgreSQL · SQL Server · SQLite
```

重點：**前端完全不碰資料庫**。所有 SQL 都在 Go 端執行，前端只透過 Wails 自動產生的 TypeScript 函式呼叫 Go 方法。

## 0.3 本書章節

| 部 | 主題 | 你會學到 |
|---|---|---|
| 1 | 環境準備 | Go、Node、wails3 CLI（mise）、三平台系統相依、建立 React + TS 專案 |
| 2 | Wails v3 核心 | Services、自動產生綁定、取消呼叫、Events、對話框、v2→v3 對照 |
| 3 | Go 連線層 | `database/sql`、四種 driver、DSN、連線池 |
| 4 | **SQL 大全** | 四種 DB 的 metadata 查詢逐條拆解 + 對照表 |
| 5 | Schema 比對 | 比對演算法、DDL 生成、原版缺陷與 Dialect 重構 |
| 6 | 資料同步 | 主鍵對齊、DML 生成、跳脫、交易、大表策略 |
| 7 | 表瀏覽與連線儲存 | 分頁 SQL、connections.json、密碼安全 |
| 8 | React + TS | 專案結構、型別、hooks、狀態管理、把 Vue 元件改寫成 React |
| 9 | **雙表並排比對**（新功能） | 任意兩張表（跨主機 / 跨 DB 種類）比筆數、欄位、逐列差異、欄位統計 |
| 10 | 跨平台打包 | macOS（M 系列）/ Windows / Linux、CGO 陷阱、簽章、CI |
| 11 | 實作路線圖 | 里程碑、驗收清單、原版問題總表 |
| 12 | 長期維護 | 版本管理、Renovate 自動升級、CI 把關、升級節奏、授權 |

## 0.4 建議學習方式

1. 每一部先讀「原版怎麼做」，再讀「SyncCraft 怎麼改」。
2. 每部結尾有 **練習**，做完再往下。
3. 準備一組本機測試資料庫（第 1 部有 `docker compose`），所有 SQL 都親手跑一次。

## 0.5 原版的關鍵觀察（先記住，後面會反覆提到）

- Schema 比對產生的 DDL **全部是 MySQL 語法**（反引號、`MODIFY COLUMN`、`AFTER`），對 PG / SQL Server 目標會失敗。
- 資料比對把 **整張表載入記憶體**，大表會爆。
- 每次呼叫都 `sql.Open` + `Close`，沒有重用連線池。
- `mattn/go-sqlite3` 需要 **CGO**，讓 Windows 交叉編譯變困難。
- 使用 Wails **v2**；SyncCraft 直接採用 v3（多視窗、可取消的呼叫、內建打包與簽章更新）。
- 密碼以明文存在 `~/.syncforge/connections.json`。
- 識別字（表名）用 `fmt.Sprintf` 拼接且沒跳脫內含的引號字元。

這些就是 SyncCraft「複刻 + 改良」的切入點。


# 第 1 部　環境準備（Wails v3）

> SyncCraft 使用 **Wails v3**（本書以 v3.1.0 為準）。原版 SyncForge 是 v2，兩者 API 差很多，對照原始碼時請記得這點；第 2 部有 v2 → v3 對照表。

## 1.1 共同需求

| 工具 | 版本 | 用途 |
|---|---|---|
| Go | **1.26 以上**（Wails v3 的硬性要求） | 後端、Wails 本體 |
| Node.js | 偶數版 LTS | 前端建置（Vite） |
| Wails CLI | `wails3` v3.1.x | 建專案、dev、build、package |
| Task | 選用 | `wails3 task` 已內建；想直接打 `task` 才需要另外裝 |

### 用 mise 安裝在專案工作區（SyncCraft 的做法）

專案根目錄的 `mise.toml`：

```toml
# 只在 SyncCraft 工作區生效的工具
# Go 使用系統版本（Wails v3.1 需要 Go >= 1.26）
[tools]
"go:github.com/wailsapp/wails/v3/cmd/wails3" = "3.1.0"
```

```bash
cd SyncCraft
mise trust && mise install      # 只有在此資料夾內才有 wails3 指令
wails3 version
```

- mise 的內建清單裡沒有 wails，所以 `mise use wails` 會失敗，必須用 `go:` backend 加上完整的模組路徑。
- 不用 mise 的話：`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`（會裝到 `~/go/bin`，全使用者生效）。

> ⚠️ **連 `wails3` CLI 本身編譯時都需要系統的 GUI 開發套件**（下一節）。在 Linux 上沒有先裝 GTK4，`mise install` 就會失敗並出現 `Package 'gtk4' not found`。

### 一鍵檢查

```bash
wails3 doctor      # 系統狀態報告
wails3 setup       # 互動式精靈：檢查並協助安裝缺少的相依
```

## 1.2 各平台系統相依

### Linux（Fedora 為例）

**Wails v3 在 Linux 預設使用 GTK4 + WebKitGTK 6.0**（v2 用的是 GTK3 + webkit2gtk-4.1）。

```bash
# Fedora
sudo dnf install gcc pkgconf-pkg-config gtk4-devel webkitgtk6.0-devel
# Ubuntu 24.04+ / Debian 13+
sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev

pkg-config --modversion gtk4 webkitgtk-6.0     # 兩個都印出版本號就 OK
```

| 後端 | build tag | 系統套件 | 狀態 |
|---|---|---|---|
| GTK4 + WebKitGTK 6.0 | （預設） | `gtk4-devel`、`webkitgtk6.0-devel` | ✅ 推薦 |
| GTK3 + webkit2gtk-4.1 | `-tags gtk3` | `gtk3-devel`、`webkit2gtk4.1-devel` | 舊版相容，官方已預告會移除 |
| purego（不需 CGO） | `-tags purego` 或 `WAILS_PUREGO=1` | 執行時才用 `dlopen` 載入 GTK | 實驗性 |

> v2 時代需要的 `-tags webkit2_41` 在 v3 **不再需要**。

### macOS（含 M1–M4）

```bash
xcode-select --install     # Command Line Tools（clang，CGO 需要）
```

WebView 使用系統的 WKWebView。Apple Silicon 原生就是 `darwin/arm64`。

### Windows 10/11

- WebView2 Runtime：Windows 11 內建；Windows 10 多半也已經有。
- 純 Go 專案**不需要 gcc**（Windows 後端不使用 CGO）。
- 若要產生 NSIS 安裝程式：`winget install NSIS.NSIS`。

## 1.3 建立 SyncCraft 專案

```bash
# ⚠️ -d 是「上層目錄」：專案會建在 <-d>/<-n> 底下
wails3 init -n synccraft -t react -d /tmp/wails-init \
  -mod github.com/<你的帳號>/synccraft \
  -productname SyncCraft \
  -productidentifier dev.synccraft.app \
  -productcompany SyncCraft \
  -productversion 0.1.0

# 再把產生的檔案搬進既有的專案資料夾（-n 不覆蓋已存在的檔案，例如你自己的 .gitignore）
cp -rn /tmp/wails-init/synccraft/{build,frontend,go.mod,go.sum,main.go,greetservice.go,Taskfile.yml} .
```

- **`-d` 的陷阱**：`-d .` 不會「在目前資料夾初始化」，而是建立 `./synccraft/` 子資料夾（v3.1.0 實測）。資料夾裡已經有東西（文件、設定檔）時，先建到暫存位置再搬過來最安全。
- **`build/config.yml` 不會吃到 `-product*` 參數**（v3.1.0 實測）：這些參數只寫進各平台檔案（Info.plist、NSIS、nfpm…），`config.yml` 仍是 `My Product`。請手動把 `info:` 區塊改成一致，否則日後執行 `common:update:build-assets` 時，名稱會被蓋回預設值。
- `-t react` 是 **React + TypeScript**（`react-js` 才是純 JS）。可用 `wails3 init -l` 列出所有範本。
- `-mod`：Go module 路徑。之後要放上 GitHub 就直接填 `github.com/帳號/synccraft`，綁定程式碼的目錄結構會跟著這個路徑走。
- 不想記參數的話：`wails3 init --ui` 會在瀏覽器開設定精靈。

產生的結構：

```
SyncCraft/
├── main.go               # application.New(...) + 建立視窗
├── greetservice.go       # 範例 Service（之後刪掉）
├── go.mod
├── Taskfile.yml          # 根 Taskfile：依 GOOS 分派到 build/<os>/Taskfile.yml
├── build/
│   ├── config.yml        # App 名稱、版本、識別碼等 metadata
│   ├── Taskfile.yml      # 共用任務：前端建置、產生綁定、產生圖示
│   ├── appicon.png       # 1024×1024 原始圖示 → 自動產生各平台格式
│   ├── darwin/  linux/  windows/  docker/   # 各平台任務與打包設定
└── frontend/
    ├── package.json      # 依賴 @wailsio/runtime
    ├── vite.config.ts    # 含 wails vite plugin
    ├── src/main.tsx  src/App.tsx
    └── bindings/         # ← 由 Go 程式碼自動產生，不要手改
        ├── events/index.ts                  # 有型別的事件名稱常數
        └── github.com/<你的帳號>/synccraft/
            ├── index.ts
            └── greetservice.ts
```

與 v2 的差異：

| v2 | v3 |
|---|---|
| `wails.json` | `build/config.yml` + `Taskfile.yml` |
| `frontend/wailsjs/go/main/App.d.ts` | `frontend/bindings/<module>/<service>.ts` |
| `frontend/wailsjs/runtime` | npm 套件 `@wailsio/runtime` |
| `build/bin/` | `bin/` |

### 升級前端套件

範本內的 React 版本偏舊（`^18`），建立專案後立刻升級一次：

```bash
cd frontend
npm install react@latest react-dom@latest @wailsio/runtime@latest
npm install -D vite@latest @vitejs/plugin-react@latest typescript@latest \
               @types/react@latest @types/react-dom@latest
```

`vite.config.ts`（範本產生的，注意 wails plugin）：

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), wails("./bindings")],
});
```

### 綁定路徑的別名（建議）

`frontend/bindings/github.com/<帳號>/synccraft/...` 路徑很長，用別名縮短。**tsconfig 與 vite 兩邊都要設**（TS 負責型別檢查、Vite 負責實際打包）：

```jsonc
// frontend/tsconfig.json（TypeScript 7 已移除 baseUrl，paths 直接相對於 tsconfig 檔案）
{
  "compilerOptions": {
    "types": ["vite/client", "node"],
    "paths": {
      "@bindings/*": ["./bindings/github.com/<帳號>/synccraft/*"],
      "@events": ["./bindings/events/index.ts"]
    }
  }
}
```

```ts
// vite.config.ts
import { fileURLToPath, URL } from "node:url";
const r = (p: string) => fileURLToPath(new URL(p, import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      "@bindings": r("./bindings/github.com/<帳號>/synccraft"),
      "@events": r("./bindings/events"),
    },
  },
  // ...
});
```

之後在程式碼裡寫 `import { SchemaService } from "@bindings/internal/app"`、`import { CountTick } from "@events"`。

> 範本的 `frontend/.npmrc` 有 `minimum-release-age=10080`（不安裝發佈未滿 7 天的版本，降低被投毒的風險）。npm 目前不支援這個設定、會印警告，pnpm / bun 才會生效；保留即可。

## 1.4 第一次執行

```bash
wails3 dev
```

- 會啟動 Vite dev server（port 9245）、編譯 Go、開啟視窗。
- 改 `.tsx` → 熱更新；改 `.go` → 自動重新產生綁定、重新編譯、重開 App。
- WebView 內按右鍵 → Inspect，可以開 DevTools（dev 模式才有）。

正式建置：

```bash
wails3 build            # 產物在 bin/synccraft
wails3 package          # 依平台產生安裝檔（第 10 部）
```

## 1.5 測試用資料庫（docker compose）

在專案根目錄建 `dev/docker-compose.yml`，一次起兩套（來源 / 目標）：

```yaml
services:
  mysql-src:
    image: mysql:8.4
    environment: { MYSQL_ROOT_PASSWORD: root, MYSQL_DATABASE: shop }
    ports: ["3306:3306"]
  mysql-dst:
    image: mysql:8.4
    environment: { MYSQL_ROOT_PASSWORD: root, MYSQL_DATABASE: shop }
    ports: ["3307:3306"]
  pg-src:
    image: postgres:17
    environment: { POSTGRES_PASSWORD: postgres, POSTGRES_DB: shop }
    ports: ["5432:5432"]
  pg-dst:
    image: postgres:17
    environment: { POSTGRES_PASSWORD: postgres, POSTGRES_DB: shop }
    ports: ["5433:5432"]
  mssql:
    image: mcr.microsoft.com/mssql/server:2022-latest
    environment: { ACCEPT_EULA: "Y", MSSQL_SA_PASSWORD: "Passw0rd!" }
    ports: ["1433:1433"]
```

```bash
docker compose -f dev/docker-compose.yml up -d     # 或 podman compose（Fedora）
# 專案已附 dev/seed/：容器第一次啟動時會自動匯入下面的種子資料；
# 想重灌就 down -v 再 up
```

種子資料（MySQL，來源與目標故意不同）：

```sql
-- 來源 3306
CREATE TABLE users (
  id INT PRIMARY KEY AUTO_INCREMENT,
  email VARCHAR(255) NOT NULL,
  nickname VARCHAR(50) DEFAULT 'guest',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_email (email)
);
INSERT INTO users (email, nickname) VALUES ('a@x.com','A'),('b@x.com','B'),('c@x.com','C');

-- 目標 3307：少一欄、少索引、資料不同
CREATE TABLE users (
  id INT PRIMARY KEY AUTO_INCREMENT,
  email VARCHAR(100) NOT NULL
);
INSERT INTO users (id, email) VALUES (1,'a@x.com'),(2,'OLD@x.com'),(9,'z@x.com');
```

這組資料在第 5、6 部會產生：加欄位、改欄位型別、加索引；以及 INSERT（id=3）、UPDATE（id=2）、DELETE（id=9）。

## 練習

1. 安裝 GTK4 相依，`mise install` 裝好 `wails3`，跑 `wails3 doctor`。
2. `wails3 init -t react` 建立專案、升級前端套件、設定 `@bindings` 別名，`wails3 dev` 成功開窗。
3. 起 docker compose，灌入上面的種子資料。


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


# 第 3 部　Go 連線層：database/sql 與四種 Driver

## 3.1 database/sql 是什麼

Go 標準庫只定義 **介面**，實際連線由 driver 提供。driver 用「匿名 import」把自己註冊進去：

```go
import (
    "database/sql"
    _ "github.com/denisenkom/go-mssqldb" // 註冊 "sqlserver"
    _ "github.com/go-sql-driver/mysql"   // 註冊 "mysql"
    _ "github.com/lib/pq"                // 註冊 "postgres"
    _ "github.com/mattn/go-sqlite3"      // 註冊 "sqlite3"（需 CGO）
)

db, err := sql.Open("mysql", dsn)   // 第一個參數 = driver 名稱
```

核心 API：

| 方法 | 用途 |
|---|---|
| `sql.Open(driver, dsn)` | 建立 **連線池** 物件（不會真的連） |
| `db.PingContext(ctx)` | 真的連一次，驗證帳密 |
| `db.QueryContext / QueryRowContext` | 查詢 |
| `db.ExecContext` | INSERT/UPDATE/DELETE/DDL |
| `db.BeginTx` | 交易 |
| `rows.Scan(&a, &b)` | 把一列讀進變數 |
| `rows.Columns()` | 動態取得欄位名 |

## 3.2 原版的連線設定與 DSN

```go
type ConnectionConfig struct {
    Type     DBType `json:"type"`      // mysql | postgresql | sqlite | sqlserver
    Host     string `json:"host"`
    Port     int    `json:"port"`
    User     string `json:"user"`
    Password string `json:"password"`
    Database string `json:"database"`
    FilePath string `json:"filePath,omitempty"` // SQLite 專用
}
```

```go
func buildDSN(config ConnectionConfig) (string, string, error) {
    switch config.Type {
    case MySQL, "":
        dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
            config.User, config.Password, config.Host, config.Port, config.Database)
        return "mysql", dsn, nil
    case PostgreSQL:
        dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", ...)
        return "postgres", dsn, nil
    case SQLite:
        return "sqlite3", config.FilePath, nil
    case SQLServer:
        dsn := fmt.Sprintf("server=%s;port=%d;user id=%s;password=%s;database=%s", ...)
        return "sqlserver", dsn, nil
    }
}
```

### 各 DSN 格式解析

| DB | DSN 範例 | 重點參數 |
|---|---|---|
| MySQL | `root:pw@tcp(127.0.0.1:3306)/shop?parseTime=true&multiStatements=true` | `parseTime`：DATETIME → `time.Time`；`multiStatements`：一次 Exec 多句（原版靠它執行整批 SQL） |
| PostgreSQL | `host=127.0.0.1 port=5432 user=postgres password=pw dbname=shop sslmode=disable` | key=value 格式；`sslmode` 可為 disable / require / verify-full |
| SQLite | `/path/to/file.db` | 可加 `?_pragma=foreign_keys(1)` 等 |
| SQL Server | `server=127.0.0.1;port=1433;user id=sa;password=pw;database=shop` | 也支援 `sqlserver://sa:pw@host:1433?database=shop` URL 格式 |

### ⚠️ 原版 DSN 的問題

1. **字串拼接不跳脫**：密碼含 `@`、`/`、`;`、空白 → 解析錯誤。PG 的 key=value 格式密碼有空白要加單引號。
2. **PG 寫死 `sslmode=disable`**：雲端 DB（RDS、Supabase）多半強制 SSL。
3. **MySQL 開 `multiStatements`**：方便但擴大 SQL injection 影響面。

### SyncCraft 的做法：用 driver 提供的結構化 API

```go
import (
    "net/url"
    "github.com/go-sql-driver/mysql"
)

func mysqlDSN(c ConnectionConfig) string {
    cfg := mysql.NewConfig()
    cfg.User, cfg.Passwd = c.User, c.Password
    cfg.Net, cfg.Addr = "tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
    cfg.DBName = c.Database
    cfg.ParseTime = true
    cfg.Params = map[string]string{"charset": "utf8mb4"}
    return cfg.FormatDSN()            // 自動跳脫
}

func postgresDSN(c ConnectionConfig) string {
    u := url.URL{
        Scheme: "postgres",
        User:   url.UserPassword(c.User, c.Password), // 自動 percent-encode
        Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
        Path:   "/" + c.Database,
    }
    q := u.Query(); q.Set("sslmode", orDefault(c.SSLMode, "prefer")); u.RawQuery = q.Encode()
    return u.String()
}

func sqlserverDSN(c ConnectionConfig) string {
    u := url.URL{
        Scheme: "sqlserver",
        User:   url.UserPassword(c.User, c.Password),
        Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
    }
    q := u.Query(); q.Set("database", c.Database); u.RawQuery = q.Encode()
    return u.String()
}
```

> 註：`lib/pq` 已進入維護模式，官方建議新專案改用 `github.com/jackc/pgx/v5/stdlib`（driver 名 `"pgx"`）。`go-mssqldb` 新版移到 `github.com/microsoft/go-mssqldb`。SyncCraft 建議直接用新的。

## 3.3 Connect 與逾時

```go
func Connect(config ConnectionConfig) (*sql.DB, error) {
    driver, dsn, err := buildDSN(config)
    if err != nil { return nil, err }
    db, err := sql.Open(driver, dsn)
    if err != nil { return nil, err }

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := db.PingContext(ctx); err != nil {
        return nil, err            // ⚠️ 原版這裡沒 db.Close()，會漏 pool
    }
    return db, nil
}
```

原版**每個 API 都 `Connect` → `defer db.Close()`**。例如資料同步一張表就開關 4 次連線（`CompareTableData` 2 次 + `GetDataSyncSummary` 又 2 次）。

### SyncCraft：連線池管理器

```go
type Pool struct {
    mu  sync.Mutex
    dbs map[string]*sql.DB      // key = 連線設定的 hash
}

func (p *Pool) Get(ctx context.Context, c ConnectionConfig) (*sql.DB, error) {
    key := c.Key()                       // 例如 sha256(type|host|port|user|db|file)
    p.mu.Lock(); defer p.mu.Unlock()
    if db, ok := p.dbs[key]; ok { return db, nil }

    driver, dsn, err := buildDSN(c)
    if err != nil { return nil, err }
    db, err := sql.Open(driver, dsn)
    if err != nil { return nil, err }
    db.SetMaxOpenConns(8)
    db.SetMaxIdleConns(4)
    db.SetConnMaxIdleTime(5 * time.Minute)

    pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    if err := db.PingContext(pingCtx); err != nil { db.Close(); return nil, err }
    p.dbs[key] = db
    return db, nil
}

func (p *Pool) CloseAll() { /* 在 Service 的 ServiceShutdown() 裡呼叫（第 2.3 節） */ }
```

> SQLite 特別注意：同一個檔案多連線寫入會 `database is locked`，可對 SQLite `SetMaxOpenConns(1)`。

## 3.4 參數佔位符（Placeholder）

| DB | 佔位符 | 範例 |
|---|---|---|
| MySQL / SQLite | `?` | `WHERE TABLE_NAME = ?` |
| PostgreSQL | `$1, $2` | `WHERE table_name = $1` |
| SQL Server | `@p1, @p2`（或具名 `@name`） | `WHERE TABLE_NAME = @p1` |

**佔位符只能放「值」，不能放表名 / 欄位名**。所以表名只能拼字串 → 必須安全地加引號（第 4.7 節）。

## 3.5 動態 Scan：不知道欄位型別時

資料同步與表瀏覽事先不知道欄位，原版用 `[]interface{}` 技巧：

```go
cols, _ := rows.Columns()
values := make([]interface{}, len(cols))
ptrs   := make([]interface{}, len(cols))
for i := range values { ptrs[i] = &values[i] }

for rows.Next() {
    rows.Scan(ptrs...)
    for i, col := range cols {
        if b, ok := values[i].([]byte); ok { row[col] = string(b) } // MySQL 很多型別回 []byte
        else { row[col] = values[i] }
    }
}
```

各 driver 掃進 `interface{}` 的實際型別不同（MySQL 的 DECIMAL 是 `[]byte`、PG 的 NUMERIC 是 `[]byte`/string、SQL Server 的 uniqueidentifier 是 `[]byte` 16 bytes…）。這是跨庫比對時「值看起來一樣卻判定不同」的主要來源，第 6 部會處理正規化。

## 練習

1. 用 `mysql.Config.FormatDSN()` 產生密碼為 `p@ss/w;rd` 的 DSN 並連線成功。
2. 實作 `Pool`，寫一個 `TestConnection` 只 Ping 不快取。
3. 寫 `func QueryMaps(ctx, db, q string, args ...any) ([]map[string]any, error)` 通用工具，對四種 DB 各跑一次 `SELECT 1 AS x`。


# 第 4 部　SQL 大全：四種資料庫的 Metadata 查詢

比對工具的本質是：**問資料庫「你長什麼樣子」**。每種 DB 回答這個問題的方式不同。本部把原版用到的每一條 SQL 拆開，說明欄位意義、輸出範例、陷阱，以及 SyncCraft 的改良版。

> 名詞：**metadata / catalog** = 描述資料庫結構的資料。標準做法是 `INFORMATION_SCHEMA`（ANSI SQL），各家另有自己的系統表（`pg_catalog`、`sys.*`、`sqlite_master`）。

---

## 4.1 列出資料庫

### MySQL

```sql
SHOW DATABASES;
```

原版在 Go 端濾掉 `information_schema`、`mysql`、`performance_schema`、`sys`。等價的標準寫法：

```sql
SELECT SCHEMA_NAME
FROM INFORMATION_SCHEMA.SCHEMATA
WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys')
ORDER BY SCHEMA_NAME;
```

> MySQL 裡 **database = schema**，是同一件事。

### PostgreSQL

```sql
SELECT datname FROM pg_database
WHERE datistemplate = false AND datname NOT IN ('postgres');
```

- 必須先連到某個 DB 才能查（原版連 `postgres` 這個預設庫）。
- `datistemplate`：排除 `template0` / `template1`。
- ⚠️ PG 有 **database → schema → table** 三層。原版**只看 `public` schema**，其它 schema 的表完全看不到。SyncCraft 應再加一層「選 schema」：

```sql
SELECT nspname FROM pg_namespace
WHERE nspname NOT LIKE 'pg_%' AND nspname <> 'information_schema'
ORDER BY nspname;
```

### SQL Server

```sql
SELECT name FROM sys.databases
WHERE name NOT IN ('master','tempdb','model','msdb');
```

同樣是三層（database → schema(`dbo`) → table）。原版沒有處理 schema。

### SQLite

一個檔案 = 一個 DB，固定回傳 `["main"]`。

---

## 4.2 列出資料表

| DB | 原版 SQL |
|---|---|
| MySQL | `SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME` |
| PostgreSQL | `SELECT tablename FROM pg_tables WHERE schemaname = 'public'` |
| SQL Server | `SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE = 'BASE TABLE'` |
| SQLite | `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'` |

解析：

- `DATABASE()`：MySQL 目前連線所在的 DB（由 DSN 指定）。
- `TABLE_TYPE = 'BASE TABLE'`：排除 VIEW。
- `sqlite_%`：排除 `sqlite_sequence`（AUTOINCREMENT 計數表）等內部表。
- ⚠️ SQL Server 版沒過濾 schema，`dbo.users` 與 `audit.users` 會同名衝突；也沒 `ORDER BY`。

**SyncCraft 改良（帶 schema、排序一致）：**

```sql
-- PostgreSQL
SELECT table_name FROM information_schema.tables
WHERE table_schema = $1 AND table_type = 'BASE TABLE'
ORDER BY table_name;

-- SQL Server
SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = @p1 AND TABLE_TYPE = 'BASE TABLE'
ORDER BY TABLE_NAME;

-- SQLite（3.33+ 也可用 sqlite_schema）
SELECT name FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name;
```

---

## 4.3 取得 CREATE TABLE

### MySQL — 原生支援

```sql
SHOW CREATE TABLE `users`;
-- 回傳兩欄：Table | Create Table
```

輸出範例：

```sql
CREATE TABLE `users` (
  `id` int NOT NULL AUTO_INCREMENT,
  `email` varchar(255) NOT NULL,
  `nickname` varchar(50) DEFAULT 'guest',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_email` (`email`)
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
```

⚠️ `AUTO_INCREMENT=4` 會隨資料變動，若直接拿來比較或在目標建表，要用 regex 移除：`AUTO_INCREMENT=\d+\s*`。

### SQLite — 存在系統表

```sql
SELECT sql FROM sqlite_master WHERE type='table' AND name=?;
```

SQLite 存的就是當初你下的那句 CREATE 原文。

### PostgreSQL / SQL Server — 沒有，要自己組

原版用欄位清單拼出一個「簡易版」CREATE（**不含主鍵、索引、外鍵、約束**）：

```go
colDef := fmt.Sprintf("%s %s", col.Name, col.Type)
if col.Nullable == "NO" { colDef += " NOT NULL" }
if col.Default != nil  { colDef += " DEFAULT " + *col.Default }
info.CreateSQL = fmt.Sprintf("CREATE TABLE %s (\n  %s\n);", tableName, strings.Join(parts, ",\n  "))
```

⚠️ 因為 PG 原版拿的是 `data_type`（見 4.4），組出來會是 `email character varying NOT NULL` —— **長度不見了**。用它在目標建表會得到 `varchar` 無長度。SyncCraft 必須補上主鍵與完整型別（4.4、4.5 的改良查詢）。

---

## 4.4 欄位（Columns）— 最重要的查詢

### MySQL（原版）

```sql
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_DEFAULT, EXTRA, ORDINAL_POSITION
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
ORDER BY ORDINAL_POSITION;
```

| 欄位 | 意義 | 範例 |
|---|---|---|
| `COLUMN_TYPE` | **完整**型別（含長度、unsigned） | `varchar(255)`、`int unsigned` |
| `DATA_TYPE` | 只有型別名（原版沒用） | `varchar` |
| `IS_NULLABLE` | `YES` / `NO` | |
| `COLUMN_KEY` | `PRI` / `UNI` / `MUL` / 空 | |
| `COLUMN_DEFAULT` | 預設值（NULL 表示沒有預設） | `guest`、`CURRENT_TIMESTAMP` |
| `EXTRA` | 額外屬性 | `auto_increment`、`on update CURRENT_TIMESTAMP`、`DEFAULT_GENERATED` |
| `ORDINAL_POSITION` | 第幾欄（1 起算） | |

⚠️ **陷阱 1：`DEFAULT_GENERATED`**。MySQL 8.0.13+ 對 `DEFAULT CURRENT_TIMESTAMP` 這種運算式預設值，`EXTRA` 會顯示 `DEFAULT_GENERATED`。原版的 `buildColumnDef` 直接把 `EXTRA` 接在後面，產生：

```sql
ALTER TABLE `users` ADD COLUMN `created_at` datetime DEFAULT CURRENT_TIMESTAMP DEFAULT_GENERATED;
--                                                                            ^^^^^^^^^^^^^^^^^ 語法錯誤
```

SyncCraft 要把 `DEFAULT_GENERATED` 從 EXTRA 移除。

⚠️ **陷阱 2：MariaDB 差異**。MariaDB 10.2.7+ 的 `COLUMN_DEFAULT` 會帶引號（`'guest'`），沒有預設值時是字串 `NULL` 而非 SQL NULL。要支援 MariaDB 需判斷版本（`SELECT VERSION()` 含 `MariaDB`）。

⚠️ **陷阱 3：字元集/定序**。原版沒抓 `CHARACTER_SET_NAME`、`COLLATION_NAME`、`COLUMN_COMMENT`，這些不同也該算差異。

### PostgreSQL（原版）

```sql
SELECT column_name, data_type, is_nullable, column_default, ordinal_position
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1
ORDER BY ordinal_position;
```

⚠️ `data_type` 只給 `character varying`、`numeric`，沒有 `(255)`、`(10,2)`。

**SyncCraft 改良：直接查 `pg_catalog`，用 `format_type` 拿完整型別**

```sql
SELECT a.attname                                  AS name,
       format_type(a.atttypid, a.atttypmod)       AS type,       -- character varying(255)
       NOT a.attnotnull                           AS nullable,
       pg_get_expr(d.adbin, d.adrelid)            AS default_expr, -- nextval('users_id_seq'::regclass)
       a.attidentity                              AS identity,   -- 'a' / 'd' / ''
       a.attnum                                   AS position,
       col_description(a.attrelid, a.attnum)      AS comment
FROM pg_attribute a
LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE a.attrelid = to_regclass(format('%I.%I', $1::text, $2::text))  -- $1=schema, $2=table
  AND a.attnum > 0
  AND NOT a.attisdropped
ORDER BY a.attnum;
```

關鍵點：

- `pg_attribute`：每個欄位一列；`attnum > 0` 排除系統欄位（ctid 等），`attisdropped` 排除已刪除欄位。
- `format('%I.%I', schema, table)`：`%I` 會正確加雙引號，**大小寫混合的表名**（如 `"UserLog"`）才不會出錯。
- `serial` 欄位的預設值是 `nextval('users_id_seq'::regclass)`，跨 DB 比較時要特別處理。

### SQL Server（原版）

```sql
SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_DEFAULT, ORDINAL_POSITION
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_NAME = @p1
ORDER BY ORDINAL_POSITION;
```

⚠️ 同樣缺長度；`COLUMN_DEFAULT` 會帶外層括號，如 `((0))`、`(getdate())`。

**SyncCraft 改良：**

```sql
SELECT c.COLUMN_NAME,
       c.DATA_TYPE,
       c.CHARACTER_MAXIMUM_LENGTH,   -- -1 表示 MAX，例如 nvarchar(max)
       c.NUMERIC_PRECISION,
       c.NUMERIC_SCALE,
       c.IS_NULLABLE,
       c.COLUMN_DEFAULT,
       COLUMNPROPERTY(OBJECT_ID(QUOTENAME(c.TABLE_SCHEMA) + '.' + QUOTENAME(c.TABLE_NAME)),
                      c.COLUMN_NAME, 'IsIdentity') AS is_identity,
       c.ORDINAL_POSITION
FROM INFORMATION_SCHEMA.COLUMNS c
WHERE c.TABLE_SCHEMA = @p1 AND c.TABLE_NAME = @p2
ORDER BY c.ORDINAL_POSITION;
```

Go 端組型別：

```go
func mssqlType(dataType string, maxLen, prec, scale sql.NullInt64) string {
    switch dataType {
    case "varchar", "nvarchar", "char", "nchar", "varbinary", "binary":
        if maxLen.Int64 == -1 { return dataType + "(max)" }
        return fmt.Sprintf("%s(%d)", dataType, maxLen.Int64)
    case "decimal", "numeric":
        return fmt.Sprintf("%s(%d,%d)", dataType, prec.Int64, scale.Int64)
    }
    return dataType
}
```

### SQLite（原版）

```sql
PRAGMA table_info('users');
```

回傳：`cid | name | type | notnull | dflt_value | pk`

| 欄位 | 意義 |
|---|---|
| `cid` | 0 起算的欄位序號 |
| `notnull` | 1 = NOT NULL |
| `dflt_value` | 預設值原文（字串會含引號 `'guest'`） |
| `pk` | 0 = 非主鍵；**複合主鍵時為 1, 2, 3… 代表在 PK 中的順序** |

⚠️ 原版 `getSQLiteTableInfo` 只在 `pk == 1` 時標 `PRI`，複合主鍵的第二欄會漏掉（但 `getPrimaryKeys` 用 `pk > 0` 是對的——同一份程式兩種寫法）。

⚠️ `PRAGMA table_info('%s')` 用字串拼接，表名含 `'` 會壞。**SyncCraft 改用 table-valued function，可以用佔位符**（SQLite 3.16+）：

```sql
SELECT cid, name, type, "notnull", dflt_value, pk
FROM pragma_table_info(?)
ORDER BY cid;
```

---

## 4.5 主鍵（Primary Key）

資料同步靠主鍵對齊每一列，這條查詢決定整個同步功能能不能用。

### MySQL

```sql
SELECT COLUMN_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
  AND CONSTRAINT_NAME = 'PRIMARY'
ORDER BY ORDINAL_POSITION;
```

MySQL 的主鍵約束名**永遠叫 `PRIMARY`**，所以可以這樣過濾。

### PostgreSQL

```sql
SELECT a.attname
FROM pg_index i
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE i.indrelid = $1::regclass AND i.indisprimary
ORDER BY array_position(i.indkey, a.attnum);
```

逐段解析：

- `pg_index`：每個索引一列；`indisprimary` = 是否為主鍵索引。
- `indkey`：`int2vector`，主鍵包含的欄位編號陣列，如 `{1,3}`。
- `a.attnum = ANY(i.indkey)`：把欄位編號對回欄位名。
- `array_position(...)`：依主鍵內順序排序（複合主鍵很重要）。
- `$1::regclass`：把字串轉成表的 OID。⚠️ 傳 `UserLog` 會被當成 `userlog`；傳不存在的表會直接報錯。

⚠️ 註：`indkey` 是 `int2vector`，某些 PG 版本 `array_position` 需轉型：`array_position(i.indkey::int2[], a.attnum)`。

**SyncCraft 版：**

```sql
SELECT a.attname
FROM pg_index i
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE i.indrelid = to_regclass(format('%I.%I', $1::text, $2::text))
  AND i.indisprimary
ORDER BY array_position(i.indkey::int2[], a.attnum);
```

### SQL Server

```sql
SELECT c.COLUMN_NAME
FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE c ON tc.CONSTRAINT_NAME = c.CONSTRAINT_NAME
WHERE tc.TABLE_NAME = @p1 AND tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
ORDER BY c.ORDINAL_POSITION;
```

- `TABLE_CONSTRAINTS`：找出這張表 `PRIMARY KEY` 類型的約束名（如 `PK__users__3213E83F...`）。
- `KEY_COLUMN_USAGE`：由約束名列出欄位。
- SyncCraft：JOIN 條件補上 `tc.TABLE_SCHEMA = c.TABLE_SCHEMA`，WHERE 加 `tc.TABLE_SCHEMA = @p2`。

### SQLite

```sql
SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk;
```

⚠️ 沒有宣告主鍵的 SQLite 表仍有隱藏的 `rowid`，可作為退路（但不同檔案間 rowid 不保證對應同一筆資料）。

---

## 4.6 索引（Indexes）

### MySQL

```sql
SHOW INDEX FROM `users`;
```

回傳 15 欄左右，原版只取 `Key_name`、`Non_unique`、`Column_name`、`Seq_in_index`。因為欄位數會隨版本變，原版用 `rows.Columns()` + 動態 Scan 再依欄名挑——這是處理 `SHOW` 指令的好技巧。

等價的標準查詢（推薦，可用佔位符）：

```sql
SELECT INDEX_NAME, NON_UNIQUE, COLUMN_NAME, SEQ_IN_INDEX, INDEX_TYPE, SUB_PART
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
ORDER BY INDEX_NAME, SEQ_IN_INDEX;
```

`SUB_PART` = 前綴索引長度（如 `email(20)`），原版沒抓，比對會漏。

### PostgreSQL

```sql
SELECT indexname, indexdef
FROM pg_indexes
WHERE schemaname = 'public' AND tablename = $1;
```

`indexdef` 是完整的建立語句：

```
CREATE UNIQUE INDEX users_pkey ON public.users USING btree (id)
```

原版把整串 `indexdef` 塞進 `Column` 欄位（偷懶但有效：比較 indexdef 字串即可判斷是否相同）。缺點是比對時再經 `buildIndexMap` 包上反引號，產生的 `ALTER TABLE ... ADD INDEX` 是 MySQL 語法，對 PG 無效。

SyncCraft 做法：PG 的「新增索引」直接執行 `indexdef`（它本身就是合法的 CREATE INDEX），「刪除」用 `DROP INDEX schema.name`；並排除 `indisprimary` 的索引（由主鍵約束管理）。

### SQL Server

```sql
SELECT i.name, c.name AS column_name, i.is_unique
FROM sys.indexes i
JOIN sys.index_columns ic ON i.object_id = ic.object_id AND i.index_id = ic.index_id
JOIN sys.columns c        ON ic.object_id = c.object_id AND ic.column_id = c.column_id
WHERE i.object_id = OBJECT_ID(@p1) AND i.name IS NOT NULL;
```

- `sys.indexes`：`index_id = 0` 是 heap（無叢集索引），`name IS NULL` 濾掉它。
- `sys.index_columns`：索引包含哪些欄位；`key_ordinal` 是順序，`is_included_column` 是 INCLUDE 欄位。
- ⚠️ 原版沒有 `ORDER BY ic.key_ordinal`，複合索引欄位順序不保證 → 可能誤判「索引改變」。
- SyncCraft 補：`AND ic.is_included_column = 0 ORDER BY i.name, ic.key_ordinal`，並取 `i.is_primary_key` 以排除主鍵。

### SQLite

```sql
PRAGMA index_list('users');   -- seq | name | unique | origin | partial
PRAGMA index_info('idx_email'); -- 原版沒用：列出索引欄位
```

`origin`：`c` = CREATE INDEX 建的、`u` = UNIQUE 約束、`pk` = 主鍵。原版只拿名稱與 unique，**沒拿欄位**，所以索引欄位變了偵測不到。

SyncCraft 一次取完：

```sql
SELECT il.name, il."unique", il.origin, ii.seqno, ii.name AS column_name
FROM pragma_index_list(?) AS il
JOIN pragma_index_info(il.name) AS ii
WHERE il.origin <> 'pk'
ORDER BY il.name, ii.seqno;
```

---

## 4.7 識別字加引號（Quote Identifier）

表名、欄位名不能用佔位符，只能拼進 SQL，所以一定要加引號：

| DB | 引號 | 內含引號的跳脫 |
|---|---|---|
| MySQL | `` `name` `` | `` ` `` → ``` `` ``` |
| PostgreSQL / SQLite | `"name"` | `"` → `""` |
| SQL Server | `[name]` | `]` → `]]` |

原版 `quoteIdentifier` **沒做跳脫**：

```go
case MySQL: return fmt.Sprintf("`%s`", name)   // 表名 a`b → `a`b`  壞掉
```

SyncCraft：

```go
func (MySQL) Quote(id string) string     { return "`" + strings.ReplaceAll(id, "`", "``") + "`" }
func (Postgres) Quote(id string) string  { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }
func (SQLServer) Quote(id string) string { return "[" + strings.ReplaceAll(id, "]", "]]") + "]" }
```

PG 也可以在 SQL 內用 `quote_ident()` / `format('%I')`；SQL Server 用 `QUOTENAME()`。

---

## 4.8 筆數

原版：

```sql
SELECT COUNT(*) FROM `users`;
```

正確但在大表很慢（InnoDB、PG 都要掃描）。「列出所有表」畫面建議用估計值：

```sql
-- MySQL（InnoDB 為估計值）
SELECT TABLE_NAME, TABLE_ROWS FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE();

-- PostgreSQL（ANALYZE 後的估計）
SELECT c.relname, c.reltuples::bigint
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relkind = 'r';

-- SQL Server（精確且快）
SELECT t.name, SUM(p.rows)
FROM sys.tables t JOIN sys.partitions p ON p.object_id = t.object_id AND p.index_id IN (0,1)
WHERE SCHEMA_NAME(t.schema_id) = @p1
GROUP BY t.name;
```

真正比對前再對選中的表做精確 `COUNT(*)`。

---

## 4.9 建立 / 刪除資料庫

| DB | SQL | 備註 |
|---|---|---|
| MySQL | ``CREATE DATABASE `shop` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`` | 連線時不指定 DB |
| PostgreSQL | `CREATE DATABASE shop` | 須連到 `postgres`；**不能在交易中執行** |
| SQL Server | `CREATE DATABASE [shop]` | 連到 `master` |
| SQLite | — | 開一個新檔案路徑即可 |

⚠️ 原版 PG 版 `CREATE DATABASE %s` 沒加引號，名稱含大寫或 `-` 會出錯／被轉小寫。

---

## 4.10 總對照表（背起來）

| 需求 | MySQL | PostgreSQL | SQL Server | SQLite |
|---|---|---|---|---|
| 列 DB | `SHOW DATABASES` | `pg_database` | `sys.databases` | 固定 `main` |
| 列 schema | （= DB） | `pg_namespace` | `sys.schemas` | — |
| 列表 | `I_S.TABLES` | `pg_tables` / `I_S.tables` | `I_S.TABLES` | `sqlite_master` |
| CREATE 原文 | `SHOW CREATE TABLE` | 自行組（或 `pg_dump -s`） | 自行組 | `sqlite_master.sql` |
| 欄位 | `I_S.COLUMNS`（`COLUMN_TYPE`） | `pg_attribute` + `format_type` | `I_S.COLUMNS` + 長度欄 | `pragma_table_info` |
| 主鍵 | `KEY_COLUMN_USAGE` + `'PRIMARY'` | `pg_index.indisprimary` | `TABLE_CONSTRAINTS` | `pk > 0` |
| 索引 | `I_S.STATISTICS` | `pg_indexes` / `pg_index` | `sys.indexes` + `index_columns` | `pragma_index_list` + `index_info` |
| 佔位符 | `?` | `$1` | `@p1` | `?` |
| 識別字引號 | `` ` `` | `"` | `[ ]` | `"` |
| 分頁 | `LIMIT n OFFSET m` | `LIMIT n OFFSET m` | `OFFSET m ROWS FETCH NEXT n ROWS ONLY` | `LIMIT n OFFSET m` |
| 自動遞增 | `AUTO_INCREMENT` | `GENERATED ... AS IDENTITY` / `serial` | `IDENTITY(1,1)` | `INTEGER PRIMARY KEY` |

## 練習

1. 在 docker 的四種 DB 各建同一張 `users` 表，逐條跑本部所有查詢並記錄輸出差異。
2. 在 MySQL 8 建 `created_at DATETIME DEFAULT CURRENT_TIMESTAMP`，親眼看到 `EXTRA = DEFAULT_GENERATED`。
3. 在 PG 建 `CREATE TABLE "UserLog"(id int primary key)`，分別用 `$1::regclass` 與 `to_regclass(format('%I.%I', ...))` 查主鍵，觀察差異。
4. 在 SQLite 建複合主鍵表 `PRIMARY KEY (a, b)`，看 `pragma_table_info` 的 `pk` 欄。


# 第 5 部　Schema 比對與 DDL 生成

## 5.1 流程

```
GetSchema(source) ─┐
                   ├─► CompareSchemas(src, dst) ─► []DiffResult{type, tableName, detail, sql}
GetSchema(target) ─┘                                         │
                                                             ▼
                                         前端勾選 ─► ExecuteSQL(target, sql)
```

資料模型：

```go
type SchemaInfo struct {
    Database string
    Tables   map[string]TableInfo      // key = 表名
}
type TableInfo struct {
    Name, CreateSQL string
    Columns []ColumnInfo
    Indexes []IndexInfo
}
type DiffResult struct {
    Type      string // "added" | "removed" | "modified"
    TableName string
    Detail    string // "Add column: nickname"
    SQL       string
}
```

> 語意：**source 是「標準答案」，target 要被改成跟 source 一樣**。

## 5.2 表層級比對（集合差）

```go
// source 有、target 沒有 → 在 target 建表
for name, t := range source.Tables {
    if _, ok := target.Tables[name]; !ok {
        results = append(results, DiffResult{"added", name, "...", t.CreateSQL + ";"})
    }
}
// target 有、source 沒有 → 在 target 刪表
for name := range target.Tables {
    if _, ok := source.Tables[name]; !ok {
        results = append(results, DiffResult{"removed", name, "...", fmt.Sprintf("DROP TABLE `%s`;", name)})
    }
}
// 兩邊都有 → 逐欄比較
for name, s := range source.Tables {
    if t, ok := target.Tables[name]; ok {
        results = append(results, compareTableStructure(name, s, t)...)
    }
}
```

最後依 `added → modified → removed` 排序（先建、再改、最後刪，執行順序也比較安全）。

⚠️ MySQL 的 `SHOW CREATE TABLE` 已經以 `)` 結尾不含分號，原版加 `";"` 沒問題；但 SQLite 的 `sqlite_master.sql` 也沒分號、PG/MSSQL 自組版**已經有 `;`** → 變成 `;;`。

## 5.3 欄位層級比對

```go
srcCols := map[name]ColumnInfo{...}
dstCols := map[name]ColumnInfo{...}
```

| 情況 | 產生的 SQL（原版） |
|---|---|
| src 有 dst 無 | ``ALTER TABLE `t` ADD COLUMN `c` <def> AFTER `prev`;``（第一欄則 `FIRST`） |
| dst 有 src 無 | ``ALTER TABLE `t` DROP COLUMN `c`;`` |
| 兩邊都有但不同 | ``ALTER TABLE `t` MODIFY COLUMN `c` <def>;`` |

「不同」的定義：

```go
func columnsEqual(a, b ColumnInfo) bool {
    return a.Type == b.Type && a.Nullable == b.Nullable &&
           a.Extra == b.Extra && defaultsEqual(a.Default, b.Default)
}
```

注意：**欄位順序（Position）不參與比較**，只在 ADD 時用來產生 `AFTER`。

### 欄位定義產生器

```go
func buildColumnDef(col ColumnInfo) string {
    def := col.Type                                  // varchar(50)
    if col.Nullable == "NO" { def += " NOT NULL" }
    if col.Default != nil {
        v := *col.Default
        if isNumericDefault(v) || isSpecialDefault(v) {
            def += " DEFAULT " + v                   // DEFAULT 0 / DEFAULT CURRENT_TIMESTAMP
        } else {
            def += " DEFAULT '" + v + "'"            // DEFAULT 'guest'   ⚠️ 未跳脫單引號
        }
    }
    if col.Extra != "" { def += " " + col.Extra }    // auto_increment   ⚠️ DEFAULT_GENERATED
    return def
}
```

用第 1 部的種子資料，預期輸出：

```sql
ALTER TABLE `users` MODIFY COLUMN `email` varchar(255) NOT NULL;
ALTER TABLE `users` ADD COLUMN `nickname` varchar(50) DEFAULT 'guest' AFTER `email`;
ALTER TABLE `users` ADD COLUMN `created_at` datetime DEFAULT CURRENT_TIMESTAMP DEFAULT_GENERATED AFTER `nickname`;  -- ✗
ALTER TABLE `users` ADD INDEX `idx_email` (`email`);
```

### ⚠️ ADD 的 AFTER 子句陷阱

`AFTER prev` 的 `prev` 是 source 上一欄；若 `prev` 本身也是本次才要新增的欄位，且執行順序在後面（map 迭代順序是隨機的！），就會 `Unknown column`。SyncCraft：**依 Position 排序後再產生 ADD**。

## 5.4 索引比對

```go
func buildIndexMap(idx []IndexInfo) map[string][]string  // 索引名 → [`col1`, `col2`]
```

- 跳過 `PRIMARY`。
- src 有 dst 無 → `ADD INDEX`；欄位不同 → `DROP INDEX x, ADD INDEX x (...)`；dst 有 src 無 → `DROP INDEX`。
- ⚠️ 不分 UNIQUE：source 的 `UNIQUE KEY` 會被重建成一般 `INDEX`。
- ⚠️ 不處理外鍵、CHECK、觸發器、VIEW。

## 5.5 原版最大的問題：DDL 方言寫死成 MySQL

| 原版產生 | PostgreSQL 正確寫法 | SQL Server 正確寫法 | SQLite |
|---|---|---|---|
| `` `t` `` | `"t"` | `[t]` | `"t"` |
| `MODIFY COLUMN c type` | `ALTER COLUMN c TYPE type` + 分開的 `SET/DROP NOT NULL`、`SET DEFAULT` | `ALTER COLUMN c type [NOT] NULL`（預設值是獨立約束） | **不支援**，需重建表 |
| `ADD COLUMN ... AFTER x` | 無 AFTER（只能加在最後） | 無 AFTER | 無 AFTER |
| `ADD INDEX i (c)` | `CREATE INDEX i ON t (c)` | `CREATE INDEX i ON t (c)` | `CREATE INDEX i ON t (c)` |
| `DROP INDEX i`（ALTER 內） | `DROP INDEX i` | `DROP INDEX i ON t` | `DROP INDEX i` |

另外原版是用 **source 的型別** 產生 DDL 在 **target** 執行；若 source 是 MySQL、target 是 PG（跨庫），`int unsigned`、`datetime`、`tinyint(1)` 在 PG 都不存在。

## 5.6 SyncCraft 重構：Dialect 介面

把「怎麼問結構」與「怎麼產生 DDL」依資料庫抽成介面：

```go
package dialect

type Dialect interface {
    Name() string
    Quote(ident string) string
    Placeholder(n int) string                       // ?, $1, @p1

    // Introspection（第 4 部的查詢）
    ListTables(ctx context.Context, db *sql.DB, schema string) ([]string, error)
    Columns(ctx context.Context, db *sql.DB, schema, table string) ([]model.Column, error)
    PrimaryKey(ctx context.Context, db *sql.DB, schema, table string) ([]string, error)
    Indexes(ctx context.Context, db *sql.DB, schema, table string) ([]model.Index, error)

    // DDL generation
    CreateTable(t model.Table) []string
    DropTable(table string) string
    AddColumn(table string, c model.Column, after *model.Column) string
    DropColumn(table, col string) string
    AlterColumn(table string, from, to model.Column) []string   // PG 可能要多句
    CreateIndex(table string, idx model.Index) string
    DropIndex(table string, idx model.Index) string
}

func For(t DBType) Dialect { /* switch */ }
```

PostgreSQL 的 AlterColumn 範例：

```go
func (p Postgres) AlterColumn(table string, from, to model.Column) []string {
    t, c := p.Quote(table), p.Quote(to.Name)
    var out []string
    if from.Type != to.Type {
        out = append(out, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s", t, c, to.Type, c, to.Type))
    }
    if from.Nullable != to.Nullable {
        if to.Nullable { out = append(out, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", t, c)) } else {
            out = append(out, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", t, c)) }
    }
    if !ptrEq(from.Default, to.Default) {
        if to.Default == nil { out = append(out, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT", t, c)) } else {
            out = append(out, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s", t, c, *to.Default)) }
    }
    return out
}
```

SQLite 的 AlterColumn：官方建議的 12 步「建新表 → 複製資料 → 刪舊表 → 改名」：

```sql
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE "users__new" ( ...source 定義... );
INSERT INTO "users__new" (id, email) SELECT id, email FROM "users";
DROP TABLE "users";
ALTER TABLE "users__new" RENAME TO "users";
-- 重建索引
COMMIT;
PRAGMA foreign_keys = ON;
```

### 比對邏輯變成純函式

```go
package schema

type Change struct {
    Kind   string   // add_table / drop_table / add_column / drop_column / alter_column / add_index / drop_index
    Table  string
    Detail string
    SQL    []string // 由 target 的 Dialect 產生
    Risk   string   // safe / destructive（DROP 類標紅）
}

func Diff(src, dst model.Schema, d dialect.Dialect) []Change
```

**用 target 的 dialect 產生 SQL**（因為 SQL 是在 target 執行）。純函式不碰資料庫 → 非常好寫單元測試：

```go
func TestDiff_AddColumn(t *testing.T) {
    src := model.Schema{Tables: map[string]model.Table{"users": {Columns: []model.Column{{Name: "id", Type: "int"}, {Name: "nick", Type: "varchar(50)"}}}}}
    dst := model.Schema{Tables: map[string]model.Table{"users": {Columns: []model.Column{{Name: "id", Type: "int"}}}}}
    got := schema.Diff(src, dst, dialect.MySQL{})
    want := "ALTER TABLE `users` ADD COLUMN `nick` varchar(50) AFTER `id`"
    if got[0].SQL[0] != want { t.Fatalf("got %q", got[0].SQL[0]) }
}
```

### 正規化：避免假差異

比較前把雙方欄位「正規化」：

| 現象 | 正規化 |
|---|---|
| MySQL 8 `int(11)` vs `int`（顯示寬度已棄用） | 去掉整數型別的 `(n)` |
| MSSQL 預設值 `((0))` | 去外層括號 → `0` |
| PG 預設 `'guest'::character varying` | 去掉 `::type` 轉型 |
| EXTRA `DEFAULT_GENERATED` | 移除 |
| 大小寫 `VARCHAR` vs `varchar` | 轉小寫 |

## 5.7 執行 SQL

原版 `ExecuteSQL`：MySQL 靠 `multiStatements=true` 一次執行整串；其他 DB 用自寫的 `splitSQLStatements` 依 `;` 切（會略過字串內的分號）。

⚠️ 切割器不處理註解（`-- ;`）、PG 的 `$$ ... $$`、MSSQL 的 `GO`。SyncCraft 既然自己產生 SQL，**就讓 Change.SQL 本身是 `[]string`**，不需要再切。

交易：

```go
tx, err := db.BeginTx(ctx, nil)
for _, s := range stmts { if _, err := tx.ExecContext(ctx, s); err != nil { tx.Rollback(); return err } }
return tx.Commit()
```

> ⚠️ **MySQL 的 DDL 會隱式 commit，無法回滾**；PG 與 SQL Server 的 DDL 可以在交易中回滾；SQLite 可以。UI 上要明確告知使用者。

## 練習

1. 用種子資料跑原版，確認 `DEFAULT_GENERATED` bug 重現。
2. 實作 `dialect.MySQL` 的 `AddColumn`、`AlterColumn`、`CreateIndex`，並寫 table-driven 測試。
3. 實作 `normalizeColumn()`，讓 `int(11)` 與 `int` 判定相同。
4. 進階：實作 `dialect.Postgres`，在 pg-src / pg-dst 上跑通完整 Schema 同步。


# 第 6 部　資料同步：主鍵對齊與 DML 生成

## 6.1 演算法（原版）

```
1. 取 source 的主鍵欄位 PK（沒有主鍵 → 報錯，無法同步）
2. 取 source 的欄位清單 C
3. SELECT C FROM table   ← source、target 各跑一次，整表讀進記憶體
4. 每列組 key = PK 值用 "|" 串接，例如 "42" 或 "2024|A001"
   sourceData: map[key]row, targetData: map[key]row
5. 對 sourceData 每個 key：
     target 有且內容不同 → UPDATE
     target 沒有         → INSERT
6. 對 targetData 每個 key：source 沒有 → DELETE
```

這就是經典的 **hash join 差集**，時間 O(n+m)，但記憶體也是 O(n+m)。

## 6.2 程式拆解

### 讀資料

```go
query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(quotedCols, ", "), quoteIdentifier(dbType, tableName))
rows, _ := db.Query(query)
for rows.Next() {
    rows.Scan(ptrs...)
    row := map[string]interface{}{}
    for i, col := range columns {
        if b, ok := values[i].([]byte); ok { row[col] = string(b) } else { row[col] = values[i] }
    }
    var pkParts []string
    for _, pk := range primaryKeys { pkParts = append(pkParts, fmt.Sprintf("%v", row[pk])) }
    data[strings.Join(pkParts, "|")] = row
}
```

⚠️ 注意 **target 也是用 source 的欄位清單去 SELECT**：如果 target 少一欄，查詢直接失敗。所以資料同步前要先做 Schema 同步——UI 應該提示。

⚠️ key 用 `"|"` 串接：主鍵值本身含 `|` 時，`("a|b","c")` 與 `("a","b|c")` 會撞 key。SyncCraft 改用長度前綴或 `\x00` 分隔。

### 比較兩列

```go
func rowsEqual(a, b map[string]interface{}) bool {
    for k, v := range a {
        if fmt.Sprintf("%v", v) != fmt.Sprintf("%v", b[k]) { return false }
    }
    return true
}
```

用字串表示比較，同庫同型別時夠用。跨庫或不同 driver 時會出現假差異：

| 值 | MySQL driver | PG driver | `%v` 結果 |
|---|---|---|---|
| DECIMAL 10.50 | `"10.50"` | `"10.50"` | 相同 ✓ |
| DATETIME | `time.Time`（parseTime=true） | `time.Time` | 時區不同時字串不同 ✗ |
| BOOLEAN | `int64(1)` | `true` | `1` vs `true` ✗ |
| FLOAT 0.1 | `float32` / `float64` | `float64` | 精度不同 ✗ |

### 產生 SQL

```go
// INSERT：依欄位順序
INSERT INTO `users` (`id`, `email`, `nickname`) VALUES (3, 'c@x.com', 'C');

// UPDATE：非主鍵欄位全部 SET，主鍵當 WHERE
UPDATE `users` SET `email` = 'b@x.com', `nickname` = 'B' WHERE `id` = 2;

// DELETE
DELETE FROM `users` WHERE `id` = 9;
```

值的跳脫：

```go
func escapeValue(val interface{}) string {
    if val == nil { return "NULL" }
    switch v := val.(type) {
    case int, int32, int64, float32, float64: return fmt.Sprintf("%v", v)
    case bool: if v { return "1" }; return "0"
    default:
        s := strings.ReplaceAll(fmt.Sprintf("%v", v), "'", "''")
        return "'" + s + "'"
    }
}
```

⚠️ 問題清單：

1. **`time.Time` 走 default** → `'2024-01-02 03:04:05 +0000 UTC'`，MySQL 不接受這個格式。
2. **MySQL 預設 `NO_BACKSLASH_ESCAPES` 關閉**：字串 `a\'` 經 `''` 跳脫後仍可能被反斜線破壞 → 潛在注入。
3. **二進位欄位**（BLOB）被轉成 Go string 再加引號，非 UTF-8 位元組會壞掉。
4. UPDATE 的 SET 用 `for col := range row`（map 迭代）→ 每次產生的欄位順序不同，不影響正確性但難以 diff/測試。
5. PG 的 `bool` 寫成 `1/0` 會失敗（PG 要 `true/false`）。
6. SQL Server 的 IDENTITY 欄位 INSERT 需要 `SET IDENTITY_INSERT [t] ON`。

## 6.3 SyncCraft 改良一：顯示用 SQL 與執行用 SQL 分離

**給人看的** SQL 可以是內插好的字串；**真正執行**時用參數化查詢，徹底避開跳脫問題：

```go
type RowChange struct {
    Kind    string         `json:"kind"`      // insert | update | delete
    Table   string         `json:"table"`
    PK      map[string]any `json:"pk"`
    Old     map[string]any `json:"old,omitempty"`
    New     map[string]any `json:"new,omitempty"`
    Changed []string       `json:"changed,omitempty"` // UPDATE 時哪些欄位不同（前端高亮用）
    Preview string         `json:"preview"`           // 給人看
}

// 執行時：
func (d MySQL) InsertStmt(table string, cols []string) string {
    ph := strings.Repeat("?, ", len(cols)); ph = ph[:len(ph)-2]
    return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", d.Quote(table), d.QuoteList(cols), ph)
}
stmt, _ := tx.PrepareContext(ctx, d.InsertStmt(table, cols))
for _, ch := range inserts { stmt.ExecContext(ctx, valuesInOrder(ch.New, cols)...) }
```

好處：型別交給 driver 處理（time、bool、[]byte 都正確），且 Prepare 一次、執行 N 次更快。

## 6.4 SyncCraft 改良二：大表用「排序合併」串流比對

不把整表放進記憶體。兩邊都 **依主鍵排序** 後同時往下讀，像合併排序的 merge 步驟：

```sql
SELECT cols FROM t ORDER BY pk1, pk2;
```

```go
s, t := srcRows.Next(), dstRows.Next()
for s != nil || t != nil {
    switch cmp := comparePK(s, t); {
    case t == nil || (s != nil && cmp < 0): emit(Insert(s)); s = srcRows.Next()
    case s == nil || cmp > 0:               emit(Delete(t)); t = dstRows.Next()
    default:
        if !equalRow(s, t) { emit(Update(s, t)) }
        s, t = srcRows.Next(), dstRows.Next()
    }
}
```

- 記憶體 O(1)（每邊只拿一列）。
- ⚠️ **兩邊排序規則必須一致**：字串主鍵在 MySQL（`utf8mb4_0900_ai_ci` 不分大小寫）與 PG（依 locale）排序可能不同 → 跨庫時字串主鍵仍建議用 hash 方式，或在 Go 端排序。整數主鍵最安全。
- 差異量很大時，`emit` 寫到暫存檔而非全部回傳前端。

### 進一步：分塊 checksum（選做）

先依主鍵範圍分塊，兩邊算 checksum，相同的塊直接跳過：

```sql
-- MySQL
SELECT COUNT(*), BIT_XOR(CRC32(CONCAT_WS('#', id, email, nickname)))
FROM users WHERE id BETWEEN ? AND ?;

-- PostgreSQL
SELECT COUNT(*), md5(string_agg(md5(row(id, email, nickname)::text), '' ORDER BY id))
FROM users WHERE id BETWEEN $1 AND $2;
```

這是 pt-table-checksum 的原理；只適用同種 DB（跨庫函式結果不同）。

## 6.5 SyncCraft 改良三：值正規化

比較前把雙方的值轉成「標準型」：

```go
func normalize(v any) any {
    switch x := v.(type) {
    case nil:        return nil
    case []byte:     if utf8.Valid(x) { return string(x) }; return hex.EncodeToString(x)
    case time.Time:  return x.UTC().Format(time.RFC3339Nano)
    case bool:       if x { return int64(1) }; return int64(0)
    case float32:    return strconv.FormatFloat(float64(x), 'g', -1, 32)
    case float64:    return strconv.FormatFloat(x, 'g', -1, 64)
    case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
                     return fmt.Sprint(x)
    default:         return fmt.Sprint(x)
    }
}
```

回傳前端前也要轉：`[]byte`、`int64 > 2^53`、`time.Time` 都轉成字串，避免 JSON 失真。

## 6.6 執行順序與交易

```
BEGIN
  1. DELETE（先刪，避免 UNIQUE 衝突）
  2. UPDATE
  3. INSERT
COMMIT（任一失敗 → ROLLBACK）
```

外鍵：多張表一起同步時，INSERT 依「父 → 子」，DELETE 依「子 → 父」。或暫時關閉檢查：

| DB | 關閉外鍵檢查 |
|---|---|
| MySQL | `SET FOREIGN_KEY_CHECKS = 0` |
| PostgreSQL | `SET session_replication_role = replica`（需 superuser）或 `ALTER TABLE ... DISABLE TRIGGER ALL` |
| SQL Server | `ALTER TABLE t NOCHECK CONSTRAINT ALL` |
| SQLite | `PRAGMA foreign_keys = OFF`（必須在交易外） |

批次大小：每 500～1000 列 commit 一次，或使用多列 INSERT：`INSERT ... VALUES (...), (...), (...)`。

## 6.7 前端互動（原版功能，React 版照做）

- 表清單 Shift+點擊範圍選取。
- 比對時逐表呼叫 `CompareTableData`，顯示日誌。
- 結果分 INSERT / UPDATE / DELETE 三個勾選類別，可展開看舊值 → 新值。
- 「預覽 SQL」後再「執行」。

SyncCraft 改為呼叫一次 `CompareTables(src, dst, tables[])`，Go 端用 `app.Event.Emit` 回報每張表的進度（第 2.6 節），前端可隨時 `.cancel()` 中止（第 2.5 節）。

## 練習

1. 用種子資料，手算出 INSERT/UPDATE/DELETE 各是哪幾列，再跑原版驗證。
2. 寫 `escapeValue` 的失敗測試：`time.Time`、含反斜線字串、`[]byte{0xff}`。
3. 實作參數化的 `ApplyChanges(ctx, db, dialect, changes)`，含交易與 DELETE→UPDATE→INSERT 順序。
4. 進階：用整數主鍵實作排序合併比對，對 100 萬列表測試記憶體用量（`runtime.ReadMemStats`）。


# 第 7 部　資料表瀏覽與連線儲存

## 7.1 表瀏覽的三個 API

| Go 方法 | 用途 | 內部 |
|---|---|---|
| `GetAllTables(cfg)` | 左側表清單＋筆數 | 直接呼叫 `GetTablesForSync`（每表 PK、欄位、`COUNT(*)`） |
| `GetTableStructure(cfg, table)` | 結構分頁（欄位、索引、CREATE） | 依 DB 呼叫 `getXxxTableInfo`（第 4 部） |
| `GetTableData(cfg, table, page, pageSize)` | 資料分頁 | 見下 |

⚠️ `GetAllTables` 對每張表都跑 3 條查詢（PK、欄位、COUNT），200 張表就是 600 次往返。SyncCraft：清單只要表名＋估計筆數（第 4.8 節的單一查詢），點進去才載入細節。

## 7.2 分頁 SQL

原版：

```go
switch dbType {
case SQLServer:
    query = fmt.Sprintf("SELECT %s FROM %s ORDER BY (SELECT NULL) OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", cols, table, offset, size)
default:
    query = fmt.Sprintf("SELECT %s FROM %s LIMIT %d OFFSET %d", cols, table, size, offset)
}
```

- SQL Server 的 `OFFSET ... FETCH` **必須搭配 ORDER BY**（2012+），`ORDER BY (SELECT NULL)` 是「隨便排」的慣用寫法。
- ⚠️ 不指定排序時，**任何 DB 都不保證分頁結果穩定**——第 2 頁可能出現第 1 頁看過的列。

### SyncCraft：依主鍵排序

```sql
-- 有主鍵
SELECT ... FROM t ORDER BY id LIMIT 50 OFFSET 100;
SELECT ... FROM [t] ORDER BY [id] OFFSET 100 ROWS FETCH NEXT 50 ROWS ONLY;
```

### 進階：Keyset（seek）分頁

`OFFSET 1000000` 仍要掃過前一百萬列。資料量大時改用「從上一頁最後一筆往後找」：

```sql
-- 下一頁：last_id 為上一頁最後一列的 id
SELECT ... FROM users WHERE id > ? ORDER BY id LIMIT 50;

-- 複合主鍵（MySQL / PG / SQLite 支援 row value 比較）
SELECT ... FROM t WHERE (a, b) > (?, ?) ORDER BY a, b LIMIT 50;
```

缺點是不能「直接跳到第 N 頁」。UI 可以兩者並用：前幾頁 OFFSET，捲動載入用 keyset。

### 回傳前的型別處理

```go
type TableDataResult struct {
    Columns    []string       `json:"columns"`
    Rows       []TableRowData `json:"rows"`     // TableRowData{ Values map[string]any }
    TotalCount int            `json:"totalCount"`
    Page, PageSize int
}
```

⚠️ 用 `map` 回傳每列 → JSON 物件的鍵順序不保證；前端一定要用 `columns` 陣列決定欄位順序。SyncCraft 可改成 `Rows [][]any`（陣列比 map 小很多，大量資料傳輸更快）。

## 7.3 連線儲存（原版）

```go
type SavedConnection struct {
    Name   string           `json:"name"`
    Config ConnectionConfig `json:"config"`   // 含明文密碼
}
// 檔案：~/.syncforge/connections.json，權限 0600
```

- 讀寫用 `sync.RWMutex` 保護（Bind 方法會在多個 goroutine 同時被呼叫）。
- `Save` 以名稱為 key：同名覆蓋、不同名新增。
- 寫檔權限 `0600`（只有本人可讀）——在 Windows 上這個權限位元基本無效。

### 問題

1. **明文密碼**：備份、同步雲端硬碟、截圖都可能洩漏。
2. 存在 `~/.syncforge` 而非各平台慣例的設定資料夾。
3. 寫檔不是原子操作，寫到一半當機會毀掉整個檔案。

## 7.4 SyncCraft：系統金鑰圈 + 慣例路徑 + 原子寫入

### 設定檔位置

```go
dir, _ := os.UserConfigDir()
// macOS:   ~/Library/Application Support
// Windows: %AppData%  (C:\Users\me\AppData\Roaming)
// Linux:   ~/.config  (或 $XDG_CONFIG_HOME)
path := filepath.Join(dir, "SyncCraft", "connections.json")
```

### 密碼放進 OS keychain

使用 `github.com/zalando/go-keyring`（純 Go，三平台都支援）：

| 平台 | 後端 |
|---|---|
| macOS | Keychain |
| Windows | Credential Manager |
| Linux | Secret Service（GNOME Keyring / KWallet，經 D-Bus） |

```go
import "github.com/zalando/go-keyring"

const service = "SyncCraft"

func (s *Store) Save(conn SavedConnection) error {
    if err := keyring.Set(service, conn.ID, conn.Config.Password); err != nil {
        return fmt.Errorf("keyring: %w", err)
    }
    conn.Config.Password = ""          // JSON 裡不存密碼
    return s.upsertAndWrite(conn)
}

func (s *Store) Resolve(id string) (ConnectionConfig, error) {
    conn := s.find(id)
    pw, err := keyring.Get(service, id)
    if err != nil && !errors.Is(err, keyring.ErrNotFound) { return conn.Config, err }
    conn.Config.Password = pw
    return conn.Config, nil
}
```

> Linux 沒有 Secret Service（例如純 Hyprland 未裝 gnome-keyring）時 `keyring.Set` 會失敗——要提供退路：不存密碼、每次詢問。

### 原子寫入

```go
func writeAtomic(path string, data []byte) error {
    tmp, err := os.CreateTemp(filepath.Dir(path), ".connections-*.json")
    if err != nil { return err }
    if _, err := tmp.Write(data); err != nil { tmp.Close(); os.Remove(tmp.Name()); return err }
    if err := tmp.Sync(); err != nil { tmp.Close(); os.Remove(tmp.Name()); return err }
    tmp.Close()
    return os.Rename(tmp.Name(), path)   // 同一檔案系統上 rename 是原子的
}
```

### 前端永遠拿不到密碼

`GetSavedConnections()` 回傳的清單不含密碼；前端只傳 `connectionId` 給 Go，Go 端自己 `Resolve`。這也讓 DevTools 裡看不到密碼。

## 7.5 原版的自動更新（簡介）

`updater/updater.go`：

1. `GET https://api.github.com/repos/<owner>/<repo>/releases/latest` 取最新 tag。
2. 與內建版本比較（`compareVersions`）。
3. 依 `runtime.GOOS/GOARCH` 找對應 asset 名稱並下載。
4. 替換自身：macOS 解壓 `.app` 覆蓋、Windows 用批次檔在程式結束後改名、Linux 直接覆蓋並 `chmod +x`。

SyncCraft 不自己寫這段：Wails v3 **內建 updater**，支援 GitHub Releases、Ed25519 簽章驗證、原子替換與回滾（第 10.9 節）。原版沒有驗證下載檔案，等於讓任何能竄改下載內容的人在你電腦上執行程式。

## 練習

1. 實作 `GetTableData` 依主鍵排序的版本，無主鍵時退回原版行為並在 UI 顯示警告。
2. 實作 Store：`os.UserConfigDir` + `go-keyring` + 原子寫入；在 Fedora 上確認 GNOME Keyring（`secret-tool search service SyncCraft`）裡看得到。
3. 讓 `GetSavedConnections` 不回傳密碼，`TestConnection` 改收 `connectionId` 或完整設定兩種形式。


# 第 8 部　React + TypeScript：把 Vue 前端改寫成 React

原版前端：`App.vue`（1148 行）+ 4 個元件 + `vue-i18n`。本部以「Vue 概念 → React 對應」的方式重建。

## 8.1 Vue ↔ React 對照

| Vue 3（原版） | React | 說明 |
|---|---|---|
| `ref(x)` / `reactive` | `useState(x)` | React 狀態不可變：`setX(新值)`，不能 `x.value = ...` |
| `computed(() => ...)` | 直接在 render 計算，昂貴時 `useMemo` | |
| `watch(src, fn)` | `useEffect(fn, [src])` | 依賴陣列決定何時重跑 |
| `onMounted` | `useEffect(fn, [])` | |
| `onUnmounted` | `useEffect` 回傳的 cleanup | |
| `props` + `emit('x')` | props + callback prop（`onX`） | |
| `v-model` | `value` + `onChange` | 受控元件 |
| `v-if` / `v-for` | `{cond && <X/>}` / `array.map(...)` | `map` 要給 `key` |
| `<style scoped>` | CSS Modules（`X.module.css`） | |
| `vue-i18n` `t()` | `react-i18next` `useTranslation()` | 可直接沿用 `locales/en.ts`、`zh.ts` |

## 8.2 專案結構

```
frontend/src/
├── main.tsx
├── App.tsx                    # 版面：頂部連線列 + 分頁
├── api/
│   └── wails.ts               # 包一層 bindings，統一處理 null / 錯誤 / 取消
├── state/
│   └── connections.tsx        # Context：source/target 設定
├── hooks/
│   ├── useAsync.ts
│   └── useWailsEvent.ts
├── features/
│   ├── connection/ConnectionForm.tsx
│   ├── schema/SchemaCompare.tsx  + DiffList.tsx
│   ├── data/DataSync.tsx
│   └── browser/TableBrowser.tsx
├── components/                # 共用 UI：Button、Tabs、SqlBlock、Modal
├── i18n/{index.ts,en.ts,zh.ts}
└── types.ts                   # 前端自己的型別（UI 狀態等）
```

## 8.3 TypeScript 與 Wails 產生的型別

Wails v3 預設把 Go struct 產生成 TS **interface**，直接用物件字面值當參數即可（v2 產生的是 class，雖然因為 TS 是結構型別也能這樣用，但比較不直覺）：

```ts
import type { ConnectionConfig } from '@bindings/internal/app'   // 第 1.3 節設定的別名

const cfg: ConnectionConfig = {
  type: 'mysql', host: '127.0.0.1', port: 3306,
  user: 'root', password: 'root', database: 'shop',
}
```

Go 的 `Type DBType` 在 TS 只是 `string`。自己收窄成聯集型別：

```ts
// types.ts
export type DBType = 'mysql' | 'postgresql' | 'sqlite' | 'sqlserver'

export const DEFAULT_PORT: Record<DBType, number> = {
  mysql: 3306, postgresql: 5432, sqlserver: 1433, sqlite: 0,
}

export type Side = 'source' | 'target'
export type DiffType = 'added' | 'removed' | 'modified'
```

> `Record<K, V>` 在這裡的好處：之後新增 `'mariadb'` 到 `DBType`，所有 `Record<DBType, ...>` 沒補上的地方都會編譯錯誤。

### 判別聯集（Discriminated Union）描述非同步狀態

```ts
export type AsyncState<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: string }
```

在 `switch (s.status)` 裡，TS 會自動知道 `success` 分支有 `data`、`error` 分支有 `error`。

## 8.4 包一層 API

```ts
// api/wails.ts
import { ConnService, SchemaService, DataService, BrowserService } from '@bindings/internal/app'
import type { ConnectionConfig } from '@bindings/internal/app'

const arr = <T,>(x: T[] | null | undefined): T[] => x ?? []   // Go nil slice → null

export const api = {
  testConnection: (c: ConnectionConfig) => ConnService.TestConnection(c),
  getDatabases: async (c: ConnectionConfig) => arr(await ConnService.GetDatabases(c)),
  compareSchemas: async (s: ConnectionConfig, t: ConnectionConfig) =>
    arr(await SchemaService.CompareSchemas(s, t)),
  executeSQL: (c: ConnectionConfig, sql: string) => SchemaService.ExecuteSQL(c, sql),
  getTablesForSync: async (c: ConnectionConfig) => arr(await DataService.GetTablesForSync(c)),
  compareTableData: async (s: ConnectionConfig, t: ConnectionConfig, table: string) =>
    arr(await DataService.CompareTableData(s, t, table)),
  getTableData: BrowserService.GetTableData,
  getTableStructure: BrowserService.GetTableStructure,
}

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))
```

好處：元件不直接依賴 bindings 的路徑與 Service 切分方式；Go 端重構時只改這一檔。

> 實際匯出的名稱以 `wails3` 產生的 `bindings/.../index.ts` 為準。
>
> 注意：上面用 `async` 包過的函式會回傳一般的 `Promise`，**`.cancel()` 就不見了**。需要取消的呼叫（例如資料比對）要直接回傳 Service 的 `CancellablePromise`，不要用 `async` 包。

## 8.5 自訂 Hooks

```ts
// hooks/useAsync.ts
import { useCallback, useState } from 'react'
import type { AsyncState } from '../types'
import { errMsg } from '../api/wails'

export function useAsync<A extends unknown[], T>(fn: (...args: A) => Promise<T>) {
  const [state, setState] = useState<AsyncState<T>>({ status: 'idle' })
  const run = useCallback(async (...args: A) => {
    setState({ status: 'loading' })
    try {
      const data = await fn(...args)
      setState({ status: 'success', data })
      return data
    } catch (e) {
      setState({ status: 'error', error: errMsg(e) })
      throw e
    }
  }, [fn])
  return { state, run }
}
```

```ts
// hooks/useWailsEvent.ts
import { useEffect, useRef } from 'react'
import { Events } from '@wailsio/runtime'

export function useWailsEvent<T>(name: string, handler: (payload: T) => void) {
  const ref = useRef(handler)
  ref.current = handler                 // 永遠呼叫最新的 handler，不必重新訂閱
  useEffect(() => Events.On(name, (ev) => ref.current(ev.data as T)), [name])  // 回傳值即取消訂閱
}
```

Go 端用 `application.RegisterEvent[T]("name")` 註冊過的事件，產生器會提供有型別的事件名稱，`ev.data` 就不需要 `as T`。

```ts
// hooks/useCancellable.ts：可取消的長時間呼叫
import { useRef, useState } from 'react'
import type { CancellablePromise } from '@wailsio/runtime'

export function useCancellable<A extends unknown[], T>(fn: (...args: A) => CancellablePromise<T>) {
  const ref = useRef<CancellablePromise<T> | null>(null)
  const [running, setRunning] = useState(false)
  const run = async (...args: A) => {
    ref.current?.cancel()
    const p = fn(...args)
    ref.current = p
    setRunning(true)
    try { return await p } finally { if (ref.current === p) { ref.current = null; setRunning(false) } }
  }
  const cancel = () => ref.current?.cancel()
  return { run, cancel, running }
}
```

> 泛型元件/函式在 `.tsx` 裡寫 `<T,>`（加逗號）避免被當成 JSX 標籤。

## 8.6 全域狀態：Context + useReducer

source / target 兩組連線設定會被所有分頁用到：

```tsx
// state/connections.tsx
import { createContext, useContext, useReducer, type ReactNode, type Dispatch } from 'react'
import type { ConnectionConfig } from '@bindings/internal/app'
import type { Side } from '../types'

type SideState = { config: ConnectionConfig; connected: boolean; databases: string[] }
type State = Record<Side, SideState>

type Action =
  | { type: 'setConfig'; side: Side; patch: Partial<ConnectionConfig> }
  | { type: 'connected'; side: Side; databases: string[] }
  | { type: 'disconnect'; side: Side }

const empty = (port: number): SideState => ({
  config: { type: 'mysql', host: '127.0.0.1', port, user: 'root', password: '', database: '' },
  connected: false, databases: [],
})

function reducer(s: State, a: Action): State {
  switch (a.type) {
    case 'setConfig':  return { ...s, [a.side]: { ...s[a.side], config: { ...s[a.side].config, ...a.patch } } }
    case 'connected':  return { ...s, [a.side]: { ...s[a.side], connected: true, databases: a.databases } }
    case 'disconnect': return { ...s, [a.side]: { ...s[a.side], connected: false, databases: [] } }
  }
}

const Ctx = createContext<{ state: State; dispatch: Dispatch<Action> } | null>(null)

export function ConnectionsProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, { source: empty(3306), target: empty(3307) })
  return <Ctx.Provider value={{ state, dispatch }}>{children}</Ctx.Provider>
}

export function useConnections() {
  const v = useContext(Ctx)
  if (!v) throw new Error('useConnections must be used inside ConnectionsProvider')
  return v
}
```

`switch` 沒有 `default` 也沒關係：TS 知道 `Action` 只有三種，漏寫會報「函式缺少結尾 return」。

> 狀態再變複雜可換成 **Zustand**（API 更簡單、不需 Provider）。學習階段先用內建的，理解 React 本身的資料流。

## 8.7 元件範例：連線表單

```tsx
// features/connection/ConnectionForm.tsx
import { useConnections } from '../../state/connections'
import { useAsync } from '../../hooks/useAsync'
import { api } from '../../api/wails'
import { DEFAULT_PORT, type DBType, type Side } from '../../types'

export function ConnectionForm({ side }: { side: Side }) {
  const { state, dispatch } = useConnections()
  const { config, connected, databases } = state[side]
  const connect = useAsync(async () => {
    await api.testConnection(config)
    return api.getDatabases(config)
  })

  const set = (patch: Partial<typeof config>) => dispatch({ type: 'setConfig', side, patch })

  const onConnect = async () => {
    const dbs = await connect.run().catch(() => null)
    if (dbs) dispatch({ type: 'connected', side, databases: dbs })
  }

  return (
    <form onSubmit={e => { e.preventDefault(); onConnect() }} className="conn-form">
      <select value={config.type}
              onChange={e => { const t = e.target.value as DBType; set({ type: t, port: DEFAULT_PORT[t] }) }}>
        <option value="mysql">MySQL</option>
        <option value="postgresql">PostgreSQL</option>
        <option value="sqlserver">SQL Server</option>
        <option value="sqlite">SQLite</option>
      </select>

      {config.type === 'sqlite' ? (
        <input placeholder="/path/to/file.db" value={config.filePath ?? ''}
               onChange={e => set({ filePath: e.target.value })} />
      ) : (
        <>
          <input value={config.host} onChange={e => set({ host: e.target.value })} />
          <input type="number" value={config.port} onChange={e => set({ port: Number(e.target.value) })} />
          <input value={config.user} onChange={e => set({ user: e.target.value })} />
          <input type="password" value={config.password} onChange={e => set({ password: e.target.value })} />
        </>
      )}

      <button disabled={connect.state.status === 'loading'}>
        {connect.state.status === 'loading' ? '連線中…' : '連線'}
      </button>
      {connect.state.status === 'error' && <p className="error">{connect.state.error}</p>}

      {connected && (
        <select value={config.database} onChange={e => set({ database: e.target.value })}>
          <option value="">— 選擇資料庫 —</option>
          {databases.map(db => <option key={db} value={db}>{db}</option>)}
        </select>
      )}
    </form>
  )
}
```

## 8.8 元件範例：Schema 比對結果

```tsx
// features/schema/SchemaCompare.tsx
import { useMemo, useState } from 'react'
import type { DiffResult } from '@bindings/internal/app'
import { api } from '../../api/wails'
import { useConnections } from '../../state/connections'
import { useAsync } from '../../hooks/useAsync'

const COLOR = { added: 'green', modified: 'amber', removed: 'red' } as const

export function SchemaCompare() {
  const { state } = useConnections()
  const compare = useAsync(api.compareSchemas)
  const [selected, setSelected] = useState<Set<number>>(new Set())

  const diffs: DiffResult[] = compare.state.status === 'success' ? compare.state.data : []
  const grouped = useMemo(() => Object.groupBy(diffs, d => d.type as keyof typeof COLOR), [diffs])
  const selectedSQL = diffs.filter((_, i) => selected.has(i)).map(d => d.sql).join('\n')

  const toggle = (i: number) => setSelected(prev => {
    const next = new Set(prev)          // 不可直接改 prev：要建新的 Set 才會觸發重繪
    next.has(i) ? next.delete(i) : next.add(i)
    return next
  })

  const execute = async () => {
    await api.executeSQL(state.target.config, selectedSQL)
    setSelected(new Set())
    compare.run(state.source.config, state.target.config)
  }

  return (
    <section>
      <button onClick={() => compare.run(state.source.config, state.target.config)}>比對 Schema</button>
      {compare.state.status === 'error' && <p className="error">{compare.state.error}</p>}

      {(['added', 'modified', 'removed'] as const).map(type => (
        <div key={type}>
          <h3 className={COLOR[type]}>{type} ({grouped[type]?.length ?? 0})</h3>
          {grouped[type]?.map(d => {
            const i = diffs.indexOf(d)
            return (
              <label key={i} className="diff-row">
                <input type="checkbox" checked={selected.has(i)} onChange={() => toggle(i)} />
                <strong>{d.tableName}</strong> — {d.detail}
                <pre>{d.sql}</pre>
              </label>
            )
          })}
        </div>
      ))}

      <button disabled={selected.size === 0} onClick={execute}>執行選取的 {selected.size} 句</button>
    </section>
  )
}
```

重點觀念：

- **不可變更新**：`Set`、陣列、物件都要建立新的再 `set`，React 用參考比較判斷是否重繪。
- `as const`：讓 `COLOR` 的鍵與值變成字面值型別，`COLOR[type]` 型別精準。
- `Object.groupBy` 需要 ES2024；`tsconfig` 的 `lib` 要含 `ES2024`（或自己寫 reduce）。

## 8.9 大量資料：虛擬捲動

資料同步可能有上萬筆差異，全部 render 會卡。用 `@tanstack/react-virtual`：

```tsx
import { useVirtualizer } from '@tanstack/react-virtual'

function DiffTable({ rows }: { rows: RowChange[] }) {
  const parentRef = useRef<HTMLDivElement>(null)
  const v = useVirtualizer({ count: rows.length, getScrollElement: () => parentRef.current, estimateSize: () => 32 })
  return (
    <div ref={parentRef} style={{ height: 500, overflow: 'auto' }}>
      <div style={{ height: v.getTotalSize(), position: 'relative' }}>
        {v.getVirtualItems().map(item => (
          <div key={item.key} style={{ position: 'absolute', top: 0, transform: `translateY(${item.start}px)`, height: item.size }}>
            <RowView row={rows[item.index]} />
          </div>
        ))}
      </div>
    </div>
  )
}
```

## 8.10 i18n

```bash
npm i i18next react-i18next
```

```ts
// i18n/index.ts
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './en'; import zh from './zh'      // 直接搬原版 locales/*.ts

i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, zh: { translation: zh } },
  lng: localStorage.getItem('lang') ?? 'zh',
  interpolation: { escapeValue: false },
})
export default i18n
```

```tsx
const { t } = useTranslation()
<h2>{t('dataSync.title')}</h2>
<p>{t('dataSync.comparedTable', { table: name, count })}</p>
```

原版 vue-i18n 插值寫法是 `{count}`，i18next 預設是 `{{count}}`——搬過來時要全域取代，或設定 `interpolation: { prefix: '{', suffix: '}' }`。

## 8.11 樣式建議

- 起步：一份 `style.css` 定義 CSS 變數（深色主題：背景 `#1b2636`，與原版 `BackgroundColour` 一致）+ CSS Modules。
- 想快：Tailwind CSS。
- SQL 顯示：`<pre>` + 等寬字型即可；要語法高亮可用 `prism-react-renderer`。
- 元件庫可選 Radix UI / shadcn/ui（無樣式、可完全客製）。

## 8.12 tsconfig 重點

```jsonc
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2024", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "react-jsx",
    "strict": true,                       // 一定要開
    "noUncheckedIndexedAccess": true,     // arr[i] 的型別會是 T | undefined，逼你處理
    "noEmit": true,
    "skipLibCheck": true
  },
  "include": ["src", "bindings"]
}
```

`package.json` 的 build：`"build": "tsc && vite build"` —— 型別錯誤會讓 `wails3 build` 失敗，這是好事。

## 練習

1. 完成 `ConnectionsProvider` 與兩個 `ConnectionForm`（source / target 並排）。
2. 完成 `SchemaCompare`，對第 1 部的種子資料顯示 4 筆差異並可執行。
3. 用 `useWailsEvent` 接第 2.6 節的 `sync:progress`，做一條真的進度條。
4. 把原版 `locales/zh.ts` 搬過來，切換語言不需重開。


# 第 9 部　擴充功能：雙表並排比對（SyncCraft 原創）

> 原版沒有這個功能。原版的資料同步假設「兩邊是同名表、同結構」，目的是**同步**；
> 這個功能的目的是**檢查**：任意挑兩張表（不同主機、不同 DB 種類、不同表名都可以），並排打開，看筆數、欄位、資料差在哪。
>
> 典型情境：
> - 正式機 `lia_ins.policy` vs 測試機 `lia_ins_uat.policy`：上版前確認資料一致
> - ETL 後驗證：MySQL 舊系統 `t_member` vs PostgreSQL 新系統 `member`
> - 同一個 DB 內：`orders` vs `orders_backup_20261120`

## 9.1 功能規格

```
┌──────────── 左表 ────────────┐ ┌──────────── 右表 ────────────┐
│ 連線 [正式 MySQL ▼]           │ │ 連線 [新系統 PG ▼]            │
│ DB/Schema [lia_ins ▼]        │ │ DB/Schema [public ▼]          │
│ 表 [t_member ▼]              │ │ 表 [member ▼]                 │
│ WHERE [created_at >= '2026'] │ │ WHERE [created_at >= '2026']  │
└──────────────────────────────┘ └───────────────────────────────┘
                 [ 1. 摘要 ]  [ 2. 欄位對應 ]  [ 3. 逐列比對 ]  [ 4. 欄位統計 ]
```

| 層級 | 內容 | 成本 |
|---|---|---|
| **1. 摘要** | 兩邊筆數、差額、欄位數、主鍵 | 2 次 COUNT + metadata，秒級 |
| **2. 欄位對應** | 同名欄位自動配對，型別差異標示；可手動改對應、勾選比較鍵 | 純 metadata |
| **3. 逐列比對** | 依「比較鍵」對齊：只在左 / 只在右 / 內容不同（標出哪幾欄）/ 相同 | 讀全表（串流） |
| **4. 欄位統計** | 每欄 COUNT、NULL 數、DISTINCT 數、MIN、MAX、SUM | 每邊 1 條聚合 SQL |

「欄位統計」是逐列比對之前的快速健檢：百萬列的表，聚合 SQL 幾秒就跑完，若 SUM、DISTINCT 都相同，資料大致一致；不同時再做逐列比對找出是哪幾筆。

## 9.2 與原版資料同步的差異

| | 原版資料同步 | 雙表並排比對 |
|---|---|---|
| 表名 | 兩邊必須同名 | 任意 |
| 結構 | 以 source 欄位為準，target 缺欄就失敗 | 欄位對應表，只比對有配對的欄位 |
| 比較鍵 | 只能用 source 主鍵 | 預設主鍵，可改選任意欄位組合（例如身分證號） |
| 範圍 | 全表 | 可加 WHERE 過濾 |
| 跨 DB 種類 | 型別不同容易誤判 | 值正規化後比較（第 6.5 節） |
| 輸出 | 產生 SQL 去執行 | 差異報表，可匯出 CSV；**唯讀，不改任何資料** |

## 9.3 Go 端資料模型

```go
package compare

// 一邊的表
type TableRef struct {
    ConnectionID string `json:"connectionId"`  // 對應第 7 部的連線儲存
    Database     string `json:"database"`
    Schema       string `json:"schema"`        // MySQL 空字串；PG 預設 public；MSSQL 預設 dbo
    Table        string `json:"table"`
    Where        string `json:"where"`         // 選填，原樣接在 WHERE 後面
}

type ColumnPair struct {
    Left  string `json:"left"`
    Right string `json:"right"`
}

type PairRequest struct {
    Left    TableRef     `json:"left"`
    Right   TableRef     `json:"right"`
    Columns []ColumnPair `json:"columns"` // 要比較的欄位對應（空 = 自動配對）
    Keys    []ColumnPair `json:"keys"`    // 比較鍵（空 = 用左表主鍵）
}

// 第 1、2 層：摘要 + 欄位對應
type PairSummary struct {
    LeftCount    int64         `json:"leftCount"`
    RightCount   int64         `json:"rightCount"`
    LeftPK       []string      `json:"leftPk"`
    RightPK      []string      `json:"rightPk"`
    Columns      []ColumnMatch `json:"columns"`
    SuggestedKey []ColumnPair  `json:"suggestedKey"`
}

type ColumnMatch struct {
    Left      *model.Column `json:"left,omitempty"`   // nil = 只在右邊
    Right     *model.Column `json:"right,omitempty"`  // nil = 只在左邊
    TypeMatch bool          `json:"typeMatch"`        // 正規化後型別是否相容
}

// 第 3 層：逐列比對結果
type RowDiff struct {
    Kind    string         `json:"kind"`              // left_only | right_only | changed
    Key     map[string]any `json:"key"`
    Left    map[string]any `json:"left,omitempty"`
    Right   map[string]any `json:"right,omitempty"`
    Changed []string       `json:"changed,omitempty"` // 不同的欄位（左欄位名）
}

type PairDiffStats struct {
    Same, LeftOnly, RightOnly, Changed int64
    ByColumn map[string]int64 `json:"byColumn"` // 每個欄位不同的筆數，例如 {"phone": 132}
}

// 第 4 層：欄位統計
type ColumnStats struct {
    Column   string  `json:"column"`
    NonNull  int64   `json:"nonNull"`
    Nulls    int64   `json:"nulls"`
    Distinct int64   `json:"distinct"`
    Min, Max *string `json:"min,omitempty"` // 統一轉字串，避免跨型別問題
    Sum      *string `json:"sum,omitempty"` // 只有數值欄位
}
```

## 9.4 Wails 綁定

```go
type CompareService struct {
    ctx    context.Context
    pool   *db.Pool
    store  *store.Store
    cancel context.CancelFunc
    mu     sync.Mutex
}

// 第 1、2 層
func (s *CompareService) PairSummary(req compare.PairRequest) (*compare.PairSummary, error)

// 第 3 層：結果多，透過事件串流；同時寫入暫存檔，前端再分頁讀取
func (s *CompareService) PairDiffStart(ctx context.Context, req compare.PairRequest) (jobID string, err error)
func (s *CompareService) PairDiffPage(jobID, kind string, offset, limit int) ([]compare.RowDiff, error)
func (s *CompareService) PairDiffCancel(jobID string)
func (s *CompareService) PairDiffExportCSV(jobID string) (path string, err error) // 用 app.Dialog.SaveFile()

// 第 4 層
func (s *CompareService) PairColumnStats(req compare.PairRequest) (left, right []compare.ColumnStats, err error)
```

事件：

| 事件名 | payload | 用途 |
|---|---|---|
| `pair:progress` | `{jobId, leftRead, rightRead, leftTotal, rightTotal}` | 進度條 |
| `pair:stats` | `PairDiffStats` | 每讀 5000 列推一次，數字即時跳動 |
| `pair:done` | `{jobId, stats, elapsedMs}` | 完成 |
| `pair:error` | `{jobId, message}` | 失敗 |

> 建議：回傳值超過「一個結果 + error」時包成 struct，例如 `type StatsPair struct{ Left, Right []ColumnStats }`，前端拿到的物件欄位有名稱，比較好讀。
>
> **多視窗（v3）**：可以把「左表」「右表」各開成一個視窗並排在兩個螢幕上，用事件同步捲動位置：`app.Window.NewWithOptions(...)` 建第二個視窗，URL 帶參數 `/#/pair/right?job=...`。

## 9.5 自動欄位配對

```go
func AutoMatch(left, right []model.Column) []ColumnMatch {
    norm := func(s string) string {                     // user_id、UserId、USER_ID 視為同名
        return strings.ToLower(strings.ReplaceAll(s, "_", ""))
    }
    rightBy := map[string]*model.Column{}
    for i := range right { rightBy[norm(right[i].Name)] = &right[i] }

    var out []ColumnMatch
    used := map[string]bool{}
    for i := range left {
        l := &left[i]
        if r, ok := rightBy[norm(l.Name)]; ok {
            out = append(out, ColumnMatch{Left: l, Right: r, TypeMatch: typeCompatible(l.Type, r.Type)})
            used[r.Name] = true
        } else {
            out = append(out, ColumnMatch{Left: l})
        }
    }
    for i := range right {
        if !used[right[i].Name] { out = append(out, ColumnMatch{Right: &right[i]}) }
    }
    return out
}
```

`typeCompatible` 把型別歸成大類再比較：

| 大類 | MySQL | PostgreSQL | SQL Server | SQLite |
|---|---|---|---|---|
| integer | `int`, `bigint`, `tinyint` | `integer`, `bigint`, `smallint` | `int`, `bigint` | `INTEGER` |
| decimal | `decimal(p,s)` | `numeric(p,s)` | `decimal`, `money` | `NUMERIC` |
| text | `varchar`, `text`, `char` | `character varying`, `text` | `nvarchar`, `varchar` | `TEXT` |
| datetime | `datetime`, `timestamp` | `timestamp`, `timestamptz` | `datetime2`, `datetime` | `TEXT`（!） |
| bool | `tinyint(1)` | `boolean` | `bit` | `INTEGER` |

同大類但長度不同（`varchar(50)` vs `varchar(100)`）→ 黃色提示「可能截斷」。

### 比較鍵建議

1. 兩邊都有主鍵且對得上 → 用主鍵。
2. 否則找兩邊都有的 UNIQUE 索引。
3. 都沒有 → 要求使用者手動勾選，並警告「比較鍵若不唯一，結果不可靠」。

可以用一條查詢驗證使用者選的鍵是否唯一：

```sql
SELECT COUNT(*) - COUNT(DISTINCT id_no) AS dup FROM t_member;     -- 單欄
SELECT COUNT(*) FROM (SELECT a, b FROM t GROUP BY a, b HAVING COUNT(*) > 1) x;  -- 多欄
```

## 9.6 第 1 層 SQL：筆數

```go
func countSQL(d dialect.Dialect, ref TableRef) string {
    q := "SELECT COUNT(*) FROM " + d.QualifiedName(ref.Schema, ref.Table)
    if strings.TrimSpace(ref.Where) != "" { q += " WHERE " + ref.Where }
    return q
}
```

`QualifiedName`：

| DB | 結果 |
|---|---|
| MySQL | `` `lia_ins`.`t_member` ``（可跨 DB 查） |
| PostgreSQL | `"public"."member"` |
| SQL Server | `[dbo].[member]` |
| SQLite | `"member"` |

⚠️ **WHERE 是使用者輸入的原始 SQL**。這是唯讀工具、使用者本來就有 DB 帳密，所以可以接受，但要防止誤用：

- 比對全程在 **唯讀交易** 中執行：
  - MySQL：`START TRANSACTION READ ONLY`
  - PostgreSQL：`BEGIN READ ONLY`（或 `SET TRANSACTION READ ONLY`）
  - SQL Server：沒有唯讀交易，建議比對專用帳號只給 `db_datareader`
- 拒絕包含 `;` 的 WHERE（防止串接第二句）。
- 設查詢逾時：`context.WithTimeout`。

## 9.7 第 3 層：逐列比對引擎

沿用第 6 部的兩種策略，但泛化成「兩張任意表 + 欄位對應 + 任意鍵」：

```go
func DiffPair(ctx context.Context, L, R Side, cols, keys []ColumnPair, emit func(RowDiff), onStats func(PairDiffStats)) error {
    // 1. 兩邊各自產生 SELECT：只選有對應的欄位，依比較鍵排序
    //    SELECT k1, k2, c1, c2, ... FROM t WHERE ... ORDER BY k1, k2
    // 2. 決定策略
    if keysAreIntegers(L, R, keys) {
        return mergeDiff(ctx, ...)   // 排序合併，記憶體 O(1)（第 6.4 節）
    }
    return hashDiff(ctx, ...)        // 字串鍵：右表讀進 map，左表串流比對
}
```

為什麼字串鍵不能直接排序合併？因為兩邊的排序規則（collation）可能不同：

```
MySQL utf8mb4_0900_ai_ci：  'a' = 'A'，'é' = 'e'
PostgreSQL（C locale）：    'B' < 'a'（依位元組）
```

左邊認為 `a` 在 `B` 前面，右邊相反，merge 會把同一筆誤判成一邊有一邊沒有。解法二選一：

- 在 SQL 指定二進位排序：MySQL `ORDER BY k COLLATE utf8mb4_bin`、PG `ORDER BY k COLLATE "C"`、SQL Server `ORDER BY k COLLATE Latin1_General_BIN2`；
- 或用 hash 策略（實作簡單，但右表要能放進記憶體）。

### 欄位層級的差異

```go
func diffRow(l, r []any, cols []ColumnPair) (changed []string) {
    for i, c := range cols {
        if normalize(l[i]) != normalize(r[i]) {   // 第 6.5 節的正規化
            changed = append(changed, c.Left)
        }
    }
    return
}
```

`PairDiffStats.ByColumn` 累計每欄不同的筆數。這通常是最有用的資訊：**「80,000 筆有差，其中 79,950 筆只是 `updated_at` 不同」** → 使用者可以把 `updated_at` 排除在比較之外再跑一次。所以 UI 要能「排除欄位」。

### 比較選項

| 選項 | 效果 |
|---|---|
| 忽略字串前後空白 | `CHAR(10)` vs `VARCHAR` 很常見 |
| 忽略大小寫 | |
| NULL 等於空字串 | Oracle 移轉過來的資料常見 |
| 數值容差 | `0.1 + 0.2` 浮點誤差，例如 `1e-9` |
| 時間精度 | 截到秒（MySQL `datetime` 預設沒有毫秒，PG 有微秒） |
| 時區 | 把 `timestamp without time zone` 視為某時區 |

```go
type CompareOptions struct {
    TrimSpace      bool    `json:"trimSpace"`
    IgnoreCase     bool    `json:"ignoreCase"`
    NullEqualsEmpty bool   `json:"nullEqualsEmpty"`
    FloatTolerance float64 `json:"floatTolerance"`
    TimePrecision  string  `json:"timePrecision"` // "s" | "ms" | "us"
    ExcludeColumns []string `json:"excludeColumns"`
}
```

### 結果存放

差異可能幾十萬筆，不能一次塞進 Wails 回傳值（JSON 序列化 + WebView 記憶體都會爆）。做法：

1. `emit` 把每筆 `RowDiff` 以 JSON Lines 寫入暫存檔（`os.CreateTemp`），每種 kind 一個檔，同時記錄每行的 byte offset。
2. 前端分頁時呼叫 `PairDiffPage(jobID, "changed", 0, 100)`，Go 端依 offset 讀取。
3. 匯出 CSV 時串流讀檔寫出，不經過前端。
4. App 關閉時（`ServiceShutdown()`）清除暫存檔。

## 9.8 第 4 層 SQL：欄位統計

一條 SQL 拿到所有欄位的統計。以兩個欄位 `amount`（數值）、`name`（字串）為例：

**MySQL**

```sql
SELECT
  COUNT(*)                         AS total,
  COUNT(`amount`)                  AS amount_nonnull,
  COUNT(DISTINCT `amount`)         AS amount_distinct,
  CAST(MIN(`amount`) AS CHAR)      AS amount_min,
  CAST(MAX(`amount`) AS CHAR)      AS amount_max,
  CAST(SUM(`amount`) AS CHAR)      AS amount_sum,
  COUNT(`name`)                    AS name_nonnull,
  COUNT(DISTINCT `name`)           AS name_distinct,
  MIN(`name`) AS name_min, MAX(`name`) AS name_max
FROM `lia_ins`.`t_member`
WHERE created_at >= '2026-01-01';
```

**PostgreSQL**：只差在引號與轉型 `::text`

```sql
SELECT COUNT(*), COUNT("amount"), COUNT(DISTINCT "amount"),
       MIN("amount")::text, MAX("amount")::text, SUM("amount")::text, ...
FROM "public"."member" WHERE ...;
```

**SQL Server**：`COUNT` 超過 21 億要用 `COUNT_BIG`；SUM 整數可能溢位要先轉型

```sql
SELECT COUNT_BIG(*), COUNT_BIG([amount]), COUNT_BIG(DISTINCT [amount]),
       CAST(MIN([amount]) AS nvarchar(100)), CAST(MAX([amount]) AS nvarchar(100)),
       CAST(SUM(CAST([amount] AS decimal(38,4))) AS nvarchar(100)), ...
FROM [dbo].[member] WHERE ...;
```

**SQLite**

```sql
SELECT COUNT(*), COUNT("amount"), COUNT(DISTINCT "amount"),
       CAST(MIN("amount") AS TEXT), CAST(MAX("amount") AS TEXT), CAST(SUM("amount") AS TEXT), ...
FROM "member" WHERE ...;
```

Go 端用 dialect 動態組欄位清單：

```go
func statsSQL(d dialect.Dialect, ref TableRef, cols []model.Column) string {
    parts := []string{d.CountStar()}
    for _, c := range cols {
        q := d.Quote(c.Name)
        parts = append(parts, d.Count(q), d.CountDistinct(q), d.AsText("MIN("+q+")"), d.AsText("MAX("+q+")"))
        if c.Category() == "integer" || c.Category() == "decimal" {
            parts = append(parts, d.AsText(d.SafeSum(q)))
        }
    }
    q := "SELECT " + strings.Join(parts, ", ") + " FROM " + d.QualifiedName(ref.Schema, ref.Table)
    if ref.Where != "" { q += " WHERE " + ref.Where }
    return q
}
```

注意事項：

- `COUNT(DISTINCT)` 在大表很耗資源；UI 讓使用者勾選要不要算。
- 二進位 / JSON / 大文字欄位（`BLOB`、`TEXT`、`jsonb`、`nvarchar(max)`）不做 MIN/MAX/DISTINCT（部分 DB 不支援，例如 SQL Server 的 `text`、`ntext`、`image`）。
- 字串的 MIN/MAX 受 collation 影響，跨 DB 不同不一定代表資料不同 → UI 用灰色顯示，不標紅。
- 浮點 SUM 跨 DB 可能尾數不同 → 套用容差。

## 9.9 React 端

### 狀態

```ts
// features/pair/types.ts
export type TableRef = {
  connectionId: string; database: string; schema: string; table: string; where: string
}
export type PairState = {
  left: TableRef
  right: TableRef
  columns: ColumnPair[]       // 使用者確認後的對應
  keys: ColumnPair[]
  options: CompareOptions
  jobId?: string
}
```

### 版面

```tsx
export function TablePairCompare() {
  const [pair, dispatch] = useReducer(pairReducer, initialPair)
  const summary = useAsync(api.pairSummary)
  const [tab, setTab] = useState<'summary' | 'columns' | 'rows' | 'stats'>('summary')

  return (
    <div className="pair-layout">
      <div className="pair-pickers">
        <TablePicker side="left"  value={pair.left}  onChange={v => dispatch({ type: 'setLeft', v })} />
        <SwapButton onClick={() => dispatch({ type: 'swap' })} />
        <TablePicker side="right" value={pair.right} onChange={v => dispatch({ type: 'setRight', v })} />
      </div>
      <Tabs value={tab} onChange={setTab} />
      {tab === 'summary' && <SummaryPanel state={summary.state} />}
      {tab === 'columns' && <ColumnMapping pair={pair} dispatch={dispatch} />}
      {tab === 'rows'    && <RowDiffPanel pair={pair} />}
      {tab === 'stats'   && <StatsPanel pair={pair} />}
    </div>
  )
}
```

### 摘要卡片

```tsx
function SummaryPanel({ s }: { s: PairSummary }) {
  const diff = s.leftCount - s.rightCount
  return (
    <div className="cards">
      <Stat label="左表筆數" value={s.leftCount.toLocaleString()} />
      <Stat label="右表筆數" value={s.rightCount.toLocaleString()} />
      <Stat label="差額" value={(diff > 0 ? '+' : '') + diff.toLocaleString()} tone={diff === 0 ? 'ok' : 'warn'} />
      <Stat label="共同欄位" value={s.columns.filter(c => c.left && c.right).length} />
      <Stat label="只在左 / 右" value={`${s.columns.filter(c => !c.right).length} / ${s.columns.filter(c => !c.left).length}`} />
    </div>
  )
}
```

### 逐列差異：並排顯示

```
 鍵: id=1024                         左 (正式)            右 (UAT)
 ─────────────────────────────────────────────────────────────────
 name                                王小明               王小明
 phone                    ●          0912-345-678         0912345678      ← 不同欄位標紅
 updated_at               ●          2026-11-20 10:00     2026-11-19 08:00
```

- 上方篩選 chip：`內容不同 1,203` `只在左 87` `只在右 12` `相同 98,698`。
- 「各欄差異數」長條圖：一眼看出是哪個欄位造成大量差異，點一下即可「排除此欄重跑」。
- 列表用第 8.9 節的虛擬捲動；資料來源是 `PairDiffPage` 分頁取得。

### 進度

```tsx
const [prog, setProg] = useState<Progress | null>(null)
const [live, setLive] = useState<PairDiffStats | null>(null)
useWailsEvent<Progress>('pair:progress', p => p.jobId === pair.jobId && setProg(p))
useWailsEvent<PairDiffStats>('pair:stats', setLive)
```

## 9.10 實作順序

1. `PairSummary`：筆數 + 欄位自動配對（完成即可解決「比對資料筆數」的需求）。
2. `PairColumnStats`：聚合統計並排表格。
3. `DiffPair` hash 策略 + 暫存檔分頁。
4. 排序合併策略 + collation 處理。
5. 比較選項、排除欄位、匯出 CSV。
6. 加分：把一組比對設定（左右表、對應、鍵、選項）存成「比對範本」，下次一鍵重跑；甚至可做排程定期比對。

## 練習

1. 實作 `QualifiedName` 與 `countSQL`，在 mysql-src 與 pg-src 各跑一次，比對 `users` 筆數。
2. 實作 `AutoMatch` + `typeCompatible`，用 MySQL `users` 對 PG `users` 測試，預期 `datetime` ↔ `timestamp` 判定相容。
3. 實作 `statsSQL` 四種方言，寫測試驗證產生的 SQL 字串。
4. 實作 hash 策略的 `DiffPair`，跑出 `ByColumn` 統計，並在 React 端畫出並排差異列表。


# 第 10 部　跨平台打包：macOS（M 系列）／ Windows ／ Linux

## 10.1 v3 的建置系統：Taskfile

v2 的 `wails build` 是一個黑盒子；v3 改用 [Task](https://taskfile.dev)，所有步驟都寫在專案裡的 YAML，**看得到、改得了**：

```
Taskfile.yml                 # 根：build / package / run 依 GOOS 分派
build/Taskfile.yml           # 共用：go mod tidy、前端安裝與建置、產生綁定、產生圖示
build/darwin/Taskfile.yml    # macOS：build、build:universal、package、create:dmg、sign、sign:notarize
build/windows/Taskfile.yml   # Windows：build、package、create:nsis:installer、create:msix:package、sign
build/linux/Taskfile.yml     # Linux：build、package、create:appimage / deb / rpm / aur、sign:packages
build/docker/                # 交叉編譯用的 Dockerfile
build/config.yml             # App 資訊（名稱、版本、識別碼…）與 dev 模式設定
```

常用指令：

```bash
wails3 build                       # 建置目前平台 → bin/synccraft
wails3 package                     # 建置 + 產生該平台的安裝檔
GOOS=windows wails3 build          # 指定目標平台
wails3 task --list                 # 列出所有可用任務
wails3 task darwin:package:universal
```

`wails3 build` / `wails3 package` 其實就是執行根 Taskfile 的 `build` / `package`，它再依 `GOOS` 呼叫 `build/<os>/Taskfile.yml` 裡的對應任務。想加步驟（例如注入版本號、先跑測試）就直接改 YAML。

## 10.2 App 資訊：build/config.yml

```yaml
info:
  companyName: "SyncCraft"
  productName: "SyncCraft"
  productIdentifier: "dev.synccraft.app"   # macOS bundle id / Windows 識別碼，發佈後不要再改
  description: "Database schema & data compare tool"
  copyright: "© 2026 SyncCraft contributors"
  version: "0.1.0"
```

改完後要重新產生各平台的資源檔（Info.plist、Windows 版本資訊、`.desktop` 等）：

```bash
wails3 task common:update:build-assets
```

> ⚠️ 這個指令會**覆蓋** `build/` 底下你手動改過的平台檔案。有客製化的話，先 commit 再執行，然後用 `git diff` 檢查。

圖示：替換 `build/appicon.png`（1024×1024），建置時會自動產生 `.icns`、`.ico` 等格式。

## 10.3 第一個大坑：mattn/go-sqlite3 需要 CGO

原版使用 `github.com/mattn/go-sqlite3`，它是 C 版 SQLite 的包裝，需要 CGO：

- 從 Linux 交叉編譯 Windows 版 → 需要 MinGW 或 Docker
- 在 Windows 本機建置 → 需要安裝 gcc

**SyncCraft 改用純 Go 的 SQLite**：

```go
import _ "modernc.org/sqlite"      // driver 名稱是 "sqlite"（不是 "sqlite3"）

db, err := sql.Open("sqlite", "/path/to/file.db")
```

| | mattn/go-sqlite3 | modernc.org/sqlite |
|---|---|---|
| CGO | 需要 | 不需要 |
| 效能 | 較快 | 稍慢（比對工具感覺不出差異） |
| 交叉編譯 | 困難 | 直接過 |

其餘三個 driver（mysql、pgx、mssql）都是純 Go。v3 的 Windows Taskfile 預設 `CGO_ENABLED=0`，換掉 SQLite 之後，**`GOOS=windows wails3 build` 可以直接在你的 Fedora 上產出 Windows 版**，不需要 Docker。

## 10.4 各平台能從哪裡建置

| 目標 | 在 Linux 上 | 在 macOS 上 | 在 Windows 上 | 備註 |
|---|---|---|---|---|
| Linux | ✅ 原生 | 🐳 Docker | 🐳 Docker | 需要 CGO（GTK） |
| Windows | ✅ 原生（CGO=0） | ✅ 原生 | ✅ 原生 | 前提是沒有其他 CGO 套件 |
| macOS | 🐳 Docker（未簽章） | ✅ 原生 | 🐳 Docker | **簽章與公證必須在 Mac 上做** |

Docker 交叉編譯要先建 image：

```bash
wails3 task common:setup:docker   # 建立 wails-cross image（只需一次）
```

> 實務建議：**本機只建 Linux 與 Windows**；macOS 版交給 CI 的 `macos-latest`（Apple Silicon 機器），或你自己的 Mac。

## 10.5 macOS（Apple Silicon M1–M4）

```bash
wails3 build                               # 本機架構（M 系列 = arm64）
ARCH=amd64 wails3 build                    # Intel Mac
wails3 task darwin:build:universal         # 一個 App 同時支援兩種架構
wails3 package                             # → bin/SyncCraft.app
wails3 task darwin:package:dmg             # → .dmg
```

### Gatekeeper：未簽章的 App

從網路下載的未簽章 App，打開時會出現「已損毀」或「無法驗證開發者」。

- **自己用 / 內部測試**：

  ```bash
  xattr -cr /Applications/SyncCraft.app    # 移除 quarantine 屬性
  ```

  或到「系統設定 → 隱私權與安全性」按「強制打開」。

- **正式發佈**：需要 Apple Developer Program（USD 99/年）。v3 內建簽章與公證流程：

  ```bash
  wails3 setup signing          # 互動設定憑證；密碼存在系統 keychain，不寫進專案
  wails3 task darwin:sign
  wails3 task darwin:sign:notarize
  ```

### 選單與複製貼上

v2 沒設定 Edit 選單時，Cmd+C/V 在 WebView 裡沒有作用（原版就有這個問題）。**v3 沒有提供自訂選單時，會自動套用預設的應用程式選單**，複製貼上可以直接用。要自訂選單時，記得自己加回 Edit 選單（`application.NewEditMenu()` 等角色選單）。

## 10.6 Windows

```bash
GOOS=windows wails3 build                  # 可在 Linux / macOS / Windows 上執行
GOOS=windows ARCH=arm64 wails3 build       # Snapdragon / Surface
GOOS=windows wails3 package                # NSIS 安裝程式（需安裝 makensis）
wails3 task windows:create:msix:package    # MSIX（Microsoft Store / Intune 用）
```

Fedora 上安裝 NSIS：`sudo dnf install mingw32-nsis`（套件名稱依發行版而異）。

### WebView2

Windows 11 內建 WebView2；Windows 10 多半也已經有。NSIS 安裝程式會在缺少時協助安裝。

> 公會內網這類**不能連外**的環境：用 `wails3 generate webview2bootstrapper` 取得 bootstrapper，或請 IT 事先佈署 WebView2 Evergreen Runtime。

### SmartScreen

未簽章的 exe 會出現「Windows 已保護您的電腦」。內部使用可以按「其他資訊 → 仍要執行」；正式發佈需要程式碼簽章憑證（OV/EV，或 Azure Trusted Signing），設定好後用 `wails3 task windows:sign`。

## 10.7 Linux

```bash
wails3 build                    # GTK4 + WebKitGTK 6.0（預設）
wails3 build -tags gtk3         # 改用 GTK3 + webkit2gtk-4.1（給較舊的發行版）
wails3 package                  # 一次產生 AppImage、.deb、.rpm、Arch 套件
wails3 task linux:create:rpm    # 只產生其中一種
```

打包設定在 `build/linux/nfpm/nfpm.yaml`（v3 內建 nFPM，不用另外安裝）。

### 相依套件與發行版

| 後端 | 需要的執行期套件 | 可執行的發行版（約略） |
|---|---|---|
| GTK4（預設） | `libgtk-4-1`、`libwebkitgtk-6.0-4` | Ubuntu 24.04+、Debian 13+、Fedora 39+ |
| GTK3（`-tags gtk3`） | `libgtk-3-0`、`libwebkit2gtk-4.1-0` | Ubuntu 22.04+、Debian 12+ |

### glibc 相容性

在 Fedora 45 上建置的執行檔會連結到很新的 glibc，拿到 Ubuntu 24.04 可能出現 `GLIBC_2.xx not found`。**發佈版要在「最舊的目標發行版」上建置**，例如 CI 用 `ubuntu-24.04`。本機 Fedora 的建置只給自己用。

### Hyprland / Wayland

WebKitGTK 在部分 Wayland + NVIDIA 環境會白畫面，可以試：

```bash
WEBKIT_DISABLE_DMABUF_RENDERER=1 ./bin/synccraft
```

## 10.8 版本號

單一來源：`build/config.yml` 的 `info.version`。

```bash
wails3 tool version --help      # 內建的語意化版本號遞增工具
```

Go 程式裡要顯示版本時，在 `build/<os>/Taskfile.yml` 的 `BUILD_FLAGS` 加上 `-X main.version={{.VERSION}}`，或在發佈時由 CI 用 git tag 注入。

## 10.9 自動更新（取代原版 updater）

原版自己寫了下載、替換執行檔的邏輯（第 7.5 節），沒有驗證檔案來源。v3 **內建 updater**：

- 支援 GitHub Releases 等更新來源
- **Ed25519 簽章驗證**：下載的檔案必須用你的私鑰簽過，才會被接受
- 原子替換，失敗可以回滾
- 內建更新 UI，可以自訂外觀

發佈端的工具：

```bash
wails3 updater genkey          # 產生簽章金鑰對（私鑰放 CI secret，公鑰編進 App）
wails3 updater sign ...        # 對發佈檔案計算 digest 並簽章
wails3 updater manifest ...    # 產生更新清單
wails3 updater verify ...      # 驗證清單、檔案、公鑰是否一致
```

App 端的整合方式請以官方文件 Updater 章節為準；SyncCraft 排在 M6 之後再做。

## 10.10 GitHub Actions：三平台建置

```yaml
# .github/workflows/release.yml
name: Release
on:
  push:
    tags: ['v*']

jobs:
  build:
    strategy:
      fail-fast: false
      matrix:
        include:
          - { os: macos-latest,     name: macos-universal, task: "darwin:package:universal" }
          - { os: windows-latest,   name: windows-amd64,   task: "windows:package" }
          - { os: ubuntu-24.04,     name: linux-amd64,     task: "linux:package" }
          - { os: ubuntu-24.04-arm, name: linux-arm64,     task: "linux:package" }
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v7
      - uses: jdx/mise-action@v5              # 讀 mise.toml：與本機相同版本的 wails3
      - uses: actions/setup-go@v7
        with: { go-version-file: go.mod }
      - uses: actions/setup-node@v7
        with: { node-version: lts/*, cache: npm, cache-dependency-path: frontend/package-lock.json }

      - name: Linux deps
        if: runner.os == 'Linux'
        run: sudo apt-get update && sudo apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev

      - run: wails3 task ${{ matrix.task }}

      - uses: actions/upload-artifact@v7
        with:
          name: synccraft-${{ matrix.name }}
          path: bin/

  release:
    needs: build
    runs-on: ubuntu-latest
    permissions: { contents: write }
    steps:
      - uses: actions/download-artifact@v8
        with: { path: dist }
      - run: cd dist && find . -type f -exec sha256sum {} \; > SHA256SUMS.txt
      - uses: softprops/action-gh-release@v3
        with:
          files: dist/**/*
          generate_release_notes: true
```

> 注意 mise 安裝 `wails3` 時本身也要編譯，所以 Linux 的 GTK4 套件要**在 mise 之前**裝好。實際的 workflow 檔（第 12 部）有處理這個順序。
>
> 公司內部若用 Gitea：Gitea Actions 語法與 GitHub Actions 相容，但 macOS 建置需要自己準備 Mac runner。

## 10.11 平台差異檢查清單

| 項目 | macOS | Windows | Linux |
|---|---|---|---|
| 設定檔路徑（`os.UserConfigDir`） | `~/Library/Application Support/SyncCraft` | `%AppData%\SyncCraft` | `~/.config/SyncCraft` |
| 密碼儲存 | Keychain | Credential Manager | Secret Service（需 gnome-keyring 等） |
| 檔案路徑 | `/` | `\`（一律用 `filepath.Join`） | `/` |
| 快捷鍵 | `Cmd` | `Ctrl` | `Ctrl` |
| 字型 | `-apple-system` | `Segoe UI` | `system-ui` / Noto |
| 安裝檔 | `.app` / `.dmg` | NSIS `.exe` / `.msix` | AppImage / `.deb` / `.rpm` |

## 練習

1. 換成 `modernc.org/sqlite`，在 Fedora 上執行 `GOOS=windows wails3 build`，拿到 Windows 機器上跑。
2. 修改 `build/config.yml` 的 `info`，執行 `common:update:build-assets`，用 `git diff` 看改了哪些檔。
3. 執行 `wails3 package`，用 `sudo dnf install ./bin/*.rpm` 安裝並從應用程式選單啟動。
4. 推一個 `v0.0.1` tag，讓 GitHub Actions 產出三平台檔案。


# 第 11 部　實作路線圖與驗收清單

每個里程碑都能獨立 demo，並對應前面的章節。

## M0　骨架（第 1、2 部）

- [ ] `wails3 init -t react`，升級 React / Vite / TS，設定 `@bindings` 別名
- [ ] `internal/` 目錄結構、多個 Service（`application.NewService`）
- [ ] `api/wails.ts` 包裝層、`useAsync`、`useWailsEvent`
- [ ] docker compose 起好四種 DB 並灌入種子資料
- **驗收**：`wails3 dev` 開窗，按鈕呼叫 Go 回傳 `Hello`，錯誤時顯示錯誤訊息；長時間呼叫可 `.cancel()`
- **維護設定**：`renovate.json`、CI workflow、LICENSE（第 12 部）

## M1　連線（第 3、7 部）

- [ ] `Pool`（快取 `*sql.DB`、Ping 逾時、`ServiceShutdown` 時關閉）
- [ ] 結構化 DSN（`mysql.Config`、`url.URL`）
- [ ] 換成 `modernc.org/sqlite`、`pgx`、`microsoft/go-mssqldb`
- [ ] Store：`os.UserConfigDir` + keyring + 原子寫入
- [ ] React：`ConnectionsProvider`、`ConnectionForm`、已存連線下拉選單、SQLite 原生選檔對話框
- **驗收**：四種 DB 都能連線並列出資料庫；密碼不出現在 JSON 檔與 DevTools 中

## M2　Introspection（第 4 部）

- [ ] `dialect.Dialect` 介面 + 四個實作的 `ListTables / Columns / PrimaryKey / Indexes`
- [ ] 每個實作都有整合測試（用 docker DB，`go test -tags integration`）
- [ ] 表瀏覽：表清單（估計筆數）、結構、依主鍵排序的分頁資料
- **驗收**：PG 大小寫混合表名、SQLite 複合主鍵、MSSQL `nvarchar(max)` 都正確顯示

## M3　Schema 比對（第 5 部）

- [ ] `schema.Diff` 純函式 + 欄位正規化 + table-driven 單元測試
- [ ] 四個 dialect 的 DDL 產生器（先完成 MySQL → MySQL、PG → PG）
- [ ] UI：分組、勾選、預覽 SQL、標示破壞性操作（DROP 標紅並二次確認）
- [ ] 交易執行（MySQL 提示 DDL 無法回滾）
- **驗收**：種子資料產生 4 筆差異，執行後再比對變成 0 筆

## M4　資料同步（第 6 部）

- [ ] 參數化 DML、DELETE → UPDATE → INSERT 順序、交易、批次
- [ ] 值正規化、`Changed` 欄位標示
- [ ] 事件進度、取消
- [ ] 大表：整數主鍵用排序合併策略
- **驗收**：種子資料 INSERT 1 / UPDATE 1 / DELETE 1；10 萬列表比對時記憶體穩定

## M5　雙表並排比對（第 9 部）

- [ ] `PairSummary`：筆數 + 自動欄位配對 + 比較鍵建議
- [ ] `PairColumnStats`：四種方言的聚合 SQL
- [ ] `DiffPair`：hash 策略 → 排序合併策略；暫存檔分頁；匯出 CSV
- [ ] 比較選項（空白、大小寫、NULL、容差、時間精度、排除欄位）
- [ ] 唯讀交易、WHERE 驗證、查詢逾時
- **驗收**：MySQL `users` vs PG `users` 跨庫比對，結果正確；「各欄差異數」能指出問題欄位

## M6　打包發佈（第 10 部）

- [ ] `build/config.yml` 的 App 資訊、`build/appicon.png`、版本注入
- [ ] `wails3 package`：AppImage / `.deb` / `.rpm`、NSIS、`.dmg`
- [ ] GitHub Actions release workflow（三平台 matrix）
- [ ] （選做）`wails3 setup signing`：macOS 簽章公證、Windows 簽章
- [ ] （選做）v3 內建 updater + Ed25519 金鑰
- **驗收**：在 M 系列 Mac、Windows 11、Fedora 三台機器上安裝並完成一次 Schema 比對

## 測試策略

| 層 | 工具 | 範圍 |
|---|---|---|
| 純邏輯 | `go test` | `schema.Diff`、DDL 產生、`normalize`、`AutoMatch`、SQL 字串產生 |
| DB 整合 | `go test -tags integration` + docker compose / testcontainers-go | 每個 dialect 的 introspection 與實際執行 |
| 前端 | Vitest + Testing Library | reducer、hooks、元件互動（mock `api/wails.ts`） |
| E2E | 手動 checklist；進階可參考 Wails v3 官方 E2E 測試指南 | 每個里程碑的驗收項目 |

前端測試時 mock 掉 Wails：

```ts
vi.mock('../api/wails', () => ({
  api: { compareSchemas: vi.fn().mockResolvedValue([{ type: 'added', tableName: 'users', detail: '', sql: 'CREATE ...' }]) },
  errMsg: String,
}))
```

## 原版問題 → SyncCraft 改善總表

| # | 原版問題 | 章節 | SyncCraft 做法 |
|---|---|---|---|
| 1 | DDL 全部是 MySQL 語法 | 5.5 | Dialect 介面，用 target 的方言產生 |
| 2 | `DEFAULT_GENERATED` 寫進 DDL | 4.4 | 正規化時移除 |
| 3 | PG/MSSQL 型別沒有長度 | 4.4 | `format_type` / 組合長度欄位 |
| 4 | 只支援 PG `public`、MSSQL 不分 schema | 4.1 | 加入 schema 層 |
| 5 | 識別字沒跳脫 | 4.7 | 各方言的 `Quote` 跳脫 |
| 6 | DSN 字串拼接 | 3.2 | `mysql.Config` / `url.URL` |
| 7 | 每次呼叫都開關連線 | 3.3 | `Pool` |
| 8 | 整表載入記憶體 | 6.4 | 排序合併 / 分塊 |
| 9 | 值內插 SQL 執行 | 6.3 | 參數化 + 只用於預覽 |
| 10 | 明文密碼 | 7.4 | OS keychain |
| 11 | 分頁沒有排序 | 7.2 | 依主鍵排序 / keyset |
| 12 | CGO sqlite | 10.2 | `modernc.org/sqlite` |
| 13 | 假的進度條、無法取消 | 2.5–2.6 | `app.Event.Emit` + `CancellablePromise` |
| 14 | ADD COLUMN 的 `AFTER` 順序隨機 | 5.3 | 依 Position 排序 |
| 15 | 只能同名表同步 | 9 | 雙表並排比對 |
| 16 | 自寫 updater 無簽章驗證 | 10.9 | v3 內建 updater（Ed25519） |
| 17 | macOS Cmd+C/V 失效（無 Edit 選單） | 10.5 | v3 預設套用應用程式選單 |

祝實作順利 🛠️


# 第 12 部　長期維護：讓專案持續保持最新

一個桌面工具真正花時間的地方，往往是發佈之後的好幾年。SyncCraft 從第一天就把「升級」設計成例行公事，而不是一年一次的大工程。

## 12.1 原則

1. **版本只寫在一個地方**，本機和 CI 讀同一份。
2. **由機器人開升級 PR**，人只負責審核。
3. **CI 把關**：測試通過才敢合併；沒有測試的升級等於賭博。
4. **小步快跑**：每週合併幾個小升級，遠比一年升一次 major 輕鬆。
5. **核心邏輯不依賴框架**：`internal/app` 以外的套件不 import Wails（第 2.8 節），框架大改版時只影響薄薄一層。

## 12.2 版本放在哪裡

| 項目 | 檔案 | 誰會讀 |
|---|---|---|
| wails3 CLI | `mise.toml` | 本機（mise）、CI（`jdx/mise-action`）、Renovate |
| Go 版本 | `go.mod` 的 `go` / `toolchain` 指令 | 本機、CI（`setup-go` 的 `go-version-file`）、Renovate |
| Go 套件（Wails 函式庫、DB driver） | `go.mod` + `go.sum` | 同上 |
| 前端套件 | `frontend/package.json` + `package-lock.json` | npm、CI（`npm ci`）、Renovate |
| GitHub Actions | `.github/workflows/*.yml` | Renovate |
| App 自己的版本 | `build/config.yml` 的 `info.version` | Wails 打包流程 |

> **Wails 有三個地方必須版本一致**：`mise.toml` 的 CLI、`go.mod` 的 `github.com/wailsapp/wails/v3`、`package.json` 的 `@wailsio/runtime`。版本不一致時，產生的綁定和 runtime 可能對不上。`renovate.json` 已經把這三個設成同一組，一定會在同一個 PR 一起升級。

Go 本身的版本建議也寫進 `go.mod`：

```go
module github.com/<you>/synccraft

go 1.26.0          // 最低需求（Wails v3 要求）
toolchain go1.27.4 // 建議使用的版本；本機較舊時 go 指令會自動下載
```

## 12.3 Renovate：自動開升級 PR

### 為什麼選 Renovate

| | Renovate | Dependabot |
|---|---|---|
| 平台 | GitHub、Gitea、GitLab… | 只有 GitHub |
| `mise.toml` | ✅ | ❌ |
| `go.mod`、npm、Actions | ✅ | ✅ |
| 把多個套件併成一個 PR | 彈性很高 | 有限 |
| `go mod tidy` | ✅ `postUpdateOptions` | ✅ |

### 啟用（GitHub）

1. 安裝 [Renovate GitHub App](https://github.com/apps/renovate)，授權給 SyncCraft repo。
2. repo 根目錄已經有 `renovate.json`，Renovate 會直接採用。
3. 第一次會開一個 **Dependency Dashboard** issue，列出所有可升級項目；之後每週一早上 9 點前開 PR。

### SyncCraft 的設定重點（`renovate.json`）

| 規則 | 行為 |
|---|---|
| 排程 | 每週一早上（台北時間）；安全性更新隨時 |
| 預設 | 穩定版（≥ 1.0）的 patch / minor：**CI 通過後自動合併** |
| major | 獨立 PR、加 `major` 標籤、手動合併 |
| `wails` 群組 | CLI + Go 函式庫 + `@wailsio/runtime` 同一個 PR，**一律手動** |
| `database drivers` 群組 | mysql / pgx / mssql / sqlite，**一律手動**（要跑整合測試） |
| `react`、`vite`、`github actions` | 各自成組，減少 PR 數量 |
| `0.x` 套件 | 不自動合併（語意化版本裡 0.x 的 minor 可能有破壞性變更） |
| 每月 | 重新產生 lock file（`lockFileMaintenance`），帶入間接相依的修正 |
| 漏洞 | 使用 OSV 資料庫，有已知漏洞的套件會立即開 PR |

驗證設定檔：

```bash
npx -y --package renovate -- renovate-config-validator --strict renovate.json
```

> **自動合併的前提**：GitHub 的 branch protection 要求 CI 必須通過（Settings → Branches → Require status checks → 勾選 `check`、`build`）。否則 Renovate 可能在 CI 還沒跑完前就合併。

## 12.4 CI：每個 PR 都要通過的檢查

`.github/workflows/ci.yml` 分兩個 job：

**check（Ubuntu）**

| 步驟 | 目的 |
|---|---|
| 安裝 GTK4 / WebKitGTK 6.0 | `wails3` CLI 編譯時就需要，所以排在最前面 |
| `setup-go`（讀 `go.mod`）→ `mise-action`（讀 `mise.toml`） | 版本與本機完全一致 |
| `npm ci` | 依照 lock file 安裝，可重現 |
| `wails3 generate bindings -clean=true -ts -i` + `git diff --exit-code` | **綁定必須與 Go 程式碼同步**；忘了提交就會失敗 |
| `npm run build` | TypeScript 型別檢查 + 產生 `frontend/dist`（`go:embed` 需要） |
| `go vet`、`go test -race` | 靜態檢查、單元測試、競態偵測 |
| `govulncheck` | Go 官方漏洞掃描，只報告程式**實際呼叫到**的漏洞函式 |
| `npm audit --omit=dev` | 前端正式相依的漏洞 |

**build（Linux / Windows / macOS）**

三平台各跑一次 `wails3 build`。升級某個套件後，常見的狀況是「Linux 好好的、Windows 編不過」，這一步就是為了抓這種問題。

### DB 整合測試（M2 之後加入）

Driver 升級最怕 SQL 行為改變。用 GitHub Actions 的 `services` 起資料庫：

```yaml
  integration:
    runs-on: ubuntu-24.04
    services:
      mysql:
        image: mysql:8.4
        env: { MYSQL_ROOT_PASSWORD: root, MYSQL_DATABASE: shop }
        ports: ["3306:3306"]
        options: --health-cmd="mysqladmin ping" --health-interval=5s --health-retries=20
      postgres:
        image: postgres:17
        env: { POSTGRES_PASSWORD: postgres, POSTGRES_DB: shop }
        ports: ["5432:5432"]
        options: --health-cmd="pg_isready" --health-interval=5s --health-retries=20
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with: { go-version-file: go.mod }
      - run: go test -tags integration ./internal/dialect/...
```

## 12.5 升級節奏

| 項目 | 節奏 | 做法 |
|---|---|---|
| patch / minor | 每週 | Renovate 自動合併 |
| Go | 每年 2 月、8 月出新版 | 新版出來後 1～2 個月內升級；保持在官方仍支援的最新兩版之內 |
| Node | 每年 4 月出新的偶數版 | 只用 **偶數 LTS**；進入 Active LTS 後再升 |
| React / Vite / TypeScript major | 出版時 | 讀官方遷移指南 → 開獨立分支 → CI 綠燈 → 手動測試主要畫面 |
| Wails minor（3.x） | 出版時 | 讀 release notes；**特別注意 Taskfile / `build/` 的變更**（見下節） |
| DB driver | 每月檢查 | 跑整合測試後手動合併 |
| 漏洞修正 | 立即 | Renovate 安全性 PR 優先處理 |

### Wails 升級的特別步驟

Wails 的 `build/` 目錄是**產生出來後提交進 repo** 的，升級 CLI 不會自動更新這些檔案。升 Wails minor 版時：

```bash
git switch -c chore/wails-3.x
# Renovate 已經改好 mise.toml、go.mod、package.json
mise install
wails3 task common:update:build-assets    # 用新版範本重新產生 build/ 底下的資源
git diff build/                            # 逐一檢查；把你自己的客製化改回來
wails3 generate bindings -clean=true -ts -i
wails3 dev                                 # 實際操作主要畫面
```

升級前先讀 `wails3 releasenotes`。

## 12.6 手動檢查指令

不想等 Renovate 時：

```bash
mise outdated                              # 工具鏈（包含 wails3）
go list -m -u all | grep '\['              # 有新版的 Go 套件
go get -u ./... && go mod tidy             # 全部升到最新 minor/patch
cd frontend && npx npm-check-updates       # 前端套件
cd frontend && npx npm-check-updates -u --target minor && npm install
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

## 12.7 授權（License）

SyncCraft 參考了 SyncForge 的原始碼，SyncForge 使用 **MIT License**。MIT 允許修改、再發佈、商用，條件只有一個：**保留原作者的版權聲明與授權條文**。

| | MIT | Apache-2.0 |
|---|---|---|
| 條文長度 | 很短 | 長 |
| 專利授權 | 無明文 | **有**：貢獻者授予使用者專利權，對方提告就失去授權 |
| 修改標示 | 不要求 | 修改過的檔案要標註 |
| NOTICE 檔 | 無 | 有 NOTICE 檔時，再發佈必須保留 |
| 與上游 SyncForge 一致 | ✅ | 不同，但相容（MIT 程式碼可以併入 Apache-2.0 專案） |
| 常見於 | 小型工具、前端生態 | 大型專案、企業背景專案（Kubernetes、Go 生態常見） |

不論選哪一個，都要：

1. 根目錄放 `LICENSE`（你選的授權）。
2. **保留 SyncForge 的 MIT 聲明**：放在 `THIRD_PARTY_NOTICES.md`（或選 MIT 時直接寫在同一個 `LICENSE` 裡，列兩行版權）。
3. README 寫明「本專案改寫自 nanablast/syncforge」。
4. 打包的 App 裡附上相依套件的授權清單（各 driver、React 等多半是 MIT / BSD / Apache-2.0，都允許商用）。

> 想讓 Renovate 引進新套件時也檢查授權，可以在 CI 加 `go-licenses`（Go）與 `license-checker`（npm）。

## 12.8 發佈到 GitHub 的檢查清單

- [ ] `go.mod` 的 module 路徑 = `github.com/<帳號>/synccraft`（要在 `wails3 init` 時就設定好，第 1.3 節）
- [ ] `LICENSE`、`THIRD_PARTY_NOTICES.md`、`README.md`
- [ ] 安裝 Renovate App
- [ ] Branch protection：`main` 要求 PR、要求 `check` 與 `build` 通過
- [ ] Settings → Code security：開啟 Dependabot alerts（只通知，不開 PR；PR 交給 Renovate）
- [ ] 第一個 tag `v0.1.0` → release workflow（第 10.10 節）產出三平台檔案
- [ ] （之後）`wails3 updater genkey`，私鑰存進 repo secrets

## 練習

1. 在 GitHub 建立 repo、推上去、安裝 Renovate，觀察 Dependency Dashboard。
2. 手動把 `frontend/package.json` 的 `vite` 降一個 minor 版，看 Renovate 是否開 PR、CI 通過後自動合併。
3. 故意改一個 Service 方法的簽章但不重新產生綁定就 push，確認 CI 的「Bindings are up to date」步驟會失敗。


