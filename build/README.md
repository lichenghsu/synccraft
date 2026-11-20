# build

Wails v3 的建置設定，由 `wails3 init` 產生後提交進 repo。建置流程由 [Task](https://taskfile.dev) 執行：根目錄的 `Taskfile.yml` 依 `GOOS` 分派到這裡各平台的 `Taskfile.yml`。

## 結構

| 路徑 | 內容 |
|---|---|
| `config.yml` | App 資訊（名稱、識別碼、版本…）與 `wails3 dev` 設定 |
| `Taskfile.yml` | 共用任務：前端建置、產生綁定、產生圖示（命名空間 `common:`） |
| `appicon.png` | 1024×1024 原始圖示，建置時自動轉成各平台格式 |
| `darwin/` | macOS：Info.plist、圖示、DMG 素材、簽章與公證任務 |
| `windows/` | Windows：版本資訊、manifest、NSIS、MSIX |
| `linux/` | Linux：`.desktop`、AppImage、nfpm（deb / rpm / Arch） |
| `docker/` | 交叉編譯用的 Dockerfile（`wails3 task common:setup:docker`） |
| `ios/`、`android/` | 行動平台（Wails 實驗性功能，目前未使用） |

## 常用指令

```bash
wails3 build                         # 目前平台 → bin/
wails3 package                       # 目前平台的安裝檔
GOOS=windows wails3 build            # 交叉編譯 Windows（CGO=0，不需要 Docker）
wails3 task darwin:package:universal # macOS universal（需在 Mac 上）
wails3 task --list                   # 列出所有任務
```

## 修改 App 資訊

1. 改 `config.yml` 的 `info:`。
2. 執行 `wails3 task common:update:build-assets`，把資訊寫進各平台檔案。
3. 這個指令會**覆蓋**平台檔案裡的手動修改，執行前先 commit，執行後用 `git diff build/` 檢查。

> `wails3 init` 的 `-product*` 參數不會寫進 `config.yml`（v3.1.0），所以這裡是手動改成和平台檔案一致的。

## 升級 Wails 時

升級 CLI 不會更新這個資料夾。升 minor 版後執行 `common:update:build-assets`，用新版範本重新產生，再把自己的客製化改回來（教材第 12.5 節）。
