# internal/schema

Schema comparison: load the full structure of a database and list the changes that make a target match a source. The source is the reference; nothing here changes the source. Does not import Wails and generates no SQL; DDL generation per target dialect builds on `Change`.

## Files

| File | Contents |
|---|---|
| `schema.go` | `Table` (table + columns + primary key + indexes), `Load` |
| `diff.go` | `Kind`, `Change`, `Diff` (pure function, no database access) |
| `normalize.go` | `normalizeType`, `normalizeDefault`: equality only, DDL keeps the original text |

## Matching

| Object | Matched by | Different when |
|---|---|---|
| Table | `Schema` + `Name` (exact) | — (missing on one side) |
| Column | `Name` (exact) | normalized type, `Nullable`, or normalized default differ. `Position` is ignored |
| Primary key | — | column list differs (nil and empty are equal) |
| Index | `Name` (exact) | `Columns`, `Unique` or `Constraint` differ |

A changed index becomes `DropIndex` + `AddIndex`; a renamed column or table becomes drop + add (no rename detection).

## Order

`Diff` never reorders its inputs and returns:

1. `AddTable` (by schema, name). `Create` holds the whole source table.
2. Changes to tables on both sides, grouped by table, in phases:
   `DropIndex` → `AddColumn` → `AlterColumn` → `AlterPrimaryKey` → `DropColumn` → `AddIndex`.
   Indexes are dropped before the columns they use; columns exist before keys and indexes need them.
   Added columns come in source position order, so `After` always names a column that already exists or was added earlier.
3. `DropTable`.

`Destructive` is set on `DropTable`, `DropColumn` and on `AlterColumn` when the type changes. Nullability and default changes are not marked (setting `NOT NULL` fails on existing NULLs rather than losing data).

## Normalization

| Case | Example | Rule |
|---|---|---|
| Type case and spacing | `VARCHAR(20)` vs `varchar(20)` | lowercase, collapse spaces |
| MySQL integer display width | `int(11)` vs `int` | drop `(n)` after integer types (also `tinyint(1)`) |
| SQL Server parentheses | `((0))` | strip parentheses around the whole expression |
| PostgreSQL casts | `'guest'::character varying` | strip a trailing top-level `::type` (not `nextval('s'::regclass)`) |
| Expression case | `CURRENT_TIMESTAMP` | lowercase outside string literals |
| MariaDB vs MySQL | `current_timestamp()` | → `current_timestamp` |

## Limits

- Comparison is meant for two databases of the same engine; types are compared as text, so cross-engine comparisons report every column as changed.
- Auto-generated constraint names (SQL Server `UQ__users__…`, `PK__…`) differ between databases and show up as index changes.
- Altering a column that an unchanged index uses fails on SQL Server; `Diff` does not drop and recreate that index.
- Not compared: foreign keys, check constraints, views, triggers, collations, comments (see `internal/dialect` "Not covered yet").

## Tests

Everything runs in plain `go test`: `Diff` and normalization are pure, and `Load` is tested against SQLite.
