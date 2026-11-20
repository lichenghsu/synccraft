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
