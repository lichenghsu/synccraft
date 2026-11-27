# features/browse

The table browser: pick a saved connection and database, then a table, and see its structure or its rows. Read-only.

## Files

| File | Contents |
|---|---|
| `BrowsePage.tsx` | Connection and database pickers (`api.listSaved`, `api.listSavedDatabases`), table list with filter, Structure / Data tabs |
| `TableStructure.tsx` | Columns (type, nullable, default, primary key position) and secondary indexes |
| `TableData.tsx` | Rows in pages of 100, Prev / Next, "Count rows" on request (cancellable) |
| `model.ts` | `tableLabel` (`schema.name`), `sameTable`, `PAGE_SIZE`, `cellText` |

## Behavior

- Only saved connections can be browsed: the backend looks them up by id and reads their passwords from the keychain.
- SQLite has no database picker; the file is the database.
- Every load goes through `useCancellable`: picking another database or table cancels the request still running for the previous one, so a slow answer never overwrites a newer one.
- `TableStructure` and `TableData` are keyed by table, so switching tables resets paging and the row count.
- The row count is not loaded automatically because `COUNT(*)` scans the table on most engines. Without a count, Next is enabled until a page comes back short.
- Tables without a primary key show a notice: their rows come in server order and may shift between pages.
- Cell values arrive already formatted by the backend (see `internal/dialect/README.md`): `NULL` is shown in italics, numbers are right-aligned, long values are cut by the backend and shown in full only up to that cut (hover shows the cell text).
