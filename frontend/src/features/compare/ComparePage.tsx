import { useCallback, useEffect, useMemo, useState } from "react";
import {
  api,
  errMsg,
  isCancel,
  type Comparison,
  type Connection,
  type Side,
  type Step,
} from "../../api/wails";
import { useCancellable } from "../../hooks/useCancellable";
import { DB_LABEL } from "../connections/model";
import { SidePicker } from "./SidePicker";
import {
  defaultSelection,
  describeChange,
  emptySide,
  groupByTable,
  scriptText,
  sideReady,
} from "./model";

export function ComparePage() {
  const [saved, setSaved] = useState<Connection[]>([]);
  const [source, setSource] = useState<Side>(emptySide);
  const [target, setTarget] = useState<Side>(emptySide);
  const [result, setResult] = useState<Comparison | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [preview, setPreview] = useState<Step[]>([]);
  const [confirming, setConfirming] = useState(false);
  const [applying, setApplying] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  const compare = useCancellable(api.compare);
  const previewer = useCancellable(api.previewChanges);

  useEffect(() => {
    api
      .listSaved()
      .then(setSaved)
      .catch((e) => setError(errMsg(e)));
  }, []);

  const srcConn = saved.find((c) => c.id === source.connectionId);
  const dstConn = saved.find((c) => c.id === target.connectionId);
  const mismatch =
    !!srcConn && !!dstConn && srcConn.config.type !== dstConn.config.type;
  const same =
    source.connectionId !== "" &&
    source.connectionId === target.connectionId &&
    source.database === target.database;
  const ready =
    sideReady(source, saved) && sideReady(target, saved) && !mismatch && !same;

  // Any change to either side invalidates the result.
  const pickSource = useCallback((s: Side) => {
    setSource(s);
    setResult(null);
  }, []);
  const pickTarget = useCallback((s: Side) => {
    setTarget(s);
    setResult(null);
  }, []);

  const runCompare = compare.run;
  const doCompare = useCallback(
    async (keepNotice = false) => {
      setError("");
      if (!keepNotice) setNotice("");
      setConfirming(false);
      setResult(null);
      try {
        const r = await runCompare(source, target);
        setResult(r);
        setSelected(defaultSelection(r.steps ?? []));
      } catch (e) {
        if (!isCancel(e)) setError(errMsg(e));
      }
    },
    [runCompare, source, target],
  );

  const steps = useMemo(() => result?.steps ?? [], [result]);
  const ids = useMemo(
    () =>
      steps.filter((s) => selected.has(s.change.id)).map((s) => s.change.id),
    [steps, selected],
  );

  // The SQL depends on the selection (SQLite rebuilds see earlier steps).
  const runPreview = previewer.run;
  useEffect(() => {
    setPreview([]);
    if (!result || ids.length === 0) return;
    runPreview(result.token, ids)
      .then(setPreview)
      .catch((e) => {
        if (!isCancel(e)) setError(errMsg(e));
      });
  }, [result, ids, runPreview]);

  const toggle = (id: string) =>
    setSelected((cur) => {
      const next = new Set(cur);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const selectWhere = (keep: (s: Step) => boolean) =>
    setSelected(
      new Set(
        steps.filter((s) => !s.unsupported && keep(s)).map((s) => s.change.id),
      ),
    );

  const destructiveCount = steps.filter(
    (s) => selected.has(s.change.id) && s.change.destructive,
  ).length;
  const script = scriptText(preview);
  const blocked = preview.some((s) => s.unsupported);

  const doApply = async () => {
    if (!result) return;
    setApplying(true);
    setError("");
    try {
      await api.applyChanges(result.token, ids);
      setNotice(`Applied ${ids.length} change${ids.length === 1 ? "" : "s"}.`);
      await doCompare(true);
    } catch (e) {
      setError(errMsg(e));
      setResult(null); // the token is spent either way
    } finally {
      setApplying(false);
      setConfirming(false);
    }
  };

  const targetLabel = dstConn
    ? `${dstConn.name}${target.database ? ` / ${target.database}` : ""}`
    : "";

  return (
    <div className="compare-page">
      <div className="compare-bar">
        <SidePicker
          label="Source (reference)"
          saved={saved}
          value={source}
          onChange={pickSource}
          disabled={applying}
        />
        <div className="compare-mid">
          <button
            type="button"
            className="ghost small"
            title="Swap source and target"
            disabled={applying}
            onClick={() => {
              setSource(target);
              setTarget(source);
              setResult(null);
            }}
          >
            ⇄
          </button>
        </div>
        <SidePicker
          label="Target (gets changed)"
          saved={saved}
          value={target}
          onChange={pickTarget}
          disabled={applying}
        />
      </div>

      <div className="row wrap">
        {compare.running ? (
          <button type="button" className="ghost" onClick={compare.cancel}>
            Cancel
          </button>
        ) : (
          <button
            type="button"
            disabled={!ready || applying}
            onClick={() => void doCompare()}
          >
            Compare
          </button>
        )}
        {compare.running && <span className="hint">Reading both schemas…</span>}
        {mismatch && (
          <span className="err">
            Source is {DB_LABEL[srcConn!.config.type]}, target is{" "}
            {DB_LABEL[dstConn!.config.type]}: only the same engine can be
            compared.
          </span>
        )}
        {same && (
          <span className="err">Source and target are the same database.</span>
        )}
        {saved.length === 0 && !error && (
          <span className="hint">
            Save a connection on the Connections page first.
          </span>
        )}
      </div>
      {notice && <p className="ok">{notice}</p>}
      {error && <p className="err">{error}</p>}

      {result && steps.length === 0 && (
        <div className="card">
          <p className="ok">The schemas match. Nothing to change.</p>
        </div>
      )}

      {result && steps.length > 0 && (
        <div className="compare-body">
          <section className="card changes">
            <div className="row between wrap">
              <h2>
                {steps.length} change{steps.length === 1 ? "" : "s"}
              </h2>
              <div className="row">
                <button
                  type="button"
                  className="ghost small"
                  onClick={() => selectWhere((s) => !s.change.destructive)}
                >
                  Safe only
                </button>
                <button
                  type="button"
                  className="ghost small"
                  onClick={() => selectWhere(() => true)}
                >
                  All
                </button>
                <button
                  type="button"
                  className="ghost small"
                  onClick={() => setSelected(new Set())}
                >
                  None
                </button>
              </div>
            </div>
            {groupByTable(steps).map((g, i) => (
              <div key={`${i}:${g.label}`} className="change-group">
                <h3 className="mono">{g.label}</h3>
                <ul>
                  {g.steps.map((s) => (
                    <li
                      key={s.change.id}
                      className={s.change.destructive ? "destructive" : ""}
                    >
                      <label className="check">
                        <input
                          type="checkbox"
                          checked={selected.has(s.change.id)}
                          disabled={!!s.unsupported || applying}
                          onChange={() => toggle(s.change.id)}
                        />
                        <span className="mono">{describeChange(s.change)}</span>
                        {s.change.destructive && (
                          <span className="badge danger">may lose data</span>
                        )}
                      </label>
                      {s.unsupported && (
                        <p className="hint">Not supported: {s.unsupported}</p>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </section>

          <section className="card sql-preview">
            <div className="row between">
              <h2>SQL</h2>
              <button
                type="button"
                className="ghost small"
                disabled={!script}
                onClick={() => void navigator.clipboard.writeText(script)}
              >
                Copy
              </button>
            </div>
            {ids.length === 0 ? (
              <p className="hint">Select changes to see their SQL.</p>
            ) : (
              <pre className="mono">
                {previewer.running && !script ? "…" : script}
              </pre>
            )}

            {!result.transactionalDdl && (
              <p className="warn">
                MySQL / MariaDB commit every DDL statement on its own. If a
                statement fails, the ones before it stay applied.
              </p>
            )}

            {!confirming ? (
              <div className="actions">
                <button
                  type="button"
                  disabled={
                    ids.length === 0 || blocked || previewer.running || applying
                  }
                  onClick={() => setConfirming(true)}
                >
                  Apply {ids.length} change{ids.length === 1 ? "" : "s"}…
                </button>
              </div>
            ) : (
              <div className="confirm">
                <p>
                  Run {ids.length} change{ids.length === 1 ? "" : "s"} on{" "}
                  <strong>{targetLabel}</strong>
                  {destructiveCount > 0 && (
                    <>
                      , including{" "}
                      <strong className="danger-text">
                        {destructiveCount} that may lose data
                      </strong>
                    </>
                  )}
                  ?
                </p>
                <div className="row">
                  <button
                    type="button"
                    className={destructiveCount > 0 ? "danger" : ""}
                    disabled={applying}
                    onClick={() => void doApply()}
                  >
                    {applying ? "Applying…" : "Confirm"}
                  </button>
                  <button
                    type="button"
                    className="ghost"
                    disabled={applying}
                    onClick={() => setConfirming(false)}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
