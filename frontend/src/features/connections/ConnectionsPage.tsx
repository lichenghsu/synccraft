import { useCallback, useEffect, useState } from "react";
import { api, errMsg, type Connection } from "../../api/wails";
import { ConnectionForm } from "./ConnectionForm";
import { DB_LABEL, formFromSaved, newForm, type FormState } from "./model";

export function ConnectionsPage() {
  const [saved, setSaved] = useState<Connection[]>([]);
  const [loadError, setLoadError] = useState("");
  const [form, setForm] = useState<FormState>(newForm);
  // Remounting the form on selection resets its notices and database list.
  const [formKey, setFormKey] = useState(0);

  const refresh = useCallback(async () => {
    try {
      setSaved(await api.listSaved());
      setLoadError("");
    } catch (e) {
      setLoadError(errMsg(e));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const open = (f: FormState) => {
    setForm(f);
    setFormKey((k) => k + 1);
  };

  return (
    <div className="conn-page">
      <aside className="card conn-list">
        <div className="row between">
          <h2>Connections</h2>
          <button type="button" className="ghost small" onClick={() => open(newForm())}>
            + New
          </button>
        </div>
        {loadError && <p className="err">{loadError}</p>}
        {!loadError && saved.length === 0 && <p className="hint">No saved connections yet.</p>}
        <ul>
          {saved.map((c) => (
            <li key={c.id}>
              <button type="button" className={`item ${form.id === c.id ? "on" : ""}`} onClick={() => open(formFromSaved(c))}>
                <span className="name">{c.name}</span>
                <span className="meta">
                  {DB_LABEL[c.config.type]} · {c.config.filePath || `${c.config.host}${c.config.database ? "/" + c.config.database : ""}`}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </aside>

      <ConnectionForm
        key={formKey}
        form={form}
        onChange={setForm}
        onSaved={(c) => {
          setForm(formFromSaved(c));
          void refresh();
        }}
        onDeleted={() => {
          open(newForm());
          void refresh();
        }}
      />
    </div>
  );
}
