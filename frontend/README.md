# frontend

React 19 + TypeScript 7 + Vite 8. Driven by `wails3 dev` / `wails3 build`; you rarely need to run npm commands directly.

## Layout

```
frontend/
├── bindings/        # generated from Go, do not edit (commit it; CI checks it is in sync)
│   ├── events/      #   event name constants
│   └── github.com/lichenghsu/synccraft/internal/   # app/ services, conn/ and store/ types
├── src/
│   ├── api/         # wrappers around the Go bindings (see api/README.md)
│   ├── features/    # one folder per screen (see each README.md)
│   ├── hooks/       # shared React hooks (see hooks/README.md)
│   ├── types.ts     # frontend-only types
│   ├── App.tsx
│   └── style.css
├── vite.config.ts
└── tsconfig.json
```

## Path aliases

| Alias | Points to | Usage |
|---|---|---|
| `@bindings/*` | `bindings/github.com/lichenghsu/synccraft/*` | `import { ConnectionService } from "@bindings/internal/app"` |
| `@events` | `bindings/events` | Event name constants (none registered yet) |

Aliases must be set in **both `tsconfig.json` (type checking) and `vite.config.ts` (bundling)**. TypeScript 7 removed `baseUrl`; `paths` resolve relative to the tsconfig file.

If the Go module path changes, update the aliases in both files.

## Commands

```bash
npm run build      # tsc type check + vite build → dist/ (embedded by Go's go:embed)
npm run dev        # Vite only; normally use wails3 dev from the project root
```

## Notes

- `minimum-release-age=10080` in `.npmrc`: don't install versions published less than 7 days ago, to reduce supply-chain poisoning risk. npm doesn't support it yet and prints a warning; it only takes effect with pnpm / bun.
- `noUncheckedIndexedAccess` is on: `arr[i]` has type `T | undefined`.
