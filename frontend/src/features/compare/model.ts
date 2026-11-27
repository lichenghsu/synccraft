import {
  arr,
  DBType,
  Kind,
  type Change,
  type Column,
  type Connection,
  type Side,
  type Step,
} from "../../api/wails";
import { tableLabel } from "../browse/model";

export const emptySide: Side = { connectionId: "", database: "" };

// SQLite has no database picker; other engines need one picked.
export const sideReady = (side: Side, saved: Connection[]) => {
  const c = saved.find((x) => x.id === side.connectionId);
  return !!c && (c.config.type === DBType.SQLite || side.database !== "");
};

const columnText = (c: Column) =>
  [
    c.type || "(no type)",
    c.nullable ? "" : "NOT NULL",
    c.default != null ? `DEFAULT ${c.default}` : "",
    c.autoIncrement ? "auto-increment" : "",
  ]
    .filter(Boolean)
    .join(" ");

const keyText = (cols: string[] | null | undefined) => {
  const a = arr(cols);
  return a.length ? `(${a.map((x) => x || "<expr>").join(", ")})` : "none";
};

export function describeChange(c: Change): string {
  switch (c.kind) {
    case Kind.AddTable:
      return `Create table (${arr(c.create?.columns).length} columns)`;
    case Kind.DropTable:
      return "Drop table";
    case Kind.AddColumn:
      return `Add column ${c.column!.name} ${columnText(c.column!)}`;
    case Kind.DropColumn:
      return `Drop column ${c.column!.name}`;
    case Kind.AlterColumn:
      return `Change column ${c.column!.name}: ${columnText(c.from!)} → ${columnText(c.column!)}`;
    case Kind.AlterPrimaryKey:
      return `Primary key ${keyText(c.fromPrimaryKey)} → ${keyText(c.primaryKey)}`;
    case Kind.AddIndex: {
      const ix = c.index!;
      const what = ix.constraint
        ? "unique constraint"
        : ix.unique
          ? "unique index"
          : "index";
      return `Add ${what} ${ix.name} ${keyText(ix.columns)}`;
    }
    case Kind.DropIndex:
      return `Drop ${c.index!.constraint ? "constraint" : "index"} ${c.index!.name}`;
  }
  return c.kind;
}

export type TableGroup = { label: string; steps: Step[] };

// Groups consecutive steps by table, keeping execution order.
export function groupByTable(steps: Step[]): TableGroup[] {
  const groups: TableGroup[] = [];
  for (const s of steps) {
    const label = tableLabel(s.change.table);
    const last = groups[groups.length - 1];
    if (last && last.label === label) last.steps.push(s);
    else groups.push({ label, steps: [s] });
  }
  return groups;
}

// Destructive and unsupported changes start unselected.
export const defaultSelection = (steps: Step[]) =>
  new Set(
    steps
      .filter((s) => !s.change.destructive && !s.unsupported)
      .map((s) => s.change.id),
  );

export function scriptText(steps: Step[]): string {
  return steps
    .map((s) => {
      const head = `-- ${tableLabel(s.change.table)}: ${describeChange(s.change)}`;
      if (s.unsupported) return `${head}\n-- not supported: ${s.unsupported}`;
      return [head, ...arr(s.sql).map((q) => `${q};`)].join("\n");
    })
    .join("\n\n");
}
