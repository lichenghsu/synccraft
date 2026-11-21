import { useState } from "react";
import { api, DBType, errMsg, FileKind, isCancel, type Connection, type ConnectionConfig, type TLSFiles } from "../../api/wails";
import { useCancellable } from "../../hooks/useCancellable";
import { FileField } from "./FileField";
import { cleanConfig, DB_LABEL, DB_TYPES, DEFAULT_PORT, SSL_MODES, toDraft, type FormState } from "./model";

type Props = {
  form: FormState;
  onChange: (f: FormState) => void;
  onSaved: (c: Connection) => void;
  onDeleted: (id: string) => void;
};

type Notice = { kind: "ok" | "err"; text: string } | null;

export function ConnectionForm({ form, onChange, onSaved, onDeleted }: Props) {
  const { config } = form;
  const [notice, setNotice] = useState<Notice>(null);
  const [databases, setDatabases] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const test = useCancellable(api.testConnection);

  const set = (patch: Partial<ConnectionConfig>) => onChange({ ...form, config: { ...config, ...patch } });
  const setTLS = (patch: Partial<TLSFiles>) => set({ tls: { ...config.tls, ...patch } });
  const isSQLite = config.type === DBType.SQLite;

  const run = async (label: string, fn: () => Promise<string>) => {
    setNotice(null);
    setBusy(true);
    try {
      setNotice({ kind: "ok", text: await fn() });
    } catch (e) {
      setNotice(isCancel(e) ? { kind: "err", text: `${label} cancelled` } : { kind: "err", text: errMsg(e) });
    } finally {
      setBusy(false);
    }
  };

  const onTest = () =>
    run("Test", async () => {
      await test.run(toDraft(form));
      return "Connection OK";
    });

  const onLoadDatabases = () =>
    run("Load", async () => {
      const dbs = await api.listDatabases(toDraft(form));
      setDatabases(dbs);
      return dbs.length ? `${dbs.length} database(s) found` : "No user databases on this server";
    });

  const onSave = () =>
    run("Save", async () => {
      const draft = toDraft(form);
      const saved = await api.saveConnection({ id: form.id, name: form.name, config: draft.config }, draft.updateSecrets);
      onSaved(saved);
      return "Saved";
    });

  const onDelete = () => {
    if (!confirmDelete) {
      setConfirmDelete(true);
      return;
    }
    setConfirmDelete(false);
    void run("Delete", async () => {
      await api.deleteConnection(form.id);
      onDeleted(form.id);
      return "Deleted";
    });
  };

  const changeType = (type: DBType) => {
    setDatabases([]);
    onChange({ ...form, config: cleanConfig({ ...config, type, port: 0 }) });
  };

  return (
    <form
      className="card conn-form"
      onSubmit={(e) => {
        e.preventDefault();
        void onSave();
      }}
    >
      <h2>{form.id ? "Edit connection" : "New connection"}</h2>

      <label className="field">
        <span className="label">Name</span>
        <input value={form.name} onChange={(e) => onChange({ ...form, name: e.target.value })} placeholder="e.g. shop (local)" required />
      </label>

      <div className="field">
        <span className="label">Database type</span>
        <div className="segmented" role="radiogroup" aria-label="Database type">
          {DB_TYPES.map((t) => (
            <button
              key={t}
              type="button"
              role="radio"
              aria-checked={config.type === t}
              className={config.type === t ? "on" : ""}
              onClick={() => changeType(t)}
            >
              {DB_LABEL[t]}
            </button>
          ))}
        </div>
      </div>

      {isSQLite ? (
        <FileField
          label="Database file"
          value={config.filePath}
          choices={[
            { kind: FileKind.FileSQLiteOpen, label: "Open…" },
            { kind: FileKind.FileSQLiteCreate, label: "New…" },
          ]}
          onChange={(filePath) => set({ filePath })}
        />
      ) : (
        <>
          <div className="grid2">
            <label className="field grow">
              <span className="label">Host</span>
              <input value={config.host} onChange={(e) => set({ host: e.target.value })} required />
            </label>
            <label className="field port">
              <span className="label">Port</span>
              <input
                type="number"
                min={0}
                max={65535}
                value={config.port || ""}
                placeholder={String(DEFAULT_PORT[config.type])}
                onChange={(e) => set({ port: Number(e.target.value) || 0 })}
              />
            </label>
          </div>

          <div className="grid2">
            <label className="field grow">
              <span className="label">User</span>
              <input value={config.user} onChange={(e) => set({ user: e.target.value })} autoComplete="off" required />
            </label>
            <label className="field grow">
              <span className="label">Password</span>
              <input
                type="password"
                autoComplete="new-password"
                value={form.secretsDirty ? config.password : ""}
                placeholder={form.id && !form.secretsDirty ? "Saved — leave empty to keep" : ""}
                onChange={(e) => onChange({ ...form, secretsDirty: true, config: { ...config, password: e.target.value } })}
              />
            </label>
          </div>
          {form.id && form.secretsDirty && (
            <p className="hint">
              {config.password ? "The new password replaces the saved one." : "Saving clears the saved password."}{" "}
              <button type="button" className="link" onClick={() => onChange({ ...form, secretsDirty: false, config: { ...config, password: "" } })}>
                Keep saved password
              </button>
            </p>
          )}

          <label className="field">
            <span className="label">Database</span>
            <div className="row">
              <input list="conn-databases" value={config.database} onChange={(e) => set({ database: e.target.value })} placeholder="optional" />
              <datalist id="conn-databases">
                {databases.map((db) => (
                  <option key={db} value={db} />
                ))}
              </datalist>
              <button type="button" className="ghost" onClick={() => void onLoadDatabases()} disabled={busy}>
                Load list
              </button>
            </div>
          </label>

          {config.type === DBType.PostgreSQL && (
            <label className="field">
              <span className="label">SSL mode</span>
              <select value={config.sslMode || "prefer"} onChange={(e) => set({ sslMode: e.target.value })}>
                {SSL_MODES.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            </label>
          )}

          {config.type === DBType.SQLServer && (
            <label className="check">
              <input type="checkbox" checked={!!config.trustServerCertificate} onChange={(e) => set({ trustServerCertificate: e.target.checked })} />
              Trust server certificate (only for self-signed certificates you control)
            </label>
          )}

          {config.type !== DBType.MySQL && (
            <details className="section" open={!!config.tls}>
              <summary>TLS certificates</summary>
              <FileField
                label="CA certificate"
                optional
                value={config.tls?.caCertPath}
                choices={[{ kind: FileKind.FileCACert, label: "Choose…" }]}
                onChange={(caCertPath) => setTLS({ caCertPath })}
              />
              {config.type === DBType.PostgreSQL && (
                <>
                  <FileField
                    label="Client certificate"
                    optional
                    value={config.tls?.clientCertPath}
                    choices={[{ kind: FileKind.FileClientCert, label: "Choose…" }]}
                    onChange={(clientCertPath) => setTLS({ clientCertPath })}
                  />
                  <FileField
                    label="Client key"
                    optional
                    value={config.tls?.clientKeyPath}
                    choices={[{ kind: FileKind.FileClientKey, label: "Choose…" }]}
                    onChange={(clientKeyPath) => setTLS({ clientKeyPath })}
                  />
                </>
              )}
            </details>
          )}
          {config.type === DBType.MySQL && <p className="hint">TLS certificates and SSH tunnels for MySQL are coming soon.</p>}
        </>
      )}

      <div className="actions">
        {test.running ? (
          <button type="button" className="ghost" onClick={test.cancel}>
            Cancel test
          </button>
        ) : (
          <button type="button" className="ghost" onClick={() => void onTest()} disabled={busy}>
            Test connection
          </button>
        )}
        <button type="submit" disabled={busy || !form.name.trim()}>
          {form.id ? "Save changes" : "Save"}
        </button>
        {form.id && (
          <button type="button" className={confirmDelete ? "danger" : "ghost danger-text"} onClick={onDelete} onBlur={() => setConfirmDelete(false)} disabled={busy}>
            {confirmDelete ? "Click again to delete" : "Delete"}
          </button>
        )}
      </div>

      {notice && (
        <p className={notice.kind} role="status">
          {notice.text}
        </p>
      )}
    </form>
  );
}
