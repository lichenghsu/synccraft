# 開發進度

| 里程碑 | 內容 | 狀態 |
|---|---|---|
| M0 | 專案骨架：Wails v3 + React/TS、Service 範例、hooks、測試資料庫、Renovate、CI | ✅ 完成 |
| M1 | 連線管理：連線池、四種 driver、結構化 DSN、密碼存入系統 keychain | ⏳ 未開始 |
| M2 | Introspection 與資料表瀏覽：Dialect 介面、四種資料庫的 metadata 查詢 | ⏳ 未開始 |
| M3 | 結構比對：比對演算法、依目標方言產生 DDL、交易執行 | ⏳ 未開始 |
| M4 | 資料同步：參數化 DML、值正規化、事件進度與取消、大表串流比對 | ⏳ 未開始 |
| M5 | 雙表並排比對：筆數、欄位對應、逐列差異、欄位統計、匯出 CSV | ⏳ 未開始 |
| M6 | 打包發佈：三平台安裝檔、release workflow、簽章、自動更新 | ⏳ 未開始 |

## M0 完成項目

- [x] `wails3 init -t react`，升級至 React 19 / TypeScript 7 / Vite 8
- [x] `@bindings`、`@events` 路徑別名
- [x] `internal/app/HelloService`：呼叫與錯誤、可取消呼叫、有型別的事件（含單元測試）
- [x] hooks：`useAsync`、`useCancellable`、`useWailsEvent`
- [x] `dev/docker-compose.yml` 與種子資料
- [x] `renovate.json`、`.github/workflows/ci.yml`
- [x] LICENSE（MIT）、README（英文 / 繁體中文）
- [ ] GitHub：安裝 Renovate App、設定 `main` 分支保護
