# frontend

React 19 + TypeScript 7 + Vite 8。由 `wails3 dev` / `wails3 build` 驅動，通常不需要單獨執行 npm 指令。

## 結構

```
frontend/
├── bindings/        # 由 Go 自動產生，請勿手改（需提交，CI 會檢查是否同步）
│   ├── events/      #   事件名稱常數
│   └── github.com/lichenghsu/synccraft/internal/app/   # Service 函式與型別
├── src/
│   ├── api/         # 包裝 Go 綁定（見 api/README.md）
│   ├── hooks/       # 共用 React hooks（見 hooks/README.md）
│   ├── types.ts     # 前端自己的型別
│   ├── App.tsx
│   └── style.css
├── vite.config.ts
└── tsconfig.json
```

## 路徑別名

| 別名 | 指向 | 用途 |
|---|---|---|
| `@bindings/*` | `bindings/github.com/lichenghsu/synccraft/*` | `import { HelloService } from "@bindings/internal/app"` |
| `@events` | `bindings/events` | `import { CountTick } from "@events"` |

別名要在 **`tsconfig.json`（型別檢查）與 `vite.config.ts`（打包）兩邊同時設定**。TypeScript 7 已移除 `baseUrl`，`paths` 直接相對於 tsconfig 檔案解析。

Go module 路徑若改變，這兩個檔案的別名也要跟著改。

## 指令

```bash
npm run build      # tsc 型別檢查 + vite build → dist/（被 Go 的 go:embed 嵌入）
npm run dev        # 只啟動 Vite；一般請用專案根目錄的 wails3 dev
```

## 其他

- `.npmrc` 的 `minimum-release-age=10080`：不安裝發佈未滿 7 天的版本，降低套件被投毒的風險。npm 目前不支援、會印警告；pnpm / bun 才會生效。
- `noUncheckedIndexedAccess` 已開啟：`arr[i]` 的型別是 `T | undefined`。
