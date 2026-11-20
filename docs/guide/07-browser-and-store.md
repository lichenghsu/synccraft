# 第 7 部　資料表瀏覽與連線儲存

## 7.1 表瀏覽的三個 API

| Go 方法 | 用途 | 內部 |
|---|---|---|
| `GetAllTables(cfg)` | 左側表清單＋筆數 | 直接呼叫 `GetTablesForSync`（每表 PK、欄位、`COUNT(*)`） |
| `GetTableStructure(cfg, table)` | 結構分頁（欄位、索引、CREATE） | 依 DB 呼叫 `getXxxTableInfo`（第 4 部） |
| `GetTableData(cfg, table, page, pageSize)` | 資料分頁 | 見下 |

⚠️ `GetAllTables` 對每張表都跑 3 條查詢（PK、欄位、COUNT），200 張表就是 600 次往返。SyncCraft：清單只要表名＋估計筆數（第 4.8 節的單一查詢），點進去才載入細節。

## 7.2 分頁 SQL

原版：

```go
switch dbType {
case SQLServer:
    query = fmt.Sprintf("SELECT %s FROM %s ORDER BY (SELECT NULL) OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", cols, table, offset, size)
default:
    query = fmt.Sprintf("SELECT %s FROM %s LIMIT %d OFFSET %d", cols, table, size, offset)
}
```

- SQL Server 的 `OFFSET ... FETCH` **必須搭配 ORDER BY**（2012+），`ORDER BY (SELECT NULL)` 是「隨便排」的慣用寫法。
- ⚠️ 不指定排序時，**任何 DB 都不保證分頁結果穩定**——第 2 頁可能出現第 1 頁看過的列。

### SyncCraft：依主鍵排序

```sql
-- 有主鍵
SELECT ... FROM t ORDER BY id LIMIT 50 OFFSET 100;
SELECT ... FROM [t] ORDER BY [id] OFFSET 100 ROWS FETCH NEXT 50 ROWS ONLY;
```

### 進階：Keyset（seek）分頁

`OFFSET 1000000` 仍要掃過前一百萬列。資料量大時改用「從上一頁最後一筆往後找」：

```sql
-- 下一頁：last_id 為上一頁最後一列的 id
SELECT ... FROM users WHERE id > ? ORDER BY id LIMIT 50;

-- 複合主鍵（MySQL / PG / SQLite 支援 row value 比較）
SELECT ... FROM t WHERE (a, b) > (?, ?) ORDER BY a, b LIMIT 50;
```

缺點是不能「直接跳到第 N 頁」。UI 可以兩者並用：前幾頁 OFFSET，捲動載入用 keyset。

### 回傳前的型別處理

```go
type TableDataResult struct {
    Columns    []string       `json:"columns"`
    Rows       []TableRowData `json:"rows"`     // TableRowData{ Values map[string]any }
    TotalCount int            `json:"totalCount"`
    Page, PageSize int
}
```

⚠️ 用 `map` 回傳每列 → JSON 物件的鍵順序不保證；前端一定要用 `columns` 陣列決定欄位順序。SyncCraft 可改成 `Rows [][]any`（陣列比 map 小很多，大量資料傳輸更快）。

## 7.3 連線儲存（原版）

```go
type SavedConnection struct {
    Name   string           `json:"name"`
    Config ConnectionConfig `json:"config"`   // 含明文密碼
}
// 檔案：~/.syncforge/connections.json，權限 0600
```

- 讀寫用 `sync.RWMutex` 保護（Bind 方法會在多個 goroutine 同時被呼叫）。
- `Save` 以名稱為 key：同名覆蓋、不同名新增。
- 寫檔權限 `0600`（只有本人可讀）——在 Windows 上這個權限位元基本無效。

### 問題

1. **明文密碼**：備份、同步雲端硬碟、截圖都可能洩漏。
2. 存在 `~/.syncforge` 而非各平台慣例的設定資料夾。
3. 寫檔不是原子操作，寫到一半當機會毀掉整個檔案。

## 7.4 SyncCraft：系統金鑰圈 + 慣例路徑 + 原子寫入

### 設定檔位置

```go
dir, _ := os.UserConfigDir()
// macOS:   ~/Library/Application Support
// Windows: %AppData%  (C:\Users\me\AppData\Roaming)
// Linux:   ~/.config  (或 $XDG_CONFIG_HOME)
path := filepath.Join(dir, "SyncCraft", "connections.json")
```

### 密碼放進 OS keychain

使用 `github.com/zalando/go-keyring`（純 Go，三平台都支援）：

| 平台 | 後端 |
|---|---|
| macOS | Keychain |
| Windows | Credential Manager |
| Linux | Secret Service（GNOME Keyring / KWallet，經 D-Bus） |

```go
import "github.com/zalando/go-keyring"

const service = "SyncCraft"

func (s *Store) Save(conn SavedConnection) error {
    if err := keyring.Set(service, conn.ID, conn.Config.Password); err != nil {
        return fmt.Errorf("keyring: %w", err)
    }
    conn.Config.Password = ""          // JSON 裡不存密碼
    return s.upsertAndWrite(conn)
}

func (s *Store) Resolve(id string) (ConnectionConfig, error) {
    conn := s.find(id)
    pw, err := keyring.Get(service, id)
    if err != nil && !errors.Is(err, keyring.ErrNotFound) { return conn.Config, err }
    conn.Config.Password = pw
    return conn.Config, nil
}
```

> Linux 沒有 Secret Service（例如純 Hyprland 未裝 gnome-keyring）時 `keyring.Set` 會失敗——要提供退路：不存密碼、每次詢問。

### 原子寫入

```go
func writeAtomic(path string, data []byte) error {
    tmp, err := os.CreateTemp(filepath.Dir(path), ".connections-*.json")
    if err != nil { return err }
    if _, err := tmp.Write(data); err != nil { tmp.Close(); os.Remove(tmp.Name()); return err }
    if err := tmp.Sync(); err != nil { tmp.Close(); os.Remove(tmp.Name()); return err }
    tmp.Close()
    return os.Rename(tmp.Name(), path)   // 同一檔案系統上 rename 是原子的
}
```

### 前端永遠拿不到密碼

`GetSavedConnections()` 回傳的清單不含密碼；前端只傳 `connectionId` 給 Go，Go 端自己 `Resolve`。這也讓 DevTools 裡看不到密碼。

## 7.5 原版的自動更新（簡介）

`updater/updater.go`：

1. `GET https://api.github.com/repos/<owner>/<repo>/releases/latest` 取最新 tag。
2. 與內建版本比較（`compareVersions`）。
3. 依 `runtime.GOOS/GOARCH` 找對應 asset 名稱並下載。
4. 替換自身：macOS 解壓 `.app` 覆蓋、Windows 用批次檔在程式結束後改名、Linux 直接覆蓋並 `chmod +x`。

SyncCraft 不自己寫這段：Wails v3 **內建 updater**，支援 GitHub Releases、Ed25519 簽章驗證、原子替換與回滾（第 10.9 節）。原版沒有驗證下載檔案，等於讓任何能竄改下載內容的人在你電腦上執行程式。

## 練習

1. 實作 `GetTableData` 依主鍵排序的版本，無主鍵時退回原版行為並在 UI 顯示警告。
2. 實作 Store：`os.UserConfigDir` + `go-keyring` + 原子寫入；在 Fedora 上確認 GNOME Keyring（`secret-tool search service SyncCraft`）裡看得到。
3. 讓 `GetSavedConnections` 不回傳密碼，`TestConnection` 改收 `connectionId` 或完整設定兩種形式。
