import type { Table } from "../../api/wails";

export const tableLabel = (t: Table) =>
  t.schema ? `${t.schema}.${t.name}` : t.name;

export const sameTable = (a: Table, b: Table) =>
  a.schema === b.schema && a.name === b.name;

export const PAGE_SIZE = 100;

export type Cell = string | number | boolean | null;

// Numbers and booleans come as JSON values; big integers, decimals (MySQL),
// times and binary come as strings already formatted by the backend.
export const cellText = (v: Cell) => (v === null ? "NULL" : String(v));
