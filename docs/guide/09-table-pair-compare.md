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
