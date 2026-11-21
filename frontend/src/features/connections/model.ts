import { DBType, type Connection, type ConnectionConfig, type ConnectionDraft } from "../../api/wails";

export const DB_TYPES = [DBType.MySQL, DBType.PostgreSQL, DBType.SQLServer, DBType.SQLite] as const;

export const DB_LABEL: Record<DBType, string> = {
  [DBType.$zero]: "",
  [DBType.MySQL]: "MySQL",
  [DBType.PostgreSQL]: "PostgreSQL",
  [DBType.SQLServer]: "SQL Server",
  [DBType.SQLite]: "SQLite",
};

export const DEFAULT_PORT: Record<DBType, number> = {
  [DBType.$zero]: 0,
  [DBType.MySQL]: 3306,
  [DBType.PostgreSQL]: 5432,
  [DBType.SQLServer]: 1433,
  [DBType.SQLite]: 0,
};

export const SSL_MODES = ["disable", "allow", "prefer", "require", "verify-ca", "verify-full"] as const;

export type FormState = {
  id: string; // "" = new connection
  name: string;
  config: ConnectionConfig;
  // True once the user types in the password field. Saved connections start
  // with an empty password field; false means "keep the stored password".
  secretsDirty: boolean;
};

export const newForm = (type: DBType = DBType.MySQL): FormState => ({
  id: "",
  name: "",
  config: { type, host: "127.0.0.1", port: 0, user: "", password: "", database: "" },
  secretsDirty: false,
});

export const formFromSaved = (c: Connection): FormState => ({
  id: c.id,
  name: c.name,
  config: c.config,
  secretsDirty: false,
});

// Drops fields that don't apply to the chosen engine, so a stale value from a
// previous type can't fail validation. SSH is not in the form until tunnels work.
export function cleanConfig(cfg: ConnectionConfig): ConnectionConfig {
  const out: ConnectionConfig = { ...cfg, ssh: null };
  if (cfg.type === DBType.SQLite) {
    return { ...out, host: "", port: 0, user: "", password: "", database: "", tls: null, sslMode: "", trustServerCertificate: false };
  }
  out.filePath = "";
  if (cfg.type !== DBType.PostgreSQL) out.sslMode = "";
  if (cfg.type !== DBType.SQLServer) out.trustServerCertificate = false;
  if (cfg.type === DBType.MySQL) out.tls = null;
  if (cfg.type === DBType.SQLServer && out.tls) out.tls = { caCertPath: out.tls.caCertPath };
  if (out.tls && !out.tls.caCertPath && !out.tls.clientCertPath && !out.tls.clientKeyPath) out.tls = null;
  return out;
}

export const toDraft = (f: FormState): ConnectionDraft => ({
  id: f.id,
  config: cleanConfig(f.config),
  // A new connection has nothing stored, so whatever is typed is the secret.
  updateSecrets: f.id === "" || f.secretsDirty,
});
