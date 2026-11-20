# dev

本機測試用資料庫。來源（src）與目標（dst）的結構和資料**故意設計成不同**，用來驗證比對結果。

## 啟動

```bash
podman compose -f dev/docker-compose.yml up -d     # 或 docker compose（需安裝 compose 外掛）
podman compose -f dev/docker-compose.yml down -v   # 連同資料一起刪除；下次 up 會重新匯入種子
```

`seed/<服務名稱>/*.sql` 會在容器**第一次**啟動時自動匯入（`docker-entrypoint-initdb.d`）。修改種子後要 `down -v` 才會重新匯入。

## 連線資訊

| 服務 | 位址 | 帳號 / 密碼 | 資料庫 |
|---|---|---|---|
| mysql-src | 127.0.0.1:3306 | root / root | shop |
| mysql-dst | 127.0.0.1:3307 | root / root | shop |
| pg-src | 127.0.0.1:5432 | postgres / postgres | shop |
| pg-dst | 127.0.0.1:5433 | postgres / postgres | shop |
| mssql | 127.0.0.1:1433 | sa / Passw0rd! | （無種子） |

## 種子資料的預期差異

`users` 表，src → dst：

| 類型 | 預期結果 |
|---|---|
| 結構 | `MODIFY email`（varchar 255 vs 100）、`ADD nickname`、`ADD created_at`、`ADD INDEX idx_email` |
| 資料 | `INSERT` id=3、`UPDATE` id=2（email 不同）、`DELETE` id=9 |

`pg-src` 另有大小寫混合名稱的表 `"UserLog"`，用來測試識別字加引號；dst 沒有這張表，結構比對應出現「新增資料表」。
