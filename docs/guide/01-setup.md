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
