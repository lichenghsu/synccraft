# 第 8 部　React + TypeScript：把 Vue 前端改寫成 React

原版前端：`App.vue`（1148 行）+ 4 個元件 + `vue-i18n`。本部以「Vue 概念 → React 對應」的方式重建。

## 8.1 Vue ↔ React 對照

| Vue 3（原版） | React | 說明 |
|---|---|---|
| `ref(x)` / `reactive` | `useState(x)` | React 狀態不可變：`setX(新值)`，不能 `x.value = ...` |
| `computed(() => ...)` | 直接在 render 計算，昂貴時 `useMemo` | |
| `watch(src, fn)` | `useEffect(fn, [src])` | 依賴陣列決定何時重跑 |
| `onMounted` | `useEffect(fn, [])` | |
| `onUnmounted` | `useEffect` 回傳的 cleanup | |
| `props` + `emit('x')` | props + callback prop（`onX`） | |
| `v-model` | `value` + `onChange` | 受控元件 |
| `v-if` / `v-for` | `{cond && <X/>}` / `array.map(...)` | `map` 要給 `key` |
| `<style scoped>` | CSS Modules（`X.module.css`） | |
| `vue-i18n` `t()` | `react-i18next` `useTranslation()` | 可直接沿用 `locales/en.ts`、`zh.ts` |

## 8.2 專案結構

```
frontend/src/
├── main.tsx
├── App.tsx                    # 版面：頂部連線列 + 分頁
├── api/
│   └── wails.ts               # 包一層 bindings，統一處理 null / 錯誤 / 取消
├── state/
│   └── connections.tsx        # Context：source/target 設定
├── hooks/
│   ├── useAsync.ts
│   └── useWailsEvent.ts
├── features/
│   ├── connection/ConnectionForm.tsx
│   ├── schema/SchemaCompare.tsx  + DiffList.tsx
│   ├── data/DataSync.tsx
│   └── browser/TableBrowser.tsx
├── components/                # 共用 UI：Button、Tabs、SqlBlock、Modal
├── i18n/{index.ts,en.ts,zh.ts}
└── types.ts                   # 前端自己的型別（UI 狀態等）
```

## 8.3 TypeScript 與 Wails 產生的型別

Wails v3 預設把 Go struct 產生成 TS **interface**，直接用物件字面值當參數即可（v2 產生的是 class，雖然因為 TS 是結構型別也能這樣用，但比較不直覺）：

```ts
import type { ConnectionConfig } from '@bindings/internal/app'   // 第 1.3 節設定的別名

const cfg: ConnectionConfig = {
  type: 'mysql', host: '127.0.0.1', port: 3306,
  user: 'root', password: 'root', database: 'shop',
}
```

Go 的 `Type DBType` 在 TS 只是 `string`。自己收窄成聯集型別：

```ts
// types.ts
export type DBType = 'mysql' | 'postgresql' | 'sqlite' | 'sqlserver'

export const DEFAULT_PORT: Record<DBType, number> = {
  mysql: 3306, postgresql: 5432, sqlserver: 1433, sqlite: 0,
}

export type Side = 'source' | 'target'
export type DiffType = 'added' | 'removed' | 'modified'
```

> `Record<K, V>` 在這裡的好處：之後新增 `'mariadb'` 到 `DBType`，所有 `Record<DBType, ...>` 沒補上的地方都會編譯錯誤。

### 判別聯集（Discriminated Union）描述非同步狀態

```ts
export type AsyncState<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: string }
```

在 `switch (s.status)` 裡，TS 會自動知道 `success` 分支有 `data`、`error` 分支有 `error`。

## 8.4 包一層 API

```ts
// api/wails.ts
import { ConnService, SchemaService, DataService, BrowserService } from '@bindings/internal/app'
import type { ConnectionConfig } from '@bindings/internal/app'

const arr = <T,>(x: T[] | null | undefined): T[] => x ?? []   // Go nil slice → null

export const api = {
  testConnection: (c: ConnectionConfig) => ConnService.TestConnection(c),
  getDatabases: async (c: ConnectionConfig) => arr(await ConnService.GetDatabases(c)),
  compareSchemas: async (s: ConnectionConfig, t: ConnectionConfig) =>
    arr(await SchemaService.CompareSchemas(s, t)),
  executeSQL: (c: ConnectionConfig, sql: string) => SchemaService.ExecuteSQL(c, sql),
  getTablesForSync: async (c: ConnectionConfig) => arr(await DataService.GetTablesForSync(c)),
  compareTableData: async (s: ConnectionConfig, t: ConnectionConfig, table: string) =>
    arr(await DataService.CompareTableData(s, t, table)),
  getTableData: BrowserService.GetTableData,
  getTableStructure: BrowserService.GetTableStructure,
}

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))
```

好處：元件不直接依賴 bindings 的路徑與 Service 切分方式；Go 端重構時只改這一檔。

> 實際匯出的名稱以 `wails3` 產生的 `bindings/.../index.ts` 為準。
>
> 注意：上面用 `async` 包過的函式會回傳一般的 `Promise`，**`.cancel()` 就不見了**。需要取消的呼叫（例如資料比對）要直接回傳 Service 的 `CancellablePromise`，不要用 `async` 包。

## 8.5 自訂 Hooks

```ts
// hooks/useAsync.ts
import { useCallback, useState } from 'react'
import type { AsyncState } from '../types'
import { errMsg } from '../api/wails'

export function useAsync<A extends unknown[], T>(fn: (...args: A) => Promise<T>) {
  const [state, setState] = useState<AsyncState<T>>({ status: 'idle' })
  const run = useCallback(async (...args: A) => {
    setState({ status: 'loading' })
    try {
      const data = await fn(...args)
      setState({ status: 'success', data })
      return data
    } catch (e) {
      setState({ status: 'error', error: errMsg(e) })
      throw e
    }
  }, [fn])
  return { state, run }
}
```

```ts
// hooks/useWailsEvent.ts
import { useEffect, useRef } from 'react'
import { Events } from '@wailsio/runtime'

export function useWailsEvent<T>(name: string, handler: (payload: T) => void) {
  const ref = useRef(handler)
  ref.current = handler                 // 永遠呼叫最新的 handler，不必重新訂閱
  useEffect(() => Events.On(name, (ev) => ref.current(ev.data as T)), [name])  // 回傳值即取消訂閱
}
```

Go 端用 `application.RegisterEvent[T]("name")` 註冊過的事件，產生器會提供有型別的事件名稱，`ev.data` 就不需要 `as T`。

```ts
// hooks/useCancellable.ts：可取消的長時間呼叫
import { useRef, useState } from 'react'
import type { CancellablePromise } from '@wailsio/runtime'

export function useCancellable<A extends unknown[], T>(fn: (...args: A) => CancellablePromise<T>) {
  const ref = useRef<CancellablePromise<T> | null>(null)
  const [running, setRunning] = useState(false)
  const run = async (...args: A) => {
    ref.current?.cancel()
    const p = fn(...args)
    ref.current = p
    setRunning(true)
    try { return await p } finally { if (ref.current === p) { ref.current = null; setRunning(false) } }
  }
  const cancel = () => ref.current?.cancel()
  return { run, cancel, running }
}
```

> 泛型元件/函式在 `.tsx` 裡寫 `<T,>`（加逗號）避免被當成 JSX 標籤。

## 8.6 全域狀態：Context + useReducer

source / target 兩組連線設定會被所有分頁用到：

```tsx
// state/connections.tsx
import { createContext, useContext, useReducer, type ReactNode, type Dispatch } from 'react'
import type { ConnectionConfig } from '@bindings/internal/app'
import type { Side } from '../types'

type SideState = { config: ConnectionConfig; connected: boolean; databases: string[] }
type State = Record<Side, SideState>

type Action =
  | { type: 'setConfig'; side: Side; patch: Partial<ConnectionConfig> }
  | { type: 'connected'; side: Side; databases: string[] }
  | { type: 'disconnect'; side: Side }

const empty = (port: number): SideState => ({
  config: { type: 'mysql', host: '127.0.0.1', port, user: 'root', password: '', database: '' },
  connected: false, databases: [],
})

function reducer(s: State, a: Action): State {
  switch (a.type) {
    case 'setConfig':  return { ...s, [a.side]: { ...s[a.side], config: { ...s[a.side].config, ...a.patch } } }
    case 'connected':  return { ...s, [a.side]: { ...s[a.side], connected: true, databases: a.databases } }
    case 'disconnect': return { ...s, [a.side]: { ...s[a.side], connected: false, databases: [] } }
  }
}

const Ctx = createContext<{ state: State; dispatch: Dispatch<Action> } | null>(null)

export function ConnectionsProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, { source: empty(3306), target: empty(3307) })
  return <Ctx.Provider value={{ state, dispatch }}>{children}</Ctx.Provider>
}

export function useConnections() {
  const v = useContext(Ctx)
  if (!v) throw new Error('useConnections must be used inside ConnectionsProvider')
  return v
}
```

`switch` 沒有 `default` 也沒關係：TS 知道 `Action` 只有三種，漏寫會報「函式缺少結尾 return」。

> 狀態再變複雜可換成 **Zustand**（API 更簡單、不需 Provider）。學習階段先用內建的，理解 React 本身的資料流。

## 8.7 元件範例：連線表單

```tsx
// features/connection/ConnectionForm.tsx
import { useConnections } from '../../state/connections'
import { useAsync } from '../../hooks/useAsync'
import { api } from '../../api/wails'
import { DEFAULT_PORT, type DBType, type Side } from '../../types'

export function ConnectionForm({ side }: { side: Side }) {
  const { state, dispatch } = useConnections()
  const { config, connected, databases } = state[side]
  const connect = useAsync(async () => {
    await api.testConnection(config)
    return api.getDatabases(config)
  })

  const set = (patch: Partial<typeof config>) => dispatch({ type: 'setConfig', side, patch })

  const onConnect = async () => {
    const dbs = await connect.run().catch(() => null)
    if (dbs) dispatch({ type: 'connected', side, databases: dbs })
  }

  return (
    <form onSubmit={e => { e.preventDefault(); onConnect() }} className="conn-form">
      <select value={config.type}
              onChange={e => { const t = e.target.value as DBType; set({ type: t, port: DEFAULT_PORT[t] }) }}>
        <option value="mysql">MySQL</option>
        <option value="postgresql">PostgreSQL</option>
        <option value="sqlserver">SQL Server</option>
        <option value="sqlite">SQLite</option>
      </select>

      {config.type === 'sqlite' ? (
        <input placeholder="/path/to/file.db" value={config.filePath ?? ''}
               onChange={e => set({ filePath: e.target.value })} />
      ) : (
        <>
          <input value={config.host} onChange={e => set({ host: e.target.value })} />
          <input type="number" value={config.port} onChange={e => set({ port: Number(e.target.value) })} />
          <input value={config.user} onChange={e => set({ user: e.target.value })} />
          <input type="password" value={config.password} onChange={e => set({ password: e.target.value })} />
        </>
      )}

      <button disabled={connect.state.status === 'loading'}>
        {connect.state.status === 'loading' ? '連線中…' : '連線'}
      </button>
      {connect.state.status === 'error' && <p className="error">{connect.state.error}</p>}

      {connected && (
        <select value={config.database} onChange={e => set({ database: e.target.value })}>
          <option value="">— 選擇資料庫 —</option>
          {databases.map(db => <option key={db} value={db}>{db}</option>)}
        </select>
      )}
    </form>
  )
}
```

## 8.8 元件範例：Schema 比對結果

```tsx
// features/schema/SchemaCompare.tsx
import { useMemo, useState } from 'react'
import type { DiffResult } from '@bindings/internal/app'
import { api } from '../../api/wails'
import { useConnections } from '../../state/connections'
import { useAsync } from '../../hooks/useAsync'

const COLOR = { added: 'green', modified: 'amber', removed: 'red' } as const

export function SchemaCompare() {
  const { state } = useConnections()
  const compare = useAsync(api.compareSchemas)
  const [selected, setSelected] = useState<Set<number>>(new Set())

  const diffs: DiffResult[] = compare.state.status === 'success' ? compare.state.data : []
  const grouped = useMemo(() => Object.groupBy(diffs, d => d.type as keyof typeof COLOR), [diffs])
  const selectedSQL = diffs.filter((_, i) => selected.has(i)).map(d => d.sql).join('\n')

  const toggle = (i: number) => setSelected(prev => {
    const next = new Set(prev)          // 不可直接改 prev：要建新的 Set 才會觸發重繪
    next.has(i) ? next.delete(i) : next.add(i)
    return next
  })

  const execute = async () => {
    await api.executeSQL(state.target.config, selectedSQL)
    setSelected(new Set())
    compare.run(state.source.config, state.target.config)
  }

  return (
    <section>
      <button onClick={() => compare.run(state.source.config, state.target.config)}>比對 Schema</button>
      {compare.state.status === 'error' && <p className="error">{compare.state.error}</p>}

      {(['added', 'modified', 'removed'] as const).map(type => (
        <div key={type}>
          <h3 className={COLOR[type]}>{type} ({grouped[type]?.length ?? 0})</h3>
          {grouped[type]?.map(d => {
            const i = diffs.indexOf(d)
            return (
              <label key={i} className="diff-row">
                <input type="checkbox" checked={selected.has(i)} onChange={() => toggle(i)} />
                <strong>{d.tableName}</strong> — {d.detail}
                <pre>{d.sql}</pre>
              </label>
            )
          })}
        </div>
      ))}

      <button disabled={selected.size === 0} onClick={execute}>執行選取的 {selected.size} 句</button>
    </section>
  )
}
```

重點觀念：

- **不可變更新**：`Set`、陣列、物件都要建立新的再 `set`，React 用參考比較判斷是否重繪。
- `as const`：讓 `COLOR` 的鍵與值變成字面值型別，`COLOR[type]` 型別精準。
- `Object.groupBy` 需要 ES2024；`tsconfig` 的 `lib` 要含 `ES2024`（或自己寫 reduce）。

## 8.9 大量資料：虛擬捲動

資料同步可能有上萬筆差異，全部 render 會卡。用 `@tanstack/react-virtual`：

```tsx
import { useVirtualizer } from '@tanstack/react-virtual'

function DiffTable({ rows }: { rows: RowChange[] }) {
  const parentRef = useRef<HTMLDivElement>(null)
  const v = useVirtualizer({ count: rows.length, getScrollElement: () => parentRef.current, estimateSize: () => 32 })
  return (
    <div ref={parentRef} style={{ height: 500, overflow: 'auto' }}>
      <div style={{ height: v.getTotalSize(), position: 'relative' }}>
        {v.getVirtualItems().map(item => (
          <div key={item.key} style={{ position: 'absolute', top: 0, transform: `translateY(${item.start}px)`, height: item.size }}>
            <RowView row={rows[item.index]} />
          </div>
        ))}
      </div>
    </div>
  )
}
```

## 8.10 i18n

```bash
npm i i18next react-i18next
```

```ts
// i18n/index.ts
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './en'; import zh from './zh'      // 直接搬原版 locales/*.ts

i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, zh: { translation: zh } },
  lng: localStorage.getItem('lang') ?? 'zh',
  interpolation: { escapeValue: false },
})
export default i18n
```

```tsx
const { t } = useTranslation()
<h2>{t('dataSync.title')}</h2>
<p>{t('dataSync.comparedTable', { table: name, count })}</p>
```

原版 vue-i18n 插值寫法是 `{count}`，i18next 預設是 `{{count}}`——搬過來時要全域取代，或設定 `interpolation: { prefix: '{', suffix: '}' }`。

## 8.11 樣式建議

- 起步：一份 `style.css` 定義 CSS 變數（深色主題：背景 `#1b2636`，與原版 `BackgroundColour` 一致）+ CSS Modules。
- 想快：Tailwind CSS。
- SQL 顯示：`<pre>` + 等寬字型即可；要語法高亮可用 `prism-react-renderer`。
- 元件庫可選 Radix UI / shadcn/ui（無樣式、可完全客製）。

## 8.12 tsconfig 重點

```jsonc
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2024", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "react-jsx",
    "strict": true,                       // 一定要開
    "noUncheckedIndexedAccess": true,     // arr[i] 的型別會是 T | undefined，逼你處理
    "noEmit": true,
    "skipLibCheck": true
  },
  "include": ["src", "bindings"]
}
```

`package.json` 的 build：`"build": "tsc && vite build"` —— 型別錯誤會讓 `wails3 build` 失敗，這是好事。

## 練習

1. 完成 `ConnectionsProvider` 與兩個 `ConnectionForm`（source / target 並排）。
2. 完成 `SchemaCompare`，對第 1 部的種子資料顯示 4 筆差異並可執行。
3. 用 `useWailsEvent` 接第 2.6 節的 `sync:progress`，做一條真的進度條。
4. 把原版 `locales/zh.ts` 搬過來，切換語言不需重開。
