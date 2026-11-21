import { useState } from "react";
import { api, errMsg, FileKind } from "../../api/wails";

type Choice = { kind: FileKind; label: string };

type Props = {
  label: string;
  value: string | undefined;
  choices: Choice[];
  onChange: (path: string) => void;
  optional?: boolean;
};

// Read-only path with dialog buttons: paths can only come from the Go-side
// file dialog, which also allowlists them in the backend.
export function FileField({ label, value, choices, onChange, optional }: Props) {
  const [error, setError] = useState("");

  const pick = async (kind: FileKind) => {
    setError("");
    try {
      const path = await api.pickFile(kind);
      if (path) onChange(path);
    } catch (e) {
      setError(errMsg(e));
    }
  };

  return (
    <div className="field">
      <span className="label">{label}</span>
      <div className="row">
        <output className="path" title={value || undefined}>
          {value || <span className="muted">{optional ? "None" : "No file chosen"}</span>}
        </output>
        {choices.map((c) => (
          <button key={c.kind} type="button" className="ghost" onClick={() => void pick(c.kind)}>
            {c.label}
          </button>
        ))}
        {optional && value && (
          <button type="button" className="ghost" onClick={() => onChange("")} aria-label={`Clear ${label}`}>
            Clear
          </button>
        )}
      </div>
      {error && <p className="err">{error}</p>}
    </div>
  );
}
