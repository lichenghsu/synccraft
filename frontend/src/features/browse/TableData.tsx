import { useEffect, useState } from "react";
import {
  api,
  arr,
  errMsg,
  isCancel,
  type RowPage,
  type Table,
} from "../../api/wails";
import { useCancellable } from "../../hooks/useCancellable";
import { PAGE_SIZE, cellText, type Cell } from "./model";

type Props = { connId: string; database: string; table: Table };

export function TableData({ connId, database, table }: Props) {
  const [offset, setOffset] = useState(0);
  const [page, setPage] = useState<RowPage | null>(null);
  const [error, setError] = useState("");
  const [total, setTotal] = useState<number | null>(null);
  const [countMsg, setCountMsg] = useState("");

  const preview = useCancellable(api.previewRows);
  const count = useCancellable(api.countRows);
  const runPreview = preview.run;

  useEffect(() => {
    runPreview(connId, database, table, offset, PAGE_SIZE)
      .then((p) => {
        setPage(p);
        setError("");
      })
      .catch((e) => {
        if (!isCancel(e)) setError(errMsg(e));
      });
  }, [runPreview, connId, database, table, offset]);

  const startCount = () => {
    setCountMsg("");
    count
      .run(connId, database, table)
      .then(setTotal)
      .catch((e) => setCountMsg(isCancel(e) ? "Count cancelled" : errMsg(e)));
  };

  const rows = arr(page?.rows).map((r) => arr(r) as Cell[]);
  const columns = arr(page?.columns);
  const atEnd =
    rows.length < PAGE_SIZE || (total !== null && offset + PAGE_SIZE >= total);

  return (
    <div className="data">
      <div className="row between wrap">
        <div className="row">
          <button
            type="button"
            className="ghost small"
            disabled={offset === 0 || preview.running}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
          >
            ← Prev
          </button>
          <button
            type="button"
            className="ghost small"
            disabled={atEnd || preview.running}
            onClick={() => setOffset(offset + PAGE_SIZE)}
          >
            Next →
          </button>
          <span className="hint">
            {rows.length === 0
              ? "No rows"
              : `Rows ${offset + 1}–${offset + rows.length}`}
            {total !== null && ` of ${total.toLocaleString()}`}
          </span>
        </div>
        <div className="row">
          {count.running ? (
            <button
              type="button"
              className="ghost small"
              onClick={count.cancel}
            >
              Cancel count
            </button>
          ) : (
            total === null && (
              <button
                type="button"
                className="ghost small"
                onClick={startCount}
              >
                Count rows
              </button>
            )
          )}
          {countMsg && <span className="err">{countMsg}</span>}
        </div>
      </div>

      {page && !page.ordered && (
        <p className="hint">
          No primary key: rows come in server order and may shift between pages.
        </p>
      )}
      {error && <p className="err">{error}</p>}
      {preview.running && !page && <p className="hint">Loading rows…</p>}

      {page && (
        <div className="grid-wrap">
          <table className="grid">
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c} className="mono">
                    {c}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={offset + i}>
                  {r.map((v, j) => (
                    <td
                      key={j}
                      className={`mono ${v === null ? "null" : typeof v === "number" ? "num" : ""}`}
                      title={cellText(v)}
                    >
                      {cellText(v)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
