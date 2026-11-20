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
