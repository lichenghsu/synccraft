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
