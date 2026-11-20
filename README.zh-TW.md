# SyncCraft

[English](README.md) | **繁體中文**

跨平台的資料庫 **結構比對 / 資料同步 / 雙表並排比對** 桌面工具。
支援 MySQL、PostgreSQL、SQL Server、SQLite；可在 macOS（Apple Silicon / Intel）、Windows、Linux 執行。

> 本專案改寫自 [nanablast/syncforge](https://github.com/nanablast/syncforge)（MIT License），
> 改用 **Wails v3 + React + TypeScript**，並重新設計資料庫方言層、連線池、密碼儲存與大表比對。

## 功能

- **連線管理**：四種資料庫，密碼存入系統 keychain
- **資料表瀏覽**：結構、索引、`CREATE` 語法、分頁資料
- **結構比對**：依*目標*資料庫的方言產生 DDL
- **資料同步**：依主鍵逐列比對、參數化 DML、交易
- **雙表並排比對**：任意兩張表（跨主機 / 跨資料庫種類）比對筆數、欄位對應、逐列差異、欄位統計

開發進度見 [`docs/progress.md`](docs/progress.md)。

## 開發

### 需求

- Go ≥ 1.26、Node.js（偶數 LTS）
- [mise](https://mise.jdx.dev)：安裝本專案固定版本的 `wails3` CLI
- 各平台系統相依：
  - **Linux**：`gtk4-devel webkitgtk6.0-devel`（Fedora）／`libgtk-4-dev libwebkitgtk-6.0-dev`（Ubuntu 24.04+）
  - **macOS**：`xcode-select --install`
  - **Windows**：WebView2 Runtime（Windows 11 內建）

### 開始

```bash
mise install          # 安裝 wails3（版本固定在 mise.toml）
wails3 doctor         # 檢查系統相依
wails3 dev            # 開發模式：前端熱更新、Go 自動重編
```

### 常用指令

```bash
wails3 build                                   # 正式建置 → bin/synccraft
wails3 package                                 # 產生目前平台的安裝檔
GOOS=windows wails3 build                      # 交叉編譯 Windows 版
wails3 generate bindings -clean=true -ts -i    # 重新產生前端綁定
go test ./...                                  # Go 測試
podman compose -f dev/docker-compose.yml up -d # 本機測試資料庫
```

### 專案結構

```
main.go               # Wails 進入點
internal/app/         # Wails Service（唯一 import Wails 的套件）
frontend/             # React + TypeScript + Vite
  bindings/           # 由 Go 自動產生（請勿手改，需提交）
  src/api/            # 包裝 Go 綁定
  src/hooks/          # 共用 React hooks
build/                # 各平台建置設定、圖示、Taskfile
dev/                  # 本機測試資料庫與種子資料
docs/                 # 開發進度
```

各模組的功能說明放在該資料夾的 `README.md`（例如 [`internal/app`](internal/app/README.md)、[`frontend/src/hooks`](frontend/src/hooks/README.md)）；程式碼內只保留必要的英文註解。

## 維護

- 相依套件由 [Renovate](https://docs.renovatebot.com) 自動開 PR（設定：`renovate.json`）
- 每個 PR 由 GitHub Actions 檢查：綁定同步、型別、測試、漏洞掃描、三平台建置（`.github/workflows/ci.yml`）

## 授權

[MIT](LICENSE)。保留原專案 SyncForge 的著作權聲明。
