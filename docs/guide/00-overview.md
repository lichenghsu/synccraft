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
