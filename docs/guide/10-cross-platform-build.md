# 第 10 部　跨平台打包：macOS（M 系列）／ Windows ／ Linux

## 10.1 v3 的建置系統：Taskfile

v2 的 `wails build` 是一個黑盒子；v3 改用 [Task](https://taskfile.dev)，所有步驟都寫在專案裡的 YAML，**看得到、改得了**：

```
Taskfile.yml                 # 根：build / package / run 依 GOOS 分派
build/Taskfile.yml           # 共用：go mod tidy、前端安裝與建置、產生綁定、產生圖示
build/darwin/Taskfile.yml    # macOS：build、build:universal、package、create:dmg、sign、sign:notarize
build/windows/Taskfile.yml   # Windows：build、package、create:nsis:installer、create:msix:package、sign
build/linux/Taskfile.yml     # Linux：build、package、create:appimage / deb / rpm / aur、sign:packages
build/docker/                # 交叉編譯用的 Dockerfile
build/config.yml             # App 資訊（名稱、版本、識別碼…）與 dev 模式設定
```

常用指令：

```bash
wails3 build                       # 建置目前平台 → bin/synccraft
wails3 package                     # 建置 + 產生該平台的安裝檔
GOOS=windows wails3 build          # 指定目標平台
wails3 task --list                 # 列出所有可用任務
wails3 task darwin:package:universal
```

`wails3 build` / `wails3 package` 其實就是執行根 Taskfile 的 `build` / `package`，它再依 `GOOS` 呼叫 `build/<os>/Taskfile.yml` 裡的對應任務。想加步驟（例如注入版本號、先跑測試）就直接改 YAML。

## 10.2 App 資訊：build/config.yml

```yaml
info:
  companyName: "SyncCraft"
  productName: "SyncCraft"
  productIdentifier: "dev.synccraft.app"   # macOS bundle id / Windows 識別碼，發佈後不要再改
  description: "Database schema & data compare tool"
  copyright: "© 2026 SyncCraft contributors"
  version: "0.1.0"
```

改完後要重新產生各平台的資源檔（Info.plist、Windows 版本資訊、`.desktop` 等）：

```bash
wails3 task common:update:build-assets
```

> ⚠️ 這個指令會**覆蓋** `build/` 底下你手動改過的平台檔案。有客製化的話，先 commit 再執行，然後用 `git diff` 檢查。

圖示：替換 `build/appicon.png`（1024×1024），建置時會自動產生 `.icns`、`.ico` 等格式。

## 10.3 第一個大坑：mattn/go-sqlite3 需要 CGO

原版使用 `github.com/mattn/go-sqlite3`，它是 C 版 SQLite 的包裝，需要 CGO：

- 從 Linux 交叉編譯 Windows 版 → 需要 MinGW 或 Docker
- 在 Windows 本機建置 → 需要安裝 gcc

**SyncCraft 改用純 Go 的 SQLite**：

```go
import _ "modernc.org/sqlite"      // driver 名稱是 "sqlite"（不是 "sqlite3"）

db, err := sql.Open("sqlite", "/path/to/file.db")
```

| | mattn/go-sqlite3 | modernc.org/sqlite |
|---|---|---|
| CGO | 需要 | 不需要 |
| 效能 | 較快 | 稍慢（比對工具感覺不出差異） |
| 交叉編譯 | 困難 | 直接過 |

其餘三個 driver（mysql、pgx、mssql）都是純 Go。v3 的 Windows Taskfile 預設 `CGO_ENABLED=0`，換掉 SQLite 之後，**`GOOS=windows wails3 build` 可以直接在你的 Fedora 上產出 Windows 版**，不需要 Docker。

## 10.4 各平台能從哪裡建置

| 目標 | 在 Linux 上 | 在 macOS 上 | 在 Windows 上 | 備註 |
|---|---|---|---|---|
| Linux | ✅ 原生 | 🐳 Docker | 🐳 Docker | 需要 CGO（GTK） |
| Windows | ✅ 原生（CGO=0） | ✅ 原生 | ✅ 原生 | 前提是沒有其他 CGO 套件 |
| macOS | 🐳 Docker（未簽章） | ✅ 原生 | 🐳 Docker | **簽章與公證必須在 Mac 上做** |

Docker 交叉編譯要先建 image：

```bash
wails3 task common:setup:docker   # 建立 wails-cross image（只需一次）
```

> 實務建議：**本機只建 Linux 與 Windows**；macOS 版交給 CI 的 `macos-latest`（Apple Silicon 機器），或你自己的 Mac。

## 10.5 macOS（Apple Silicon M1–M4）

```bash
wails3 build                               # 本機架構（M 系列 = arm64）
ARCH=amd64 wails3 build                    # Intel Mac
wails3 task darwin:build:universal         # 一個 App 同時支援兩種架構
wails3 package                             # → bin/SyncCraft.app
wails3 task darwin:package:dmg             # → .dmg
```

### Gatekeeper：未簽章的 App

從網路下載的未簽章 App，打開時會出現「已損毀」或「無法驗證開發者」。

- **自己用 / 內部測試**：

  ```bash
  xattr -cr /Applications/SyncCraft.app    # 移除 quarantine 屬性
  ```

  或到「系統設定 → 隱私權與安全性」按「強制打開」。

- **正式發佈**：需要 Apple Developer Program（USD 99/年）。v3 內建簽章與公證流程：

  ```bash
  wails3 setup signing          # 互動設定憑證；密碼存在系統 keychain，不寫進專案
  wails3 task darwin:sign
  wails3 task darwin:sign:notarize
  ```

### 選單與複製貼上

v2 沒設定 Edit 選單時，Cmd+C/V 在 WebView 裡沒有作用（原版就有這個問題）。**v3 沒有提供自訂選單時，會自動套用預設的應用程式選單**，複製貼上可以直接用。要自訂選單時，記得自己加回 Edit 選單（`application.NewEditMenu()` 等角色選單）。

## 10.6 Windows

```bash
GOOS=windows wails3 build                  # 可在 Linux / macOS / Windows 上執行
GOOS=windows ARCH=arm64 wails3 build       # Snapdragon / Surface
GOOS=windows wails3 package                # NSIS 安裝程式（需安裝 makensis）
wails3 task windows:create:msix:package    # MSIX（Microsoft Store / Intune 用）
```

Fedora 上安裝 NSIS：`sudo dnf install mingw32-nsis`（套件名稱依發行版而異）。

### WebView2

Windows 11 內建 WebView2；Windows 10 多半也已經有。NSIS 安裝程式會在缺少時協助安裝。

> 公會內網這類**不能連外**的環境：用 `wails3 generate webview2bootstrapper` 取得 bootstrapper，或請 IT 事先佈署 WebView2 Evergreen Runtime。

### SmartScreen

未簽章的 exe 會出現「Windows 已保護您的電腦」。內部使用可以按「其他資訊 → 仍要執行」；正式發佈需要程式碼簽章憑證（OV/EV，或 Azure Trusted Signing），設定好後用 `wails3 task windows:sign`。

## 10.7 Linux

```bash
wails3 build                    # GTK4 + WebKitGTK 6.0（預設）
wails3 build -tags gtk3         # 改用 GTK3 + webkit2gtk-4.1（給較舊的發行版）
wails3 package                  # 一次產生 AppImage、.deb、.rpm、Arch 套件
wails3 task linux:create:rpm    # 只產生其中一種
```

打包設定在 `build/linux/nfpm/nfpm.yaml`（v3 內建 nFPM，不用另外安裝）。

### 相依套件與發行版

| 後端 | 需要的執行期套件 | 可執行的發行版（約略） |
|---|---|---|
| GTK4（預設） | `libgtk-4-1`、`libwebkitgtk-6.0-4` | Ubuntu 24.04+、Debian 13+、Fedora 39+ |
| GTK3（`-tags gtk3`） | `libgtk-3-0`、`libwebkit2gtk-4.1-0` | Ubuntu 22.04+、Debian 12+ |

### glibc 相容性

在 Fedora 45 上建置的執行檔會連結到很新的 glibc，拿到 Ubuntu 24.04 可能出現 `GLIBC_2.xx not found`。**發佈版要在「最舊的目標發行版」上建置**，例如 CI 用 `ubuntu-24.04`。本機 Fedora 的建置只給自己用。

### Hyprland / Wayland

WebKitGTK 在部分 Wayland + NVIDIA 環境會白畫面，可以試：

```bash
WEBKIT_DISABLE_DMABUF_RENDERER=1 ./bin/synccraft
```

## 10.8 版本號

單一來源：`build/config.yml` 的 `info.version`。

```bash
wails3 tool version --help      # 內建的語意化版本號遞增工具
```

Go 程式裡要顯示版本時，在 `build/<os>/Taskfile.yml` 的 `BUILD_FLAGS` 加上 `-X main.version={{.VERSION}}`，或在發佈時由 CI 用 git tag 注入。

## 10.9 自動更新（取代原版 updater）

原版自己寫了下載、替換執行檔的邏輯（第 7.5 節），沒有驗證檔案來源。v3 **內建 updater**：

- 支援 GitHub Releases 等更新來源
- **Ed25519 簽章驗證**：下載的檔案必須用你的私鑰簽過，才會被接受
- 原子替換，失敗可以回滾
- 內建更新 UI，可以自訂外觀

發佈端的工具：

```bash
wails3 updater genkey          # 產生簽章金鑰對（私鑰放 CI secret，公鑰編進 App）
wails3 updater sign ...        # 對發佈檔案計算 digest 並簽章
wails3 updater manifest ...    # 產生更新清單
wails3 updater verify ...      # 驗證清單、檔案、公鑰是否一致
```

App 端的整合方式請以官方文件 Updater 章節為準；SyncCraft 排在 M6 之後再做。

## 10.10 GitHub Actions：三平台建置

```yaml
# .github/workflows/release.yml
name: Release
on:
  push:
    tags: ['v*']

jobs:
  build:
    strategy:
      fail-fast: false
      matrix:
        include:
          - { os: macos-latest,     name: macos-universal, task: "darwin:package:universal" }
          - { os: windows-latest,   name: windows-amd64,   task: "windows:package" }
          - { os: ubuntu-24.04,     name: linux-amd64,     task: "linux:package" }
          - { os: ubuntu-24.04-arm, name: linux-arm64,     task: "linux:package" }
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v7
      - uses: jdx/mise-action@v5              # 讀 mise.toml：與本機相同版本的 wails3
      - uses: actions/setup-go@v7
        with: { go-version-file: go.mod }
      - uses: actions/setup-node@v7
        with: { node-version: lts/*, cache: npm, cache-dependency-path: frontend/package-lock.json }

      - name: Linux deps
        if: runner.os == 'Linux'
        run: sudo apt-get update && sudo apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev

      - run: wails3 task ${{ matrix.task }}

      - uses: actions/upload-artifact@v7
        with:
          name: synccraft-${{ matrix.name }}
          path: bin/

  release:
    needs: build
    runs-on: ubuntu-latest
    permissions: { contents: write }
    steps:
      - uses: actions/download-artifact@v8
        with: { path: dist }
      - run: cd dist && find . -type f -exec sha256sum {} \; > SHA256SUMS.txt
      - uses: softprops/action-gh-release@v3
        with:
          files: dist/**/*
          generate_release_notes: true
```

> 注意 mise 安裝 `wails3` 時本身也要編譯，所以 Linux 的 GTK4 套件要**在 mise 之前**裝好。實際的 workflow 檔（第 12 部）有處理這個順序。
>
> 公司內部若用 Gitea：Gitea Actions 語法與 GitHub Actions 相容，但 macOS 建置需要自己準備 Mac runner。

## 10.11 平台差異檢查清單

| 項目 | macOS | Windows | Linux |
|---|---|---|---|
| 設定檔路徑（`os.UserConfigDir`） | `~/Library/Application Support/SyncCraft` | `%AppData%\SyncCraft` | `~/.config/SyncCraft` |
| 密碼儲存 | Keychain | Credential Manager | Secret Service（需 gnome-keyring 等） |
| 檔案路徑 | `/` | `\`（一律用 `filepath.Join`） | `/` |
| 快捷鍵 | `Cmd` | `Ctrl` | `Ctrl` |
| 字型 | `-apple-system` | `Segoe UI` | `system-ui` / Noto |
| 安裝檔 | `.app` / `.dmg` | NSIS `.exe` / `.msix` | AppImage / `.deb` / `.rpm` |

## 練習

1. 換成 `modernc.org/sqlite`，在 Fedora 上執行 `GOOS=windows wails3 build`，拿到 Windows 機器上跑。
2. 修改 `build/config.yml` 的 `info`，執行 `common:update:build-assets`，用 `git diff` 看改了哪些檔。
3. 執行 `wails3 package`，用 `sudo dnf install ./bin/*.rpm` 安裝並從應用程式選單啟動。
4. 推一個 `v0.0.1` tag，讓 GitHub Actions 產出三平台檔案。
