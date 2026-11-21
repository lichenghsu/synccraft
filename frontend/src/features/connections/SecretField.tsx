type Props = {
  label: string;
  value: string | undefined;
  saved: boolean; // editing a saved connection
  dirty: boolean;
  onChange: (value: string) => void;
  onRevert: () => void;
};

// Password-like field that never shows the stored value. Untouched means
// "keep what is in the keychain".
export function SecretField({
  label,
  value,
  saved,
  dirty,
  onChange,
  onRevert,
}: Props) {
  return (
    <label className="field grow">
      <span className="label">{label}</span>
      <input
        type="password"
        autoComplete="new-password"
        value={dirty ? (value ?? "") : ""}
        placeholder={saved && !dirty ? "Saved — leave empty to keep" : ""}
        onChange={(e) => onChange(e.target.value)}
      />
      {saved && dirty && (
        <span className="hint">
          {value
            ? "Replaces the saved value."
            : "Saving clears the saved value."}{" "}
          <button type="button" className="link" onClick={onRevert}>
            Keep saved
          </button>
        </span>
      )}
    </label>
  );
}
