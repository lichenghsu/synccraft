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
