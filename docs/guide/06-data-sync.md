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
