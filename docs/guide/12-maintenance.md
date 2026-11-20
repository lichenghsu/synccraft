# 第 12 部　長期維護：讓專案持續保持最新

一個桌面工具真正花時間的地方，往往是發佈之後的好幾年。SyncCraft 從第一天就把「升級」設計成例行公事，而不是一年一次的大工程。

## 12.1 原則

1. **版本只寫在一個地方**，本機和 CI 讀同一份。
2. **由機器人開升級 PR**，人只負責審核。
3. **CI 把關**：測試通過才敢合併；沒有測試的升級等於賭博。
4. **小步快跑**：每週合併幾個小升級，遠比一年升一次 major 輕鬆。
5. **核心邏輯不依賴框架**：`internal/app` 以外的套件不 import Wails（第 2.8 節），框架大改版時只影響薄薄一層。

## 12.2 版本放在哪裡

| 項目 | 檔案 | 誰會讀 |
|---|---|---|
| wails3 CLI | `mise.toml` | 本機（mise）、CI（`jdx/mise-action`）、Renovate |
| Go 版本 | `go.mod` 的 `go` / `toolchain` 指令 | 本機、CI（`setup-go` 的 `go-version-file`）、Renovate |
| Go 套件（Wails 函式庫、DB driver） | `go.mod` + `go.sum` | 同上 |
| 前端套件 | `frontend/package.json` + `package-lock.json` | npm、CI（`npm ci`）、Renovate |
| GitHub Actions | `.github/workflows/*.yml` | Renovate |
| App 自己的版本 | `build/config.yml` 的 `info.version` | Wails 打包流程 |

> **Wails 有三個地方必須版本一致**：`mise.toml` 的 CLI、`go.mod` 的 `github.com/wailsapp/wails/v3`、`package.json` 的 `@wailsio/runtime`。版本不一致時，產生的綁定和 runtime 可能對不上。`renovate.json` 已經把這三個設成同一組，一定會在同一個 PR 一起升級。

Go 本身的版本建議也寫進 `go.mod`：

```go
module github.com/<you>/synccraft

go 1.26.0          // 最低需求（Wails v3 要求）
toolchain go1.27.4 // 建議使用的版本；本機較舊時 go 指令會自動下載
```

## 12.3 Renovate：自動開升級 PR

### 為什麼選 Renovate

| | Renovate | Dependabot |
|---|---|---|
| 平台 | GitHub、Gitea、GitLab… | 只有 GitHub |
| `mise.toml` | ✅ | ❌ |
| `go.mod`、npm、Actions | ✅ | ✅ |
| 把多個套件併成一個 PR | 彈性很高 | 有限 |
| `go mod tidy` | ✅ `postUpdateOptions` | ✅ |

### 啟用（GitHub）

1. 安裝 [Renovate GitHub App](https://github.com/apps/renovate)，授權給 SyncCraft repo。
2. repo 根目錄已經有 `renovate.json`，Renovate 會直接採用。
3. 第一次會開一個 **Dependency Dashboard** issue，列出所有可升級項目；之後每週一早上 9 點前開 PR。

### SyncCraft 的設定重點（`renovate.json`）

| 規則 | 行為 |
|---|---|
| 排程 | 每週一早上（台北時間）；安全性更新隨時 |
| 預設 | 穩定版（≥ 1.0）的 patch / minor：**CI 通過後自動合併** |
| major | 獨立 PR、加 `major` 標籤、手動合併 |
| `wails` 群組 | CLI + Go 函式庫 + `@wailsio/runtime` 同一個 PR，**一律手動** |
| `database drivers` 群組 | mysql / pgx / mssql / sqlite，**一律手動**（要跑整合測試） |
| `react`、`vite`、`github actions` | 各自成組，減少 PR 數量 |
| `0.x` 套件 | 不自動合併（語意化版本裡 0.x 的 minor 可能有破壞性變更） |
| 每月 | 重新產生 lock file（`lockFileMaintenance`），帶入間接相依的修正 |
| 漏洞 | 使用 OSV 資料庫，有已知漏洞的套件會立即開 PR |

驗證設定檔：

```bash
npx -y --package renovate -- renovate-config-validator --strict renovate.json
```

> **自動合併的前提**：GitHub 的 branch protection 要求 CI 必須通過（Settings → Branches → Require status checks → 勾選 `check`、`build`）。否則 Renovate 可能在 CI 還沒跑完前就合併。

## 12.4 CI：每個 PR 都要通過的檢查

`.github/workflows/ci.yml` 分兩個 job：

**check（Ubuntu）**

| 步驟 | 目的 |
|---|---|
| 安裝 GTK4 / WebKitGTK 6.0 | `wails3` CLI 編譯時就需要，所以排在最前面 |
| `setup-go`（讀 `go.mod`）→ `mise-action`（讀 `mise.toml`） | 版本與本機完全一致 |
| `npm ci` | 依照 lock file 安裝，可重現 |
| `wails3 generate bindings -clean=true -ts -i` + `git diff --exit-code` | **綁定必須與 Go 程式碼同步**；忘了提交就會失敗 |
| `npm run build` | TypeScript 型別檢查 + 產生 `frontend/dist`（`go:embed` 需要） |
| `go vet`、`go test -race` | 靜態檢查、單元測試、競態偵測 |
| `govulncheck` | Go 官方漏洞掃描，只報告程式**實際呼叫到**的漏洞函式 |
| `npm audit --omit=dev` | 前端正式相依的漏洞 |

**build（Linux / Windows / macOS）**

三平台各跑一次 `wails3 build`。升級某個套件後，常見的狀況是「Linux 好好的、Windows 編不過」，這一步就是為了抓這種問題。

### DB 整合測試（M2 之後加入）

Driver 升級最怕 SQL 行為改變。用 GitHub Actions 的 `services` 起資料庫：

```yaml
  integration:
    runs-on: ubuntu-24.04
    services:
      mysql:
        image: mysql:8.4
        env: { MYSQL_ROOT_PASSWORD: root, MYSQL_DATABASE: shop }
        ports: ["3306:3306"]
        options: --health-cmd="mysqladmin ping" --health-interval=5s --health-retries=20
      postgres:
        image: postgres:17
        env: { POSTGRES_PASSWORD: postgres, POSTGRES_DB: shop }
        ports: ["5432:5432"]
        options: --health-cmd="pg_isready" --health-interval=5s --health-retries=20
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with: { go-version-file: go.mod }
      - run: go test -tags integration ./internal/dialect/...
```

## 12.5 升級節奏

| 項目 | 節奏 | 做法 |
|---|---|---|
| patch / minor | 每週 | Renovate 自動合併 |
| Go | 每年 2 月、8 月出新版 | 新版出來後 1～2 個月內升級；保持在官方仍支援的最新兩版之內 |
| Node | 每年 4 月出新的偶數版 | 只用 **偶數 LTS**；進入 Active LTS 後再升 |
| React / Vite / TypeScript major | 出版時 | 讀官方遷移指南 → 開獨立分支 → CI 綠燈 → 手動測試主要畫面 |
| Wails minor（3.x） | 出版時 | 讀 release notes；**特別注意 Taskfile / `build/` 的變更**（見下節） |
| DB driver | 每月檢查 | 跑整合測試後手動合併 |
| 漏洞修正 | 立即 | Renovate 安全性 PR 優先處理 |

### Wails 升級的特別步驟

Wails 的 `build/` 目錄是**產生出來後提交進 repo** 的，升級 CLI 不會自動更新這些檔案。升 Wails minor 版時：

```bash
git switch -c chore/wails-3.x
# Renovate 已經改好 mise.toml、go.mod、package.json
mise install
wails3 task common:update:build-assets    # 用新版範本重新產生 build/ 底下的資源
git diff build/                            # 逐一檢查；把你自己的客製化改回來
wails3 generate bindings -clean=true -ts -i
wails3 dev                                 # 實際操作主要畫面
```

升級前先讀 `wails3 releasenotes`。

## 12.6 手動檢查指令

不想等 Renovate 時：

```bash
mise outdated                              # 工具鏈（包含 wails3）
go list -m -u all | grep '\['              # 有新版的 Go 套件
go get -u ./... && go mod tidy             # 全部升到最新 minor/patch
cd frontend && npx npm-check-updates       # 前端套件
cd frontend && npx npm-check-updates -u --target minor && npm install
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

## 12.7 授權（License）

SyncCraft 參考了 SyncForge 的原始碼，SyncForge 使用 **MIT License**。MIT 允許修改、再發佈、商用，條件只有一個：**保留原作者的版權聲明與授權條文**。

| | MIT | Apache-2.0 |
|---|---|---|
| 條文長度 | 很短 | 長 |
| 專利授權 | 無明文 | **有**：貢獻者授予使用者專利權，對方提告就失去授權 |
| 修改標示 | 不要求 | 修改過的檔案要標註 |
| NOTICE 檔 | 無 | 有 NOTICE 檔時，再發佈必須保留 |
| 與上游 SyncForge 一致 | ✅ | 不同，但相容（MIT 程式碼可以併入 Apache-2.0 專案） |
| 常見於 | 小型工具、前端生態 | 大型專案、企業背景專案（Kubernetes、Go 生態常見） |

不論選哪一個，都要：

1. 根目錄放 `LICENSE`（你選的授權）。
2. **保留 SyncForge 的 MIT 聲明**：放在 `THIRD_PARTY_NOTICES.md`（或選 MIT 時直接寫在同一個 `LICENSE` 裡，列兩行版權）。
3. README 寫明「本專案改寫自 nanablast/syncforge」。
4. 打包的 App 裡附上相依套件的授權清單（各 driver、React 等多半是 MIT / BSD / Apache-2.0，都允許商用）。

> 想讓 Renovate 引進新套件時也檢查授權，可以在 CI 加 `go-licenses`（Go）與 `license-checker`（npm）。

## 12.8 發佈到 GitHub 的檢查清單

- [ ] `go.mod` 的 module 路徑 = `github.com/<帳號>/synccraft`（要在 `wails3 init` 時就設定好，第 1.3 節）
- [ ] `LICENSE`、`THIRD_PARTY_NOTICES.md`、`README.md`
- [ ] 安裝 Renovate App
- [ ] Branch protection：`main` 要求 PR、要求 `check` 與 `build` 通過
- [ ] Settings → Code security：開啟 Dependabot alerts（只通知，不開 PR；PR 交給 Renovate）
- [ ] 第一個 tag `v0.1.0` → release workflow（第 10.10 節）產出三平台檔案
- [ ] （之後）`wails3 updater genkey`，私鑰存進 repo secrets

## 練習

1. 在 GitHub 建立 repo、推上去、安裝 Renovate，觀察 Dependency Dashboard。
2. 手動把 `frontend/package.json` 的 `vite` 降一個 minor 版，看 Renovate 是否開 PR、CI 通過後自動合併。
3. 故意改一個 Service 方法的簽章但不重新產生綁定就 push，確認 CI 的「Bindings are up to date」步驟會失敗。
