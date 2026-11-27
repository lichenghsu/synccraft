# features/compare

Schema compare: pick a source (the reference) and a target, list the structural differences, choose which to apply, review the SQL, and run it on the target.

## Files

| File | Contents |
|---|---|
| `ComparePage.tsx` | Source / target pickers, Compare, change list with selection, SQL preview, Apply with confirmation |
| `SidePicker.tsx` | Saved connection + database picker for one side (SQLite has no database picker) |
| `model.ts` | `describeChange` (one line per change), `groupByTable`, `defaultSelection`, `scriptText`, `sideReady` |

## Flow

1. Both sides picked, same engine, not the same database: Compare is enabled. Different engines are rejected here and again by the backend.
2. `api.compare` returns every change with a token. Destructive changes (drop table, drop column, type change) and unsupported ones start unselected.
3. Each selection change calls `api.previewChanges(token, ids)`. The SQL is generated for the selection as a whole, because later steps can depend on earlier ones (SQLite table rebuilds).
4. Apply asks for a second click that names the target and how many selected changes may lose data. For MySQL / MariaDB the page warns that DDL commits statement by statement.
5. `api.applyChanges(token, ids)` sends only change ids, never SQL. The backend reloads both schemas and refuses if either changed since step 2. After a successful apply the page compares again; ideally nothing is left.

Changing either side clears the result. The token is spent by Apply whether it succeeds or not, so a failed apply clears the result too; compare again to see the current state.
