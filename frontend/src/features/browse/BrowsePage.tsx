import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  DBType,
  errMsg,
  isCancel,
  type Connection,
  type Table,
} from "../../api/wails";
import { useCancellable } from "../../hooks/useCancellable";
import { DB_LABEL } from "../connections/model";
import { TableData } from "./TableData";
import { TableStructure } from "./TableStructure";
import { sameTable, tableLabel } from "./model";

type Tab = "structure" | "data";

export function BrowsePage() {
  const [saved, setSaved] = useState<Connection[]>([]);
  const [connId, setConnId] = useState("");
  const [databases, setDatabases] = useState<string[]>([]);
  const [database, setDatabase] = useState("");
  const [tables, setTables] = useState<Table[] | null>(null);
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<Table | null>(null);
  const [tab, setTab] = useState<Tab>("structure");
  const [error, setError] = useState("");

  const conn = saved.find((c) => c.id === connId);
  const isSQLite = conn?.config.type === DBType.SQLite;

  const loadTables = useCancellable(api.listTables);
  // Guards against a slow database list arriving after another pick.
  const picked = useRef("");

  useEffect(() => {
    api
      .listSaved()
      .then(setSaved)
      .catch((e) => setError(errMsg(e)));
  }, []);

  const pickConnection = useCallback(
    async (id: string) => {
      picked.current = id;
      setConnId(id);
      setDatabases([]);
      setDatabase("");
      setTables(null);
      setSelected(null);
      setError("");
      const c = saved.find((x) => x.id === id);
      if (!c || c.config.type === DBType.SQLite) return;
      try {
        const dbs = await api.listSavedDatabases(id);
        if (picked.current !== id) return;
        setDatabases(dbs);
        setDatabase(c.config.database || dbs[0] || "");
      } catch (e) {
        if (picked.current === id) setError(errMsg(e));
      }
    },
    [saved],
  );

  // SQLite has one database per file, so it is browsed with database "".
  const ready = conn !== undefined && (isSQLite || database !== "");
  const run = loadTables.run;
  useEffect(() => {
    if (!ready) return;
    setTables(null);
    setSelected(null);
    run(connId, database)
      .then((ts) => {
        setTables(ts);
        setError("");
      })
      .catch((e) => {
        if (!isCancel(e)) setError(errMsg(e));
      });
  }, [ready, connId, database, run]);

  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return (tables ?? []).filter((t) =>
      tableLabel(t).toLowerCase().includes(q),
    );
  }, [tables, filter]);

  return (
    <div className="browse-page">
      <div className="card browse-bar">
        <label className="field">
          <span className="label">Connection</span>
          <select
            value={connId}
            onChange={(e) => void pickConnection(e.target.value)}
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
              value={database}
              onChange={(e) => setDatabase(e.target.value)}
              disabled={databases.length === 0}
            >
              {databases.map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </select>
          </label>
        )}
        {error && <p className="err">{error}</p>}
        {saved.length === 0 && !error && (
          <p className="hint">
            Save a connection on the Connections page first.
          </p>
        )}
      </div>

      {ready && (
        <div className="browse-body">
          <aside className="card table-list">
            <input
              type="search"
              placeholder="Filter tables"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
            {loadTables.running && <p className="hint">Loading tables…</p>}
            {tables && tables.length === 0 && (
              <p className="hint">No tables in this database.</p>
            )}
            <ul>
              {shown.map((t) => (
                <li key={tableLabel(t)}>
                  <button
                    type="button"
                    className={`item ${selected && sameTable(selected, t) ? "on" : ""}`}
                    onClick={() => setSelected(t)}
                    title={tableLabel(t)}
                  >
                    {t.schema && <span className="muted">{t.schema}.</span>}
                    {t.name}
                  </button>
                </li>
              ))}
            </ul>
          </aside>

          <section className="card table-view">
            {!selected ? (
              <p className="hint">Pick a table on the left.</p>
            ) : (
              <>
                <div className="row between">
                  <h2>{tableLabel(selected)}</h2>
                  <div className="segmented">
                    <button
                      type="button"
                      className={tab === "structure" ? "on" : ""}
                      onClick={() => setTab("structure")}
                    >
                      Structure
                    </button>
                    <button
                      type="button"
                      className={tab === "data" ? "on" : ""}
                      onClick={() => setTab("data")}
                    >
                      Data
                    </button>
                  </div>
                </div>
                {/* Keyed so switching tables resets paging and notices. */}
                {tab === "structure" ? (
                  <TableStructure
                    key={tableLabel(selected)}
                    connId={connId}
                    database={database}
                    table={selected}
                  />
                ) : (
                  <TableData
                    key={tableLabel(selected)}
                    connId={connId}
                    database={database}
                    table={selected}
                  />
                )}
              </>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
