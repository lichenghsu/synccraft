import { useEffect, useState } from "react";
import {
  api,
  arr,
  errMsg,
  isCancel,
  type Table,
  type TableInfo,
} from "../../api/wails";
import { useCancellable } from "../../hooks/useCancellable";

type Props = { connId: string; database: string; table: Table };

export function TableStructure({ connId, database, table }: Props) {
  const [info, setInfo] = useState<TableInfo | null>(null);
  const [error, setError] = useState("");
  const describe = useCancellable(api.describeTable);
  const run = describe.run;

  useEffect(() => {
    run(connId, database, table)
      .then(setInfo)
      .catch((e) => {
        if (!isCancel(e)) setError(errMsg(e));
      });
  }, [run, connId, database, table]);

  if (error) return <p className="err">{error}</p>;
  if (!info) return <p className="hint">Loading structure…</p>;

  const pk = arr(info.primaryKey);
  const indexes = arr(info.indexes);
  return (
    <div className="structure">
      <div className="grid-wrap">
        <table className="grid">
          <thead>
            <tr>
              <th>#</th>
              <th>Column</th>
              <th>Type</th>
              <th>Null</th>
              <th>Default</th>
              <th>Key</th>
            </tr>
          </thead>
          <tbody>
            {arr(info.columns).map((c) => {
              const keyPos = pk.indexOf(c.name);
              return (
                <tr key={c.name}>
                  <td className="num">{c.position}</td>
                  <td className="mono">{c.name}</td>
                  <td className="mono">
                    {c.type || <i className="muted">none</i>}
                  </td>
                  <td>{c.nullable ? "yes" : "no"}</td>
                  <td className="mono">
                    {c.default ?? <i className="muted">none</i>}
                  </td>
                  <td>
                    {keyPos >= 0 && (
                      <span className="badge">
                        PK{pk.length > 1 ? ` ${keyPos + 1}` : ""}
                      </span>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <h3>Indexes</h3>
      {pk.length === 0 && <p className="hint">No primary key.</p>}
      {indexes.length === 0 ? (
        <p className="hint">No secondary indexes.</p>
      ) : (
        <ul className="index-list">
          {indexes.map((ix) => (
            <li key={ix.name}>
              <span className="mono">{ix.name}</span>{" "}
              {ix.unique && <span className="badge">unique</span>}{" "}
              {ix.constraint && <span className="badge">constraint</span>}
              <span className="muted mono">
                {" "}
                (
                {arr(ix.columns)
                  .map((c) => c || "<expression>")
                  .join(", ")}
                )
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
