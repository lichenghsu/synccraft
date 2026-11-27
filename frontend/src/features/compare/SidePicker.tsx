import { useEffect, useRef, useState } from "react";
import {
  api,
  DBType,
  errMsg,
  type Connection,
  type Side,
} from "../../api/wails";
import { DB_LABEL } from "../connections/model";

type Props = {
  label: string;
  saved: Connection[];
  value: Side;
  onChange: (side: Side) => void;
  disabled?: boolean;
};

// Connection + database picker for one side of a comparison.
export function SidePicker({ label, saved, value, onChange, disabled }: Props) {
  const [databases, setDatabases] = useState<string[]>([]);
  const [error, setError] = useState("");
  const conn = saved.find((c) => c.id === value.connectionId);
  const isSQLite = conn?.config.type === DBType.SQLite;
  // Guards against a slow database list arriving after another pick.
  const picked = useRef("");

  const id = value.connectionId;
  useEffect(() => {
    picked.current = id;
    setDatabases([]);
    setError("");
    if (!conn || conn.config.type === DBType.SQLite) return;
    api
      .listSavedDatabases(id)
      .then((dbs) => {
        if (picked.current !== id) return;
        setDatabases(dbs);
      })
      .catch((e) => {
        if (picked.current === id) setError(errMsg(e));
      });
  }, [id, conn]);

  // Default to the connection's own database once the list is in.
  useEffect(() => {
    if (!conn || isSQLite || value.database || databases.length === 0) return;
    const d = conn.config.database;
    onChange({
      connectionId: id,
      database: d && databases.includes(d) ? d : (databases[0] ?? ""),
    });
  }, [databases, conn, isSQLite, value.database, id, onChange]);

  return (
    <fieldset className="card side" disabled={disabled}>
      <legend>{label}</legend>
      <label className="field">
        <span className="label">Connection</span>
        <select
          value={value.connectionId}
          onChange={(e) =>
            onChange({ connectionId: e.target.value, database: "" })
          }
        >
          <option value="">Choose a saved connection…</option>
          {saved.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name} ({DB_LABEL[c.config.type]})
            </option>
          ))}
        </select>
      </label>
      {conn && !isSQLite && (
        <label className="field">
          <span className="label">Database</span>
          <select
            value={value.database}
            onChange={(e) =>
              onChange({ connectionId: id, database: e.target.value })
            }
            disabled={databases.length === 0}
          >
            {value.database === "" && <option value="">Loading…</option>}
            {databases.map((d) => (
              <option key={d} value={d}>
                {d}
              </option>
            ))}
          </select>
        </label>
      )}
      {error && <p className="err">{error}</p>}
    </fieldset>
  );
}
