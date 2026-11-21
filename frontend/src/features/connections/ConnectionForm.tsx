import { useState } from "react";
import {
  api,
  DBType,
  errMsg,
  FileKind,
  isCancel,
  SSHAuthMethod,
  type Connection,
  type ConnectionConfig,
  type SSHTunnel,
  type TLSFiles,
} from "../../api/wails";
import { useCancellable } from "../../hooks/useCancellable";
import { FileField } from "./FileField";
import { K8sSection } from "./K8sSection";
import { SecretField } from "./SecretField";
import {
  cleanConfig,
  DB_LABEL,
  DB_TYPES,
  DEFAULT_PORT,
  newK8s,
  newSSH,
  ROUTES,
  SECRET,
  SOCKET_PLACEHOLDER,
  supportsSocket,
  SSH_AUTHS,
  SSL_MODES,
  toDraft,
  unusedSSHSecrets,
  type FormState,
  type Route,
  type SecretName,
} from "./model";

type Props = {
  form: FormState;
  onChange: (f: FormState) => void;
  onSaved: (c: Connection) => void;
  onDeleted: (id: string) => void;
};

type Notice = { kind: "ok" | "err"; text: string } | null;

export function ConnectionForm({ form, onChange, onSaved, onDeleted }: Props) {
  const { config } = form;
  const ssh = form.route === "ssh" ? (config.ssh ?? null) : null;
  const k8s = form.route === "k8s" ? (config.k8s ?? null) : null;
  // CNPG supplies host, credentials and CA, so those fields are hidden.
  const cnpgMode =
    !!k8s && form.k8sTarget === "cnpg" && config.type === DBType.PostgreSQL;
  const [notice, setNotice] = useState<Notice>(null);
  const [databases, setDatabases] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const test = useCancellable(api.testConnection);

  const set = (patch: Partial<ConnectionConfig>) =>
    onChange({ ...form, config: { ...config, ...patch } });
  const setTLS = (patch: Partial<TLSFiles>) =>
    set({ tls: { ...config.tls, ...patch } });
  const setSSH = (patch: Partial<SSHTunnel>) =>
    set({ ssh: { ...(ssh ?? newSSH()), ...patch } });
  const isSQLite = config.type === DBType.SQLite;
  const saved = form.id !== "";
  const socketMode = !!ssh && form.viaSocket && supportsSocket(config.type);

  const isDirty = (name: SecretName) => form.dirtySecrets.includes(name);
  const markDirty = (names: SecretName[], patch: Partial<ConnectionConfig>) =>
    onChange({
      ...form,
      dirtySecrets: [...new Set([...form.dirtySecrets, ...names])],
      config: { ...config, ...patch },
    });
  const revert = (name: SecretName, patch: Partial<ConnectionConfig>) =>
    onChange({
      ...form,
      dirtySecrets: form.dirtySecrets.filter((n) => n !== name),
      config: { ...config, ...patch },
    });

  const run = async (label: string, fn: () => Promise<string>) => {
    setNotice(null);
    setBusy(true);
    try {
      setNotice({ kind: "ok", text: await fn() });
    } catch (e) {
      setNotice(
        isCancel(e)
          ? { kind: "err", text: `${label} cancelled` }
          : { kind: "err", text: errMsg(e) },
      );
    } finally {
      setBusy(false);
    }
  };

  // In socket mode with an empty path, find the socket first and fill it in.
  const withSocket = async (): Promise<{ f: FormState; found: string }> => {
    if (!socketMode || config.socketPath?.trim()) return { f: form, found: "" };
    const found = await api.detectSocket(toDraft(form));
    const f = { ...form, config: { ...config, socketPath: found } };
    onChange(f);
    return { f, found };
  };
  const foundNote = (found: string) =>
    found ? ` (socket found at ${found})` : "";

  const onDetect = () =>
    run("Detect", async () => {
      const found = await api.detectSocket(toDraft(form));
      set({ socketPath: found });
      return `Socket found at ${found}`;
    });

  const onTest = () =>
    run("Test", async () => {
      const { f, found } = await withSocket();
      const info = await test.run(toDraft(f));
      return `Connection OK · ${info.product} ${info.version}${foundNote(found)}`;
    });

  const onLoadDatabases = () =>
    run("Load", async () => {
      const { f, found } = await withSocket();
      const dbs = await api.listDatabases(toDraft(f));
      setDatabases(dbs);
      return (
        (dbs.length
          ? `${dbs.length} database(s) found`
          : "No user databases on this server") + foundNote(found)
      );
    });

  const onSave = () =>
    run("Save", async () => {
      const { f } = await withSocket();
      const draft = toDraft(f);
      const result = await api.saveConnection(
        { id: f.id, name: f.name, config: draft.config },
        draft.updateSecrets ?? [],
      );
      onSaved(result);
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
    // Database names don't carry over between engines.
    onChange({
      ...form,
      config: cleanConfig({ ...config, type, port: 0, database: "" }),
    });
  };

  // Switching routes keeps the other routes' fields; toDraft drops them.
  const changeRoute = (route: Route) =>
    onChange({
      ...form,
      route,
      config: {
        ...config,
        ssh: config.ssh ?? (route === "ssh" ? newSSH() : null),
        k8s: config.k8s ?? (route === "k8s" ? newK8s() : null),
        // With a tunnel, 127.0.0.1 means the SSH server itself.
        host:
          route === "ssh" && !config.host.trim() ? "127.0.0.1" : config.host,
      },
    });

  const changeSSHAuth = (auth: SSHAuthMethod) => {
    const cleared = unusedSSHSecrets(auth);
    const next = { ...(ssh ?? newSSH()), auth };
    // Saved secrets the new method doesn't use get deleted on save.
    onChange({
      ...form,
      dirtySecrets: saved
        ? [...new Set([...form.dirtySecrets, ...cleared])]
        : form.dirtySecrets,
      config: { ...config, ssh: next },
    });
  };

  return (
    <form
      className="card conn-form"
      onSubmit={(e) => {
        e.preventDefault();
        void onSave();
      }}
    >
      <h2>{saved ? "Edit connection" : "New connection"}</h2>

      <label className="field">
        <span className="label">Name</span>
        <input
          value={form.name}
          onChange={(e) => onChange({ ...form, name: e.target.value })}
          placeholder="e.g. shop (local)"
          required
        />
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
          <div className="field">
            <span className="label">Connect</span>
            <div className="segmented" role="radiogroup" aria-label="Route">
              {ROUTES.map((r) => (
                <button
                  key={r.value}
                  type="button"
                  role="radio"
                  aria-checked={form.route === r.value}
                  className={form.route === r.value ? "on" : ""}
                  onClick={() => changeRoute(r.value)}
                >
                  {r.label}
                </button>
              ))}
            </div>
          </div>

          {k8s && (
            <K8sSection
              dbType={config.type}
              k8s={k8s}
              target={form.k8sTarget}
              onChange={(next) => set({ k8s: next })}
              onTargetChange={(k8sTarget) => onChange({ ...form, k8sTarget })}
            />
          )}

          {ssh && (
            <div className="section">
              {ssh && (
                <div className="ssh-fields">
                  <div className="grid2">
                    <label className="field grow">
                      <span className="label">SSH host</span>
                      <input
                        value={ssh.host}
                        onChange={(e) => setSSH({ host: e.target.value })}
                        required
                      />
                    </label>
                    <label className="field port">
                      <span className="label">Port</span>
                      <input
                        type="number"
                        min={0}
                        max={65535}
                        value={ssh.port || ""}
                        placeholder="22"
                        onChange={(e) =>
                          setSSH({ port: Number(e.target.value) || 0 })
                        }
                      />
                    </label>
                  </div>
                  <label className="field">
                    <span className="label">SSH user</span>
                    <input
                      value={ssh.user}
                      onChange={(e) => setSSH({ user: e.target.value })}
                      autoComplete="off"
                      required
                    />
                  </label>

                  <div className="field">
                    <span className="label">Authentication</span>
                    <div
                      className="segmented"
                      role="radiogroup"
                      aria-label="SSH authentication"
                    >
                      {SSH_AUTHS.map((a) => (
                        <button
                          key={a.value}
                          type="button"
                          role="radio"
                          aria-checked={ssh.auth === a.value}
                          className={ssh.auth === a.value ? "on" : ""}
                          onClick={() => changeSSHAuth(a.value)}
                        >
                          {a.label}
                        </button>
                      ))}
                    </div>
                  </div>

                  {ssh.auth === SSHAuthMethod.SSHAuthPassword && (
                    <SecretField
                      label="SSH password"
                      value={ssh.password}
                      saved={saved}
                      dirty={isDirty(SECRET.sshPassword)}
                      onChange={(password) =>
                        markDirty([SECRET.sshPassword], {
                          ssh: { ...ssh, password },
                        })
                      }
                      onRevert={() =>
                        revert(SECRET.sshPassword, {
                          ssh: { ...ssh, password: "" },
                        })
                      }
                    />
                  )}
                  {ssh.auth === SSHAuthMethod.SSHAuthPrivateKey && (
                    <>
                      <FileField
                        label="Private key"
                        value={ssh.privateKeyPath}
                        choices={[
                          {
                            kind: FileKind.FileSSHPrivateKey,
                            label: "Choose…",
                          },
                        ]}
                        onChange={(privateKeyPath) =>
                          setSSH({ privateKeyPath })
                        }
                      />
                      <SecretField
                        label="Passphrase (if the key is encrypted)"
                        value={ssh.passphrase}
                        saved={saved}
                        dirty={isDirty(SECRET.sshPassphrase)}
                        onChange={(passphrase) =>
                          markDirty([SECRET.sshPassphrase], {
                            ssh: { ...ssh, passphrase },
                          })
                        }
                        onRevert={() =>
                          revert(SECRET.sshPassphrase, {
                            ssh: { ...ssh, passphrase: "" },
                          })
                        }
                      />
                    </>
                  )}
                  {ssh.auth === SSHAuthMethod.SSHAuthAgent && (
                    <p className="hint">
                      Uses the keys loaded in your SSH agent (SSH_AUTH_SOCK).
                    </p>
                  )}

                  <FileField
                    label="known_hosts (default: ~/.ssh/known_hosts)"
                    optional
                    value={ssh.knownHostsPath}
                    choices={[
                      { kind: FileKind.FileKnownHosts, label: "Choose…" },
                    ]}
                    onChange={(knownHostsPath) => setSSH({ knownHostsPath })}
                  />
                  <p className="hint">
                    The server's host key must already be in known_hosts.
                    Connect once with ssh to verify and add it.
                  </p>
                </div>
              )}
            </div>
          )}

          {ssh && supportsSocket(config.type) && (
            <div className="field">
              <span className="label">Reach the database via</span>
              <div
                className="segmented"
                role="radiogroup"
                aria-label="Database transport"
              >
                <button
                  type="button"
                  role="radio"
                  aria-checked={!form.viaSocket}
                  className={!form.viaSocket ? "on" : ""}
                  onClick={() => onChange({ ...form, viaSocket: false })}
                >
                  TCP host:port
                </button>
                <button
                  type="button"
                  role="radio"
                  aria-checked={form.viaSocket}
                  className={form.viaSocket ? "on" : ""}
                  onClick={() => onChange({ ...form, viaSocket: true })}
                >
                  Unix socket
                </button>
              </div>
            </div>
          )}
          {socketMode && (
            <label className="field">
              <span className="label">
                {config.type === DBType.PostgreSQL
                  ? "Socket directory on the SSH server"
                  : "Socket path on the SSH server"}
              </span>
              <div className="row">
                <input
                  value={config.socketPath ?? ""}
                  placeholder={`empty = detect (e.g. ${SOCKET_PLACEHOLDER[config.type]})`}
                  onChange={(e) => set({ socketPath: e.target.value })}
                />
                <button
                  type="button"
                  className="ghost"
                  onClick={() => void onDetect()}
                  disabled={busy}
                >
                  Detect
                </button>
              </div>
              <span className="hint">
                Logs in as the SSH user's OS account, so accounts that only
                allow local socket logins (e.g. root on Debian/Ubuntu MariaDB,
                PostgreSQL peer) work without a password when the names match.
              </span>
            </label>
          )}

          {!socketMode && !k8s && (
            <div className="grid2">
              <label className="field grow">
                <span className="label">
                  Host
                  {ssh &&
                    " (as seen from the SSH server; 127.0.0.1 = the SSH server itself)"}
                </span>
                <input
                  value={config.host}
                  onChange={(e) => set({ host: e.target.value })}
                  required
                />
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
          )}

          {!cnpgMode && (
            <div className="grid2">
              <label className="field grow">
                <span className="label">Database user</span>
                <input
                  value={config.user}
                  onChange={(e) => set({ user: e.target.value })}
                  autoComplete="off"
                  required={!socketMode}
                  placeholder={
                    socketMode
                      ? `defaults to SSH user${ssh?.user ? ` (${ssh.user})` : ""}`
                      : ""
                  }
                />
              </label>
              <SecretField
                label="Password"
                value={config.password}
                saved={saved}
                dirty={isDirty(SECRET.password)}
                onChange={(password) =>
                  markDirty([SECRET.password], { password })
                }
                onRevert={() => revert(SECRET.password, { password: "" })}
              />
            </div>
          )}

          <label className="field">
            <span className="label">Database</span>
            <div className="row">
              <input
                list="conn-databases"
                value={config.database}
                onChange={(e) => set({ database: e.target.value })}
                placeholder={
                  cnpgMode ? "default: the cluster's app database" : "optional"
                }
              />
              <datalist id="conn-databases">
                {databases.map((db) => (
                  <option key={db} value={db} />
                ))}
              </datalist>
              <button
                type="button"
                className="ghost"
                onClick={() => void onLoadDatabases()}
                disabled={busy}
              >
                Load list
              </button>
            </div>
          </label>

          {config.type === DBType.PostgreSQL && !cnpgMode && (
            <label className="field">
              <span className="label">SSL mode</span>
              <select
                value={config.sslMode || "prefer"}
                onChange={(e) => set({ sslMode: e.target.value })}
              >
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
              <input
                type="checkbox"
                checked={!!config.trustServerCertificate}
                onChange={(e) =>
                  set({ trustServerCertificate: e.target.checked })
                }
              />
              Trust server certificate (only for self-signed certificates you
              control)
            </label>
          )}

          {!socketMode && !cnpgMode && (
            <details className="section" open={!!config.tls}>
              <summary>TLS certificates</summary>
              <FileField
                label="CA certificate"
                optional
                value={config.tls?.caCertPath}
                choices={[{ kind: FileKind.FileCACert, label: "Choose…" }]}
                onChange={(caCertPath) => setTLS({ caCertPath })}
              />
              {config.type !== DBType.SQLServer && (
                <>
                  <FileField
                    label="Client certificate"
                    optional
                    value={config.tls?.clientCertPath}
                    choices={[
                      { kind: FileKind.FileClientCert, label: "Choose…" },
                    ]}
                    onChange={(clientCertPath) => setTLS({ clientCertPath })}
                  />
                  <FileField
                    label="Client key"
                    optional
                    value={config.tls?.clientKeyPath}
                    choices={[
                      { kind: FileKind.FileClientKey, label: "Choose…" },
                    ]}
                    onChange={(clientKeyPath) => setTLS({ clientKeyPath })}
                  />
                </>
              )}
            </details>
          )}
        </>
      )}

      <div className="actions">
        {test.running ? (
          <button type="button" className="ghost" onClick={test.cancel}>
            Cancel test
          </button>
        ) : (
          <button
            type="button"
            className="ghost"
            onClick={() => void onTest()}
            disabled={busy}
          >
            Test connection
          </button>
        )}
        <button type="submit" disabled={busy || !form.name.trim()}>
          {saved ? "Save changes" : "Save"}
        </button>
        {saved && (
          <button
            type="button"
            className={confirmDelete ? "danger" : "ghost danger-text"}
            onClick={onDelete}
            onBlur={() => setConfirmDelete(false)}
            disabled={busy}
          >
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
