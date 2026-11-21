import {
  DBType,
  SSHAuthMethod,
  type Connection,
  type ConnectionConfig,
  type ConnectionDraft,
  type SSHTunnel,
} from "../../api/wails";

export const DB_TYPES = [
  DBType.MySQL,
  DBType.PostgreSQL,
  DBType.SQLServer,
  DBType.SQLite,
] as const;

export const DB_LABEL: Record<DBType, string> = {
  [DBType.$zero]: "",
  [DBType.MySQL]: "MySQL / MariaDB",
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

export const SSL_MODES = [
  "disable",
  "allow",
  "prefer",
  "require",
  "verify-ca",
  "verify-full",
] as const;

export const SSH_AUTHS = [
  { value: SSHAuthMethod.SSHAuthPassword, label: "Password" },
  { value: SSHAuthMethod.SSHAuthPrivateKey, label: "Private key" },
  { value: SSHAuthMethod.SSHAuthAgent, label: "SSH agent" },
] as const;

// Mirrors store.Secret* in Go: keychain entry names and updateSecrets values.
export const SECRET = {
  password: "password",
  sshPassword: "ssh-password",
  sshPassphrase: "ssh-passphrase",
} as const;
export type SecretName = (typeof SECRET)[keyof typeof SECRET];
const ALL_SECRETS: SecretName[] = Object.values(SECRET);

export type FormState = {
  id: string; // "" = new connection
  name: string;
  config: ConnectionConfig;
  // Secrets the user typed into. Saved connections open with empty secret
  // fields; anything not listed here keeps its stored value.
  dirtySecrets: SecretName[];
  // The SSH box; config.ssh is kept while unticked so nothing typed is lost.
  useSSH: boolean;
  // Reach MySQL / PostgreSQL through a Unix socket on the SSH server.
  viaSocket: boolean;
};

export const SOCKET_PLACEHOLDER: Partial<Record<DBType, string>> = {
  [DBType.MySQL]: "/run/mysqld/mysqld.sock",
  [DBType.PostgreSQL]: "/var/run/postgresql",
};

export const supportsSocket = (t: DBType) =>
  t === DBType.MySQL || t === DBType.PostgreSQL;

export const newForm = (type: DBType = DBType.MySQL): FormState => ({
  id: "",
  name: "",
  config: {
    type,
    host: "127.0.0.1",
    port: 0,
    user: "",
    password: "",
    database: "",
  },
  dirtySecrets: [],
  useSSH: false,
  viaSocket: false,
});

export const formFromSaved = (c: Connection): FormState => ({
  id: c.id,
  name: c.name,
  config: c.config,
  dirtySecrets: [],
  useSSH: !!c.config.ssh,
  viaSocket: !!c.config.socketPath,
});

export const newSSH = (): SSHTunnel => ({
  host: "",
  port: 0,
  user: "",
  auth: SSHAuthMethod.SSHAuthPassword,
});

// Drops fields that don't apply to the chosen engine or SSH auth method, so a
// stale value can't fail validation or be sent along.
export function cleanConfig(cfg: ConnectionConfig): ConnectionConfig {
  if (cfg.type === DBType.SQLite) {
    return {
      ...cfg,
      host: "",
      port: 0,
      user: "",
      password: "",
      database: "",
      tls: null,
      ssh: null,
      socketPath: "",
      sslMode: "",
      trustServerCertificate: false,
    };
  }
  const out: ConnectionConfig = { ...cfg, filePath: "" };
  if (cfg.type !== DBType.PostgreSQL) out.sslMode = "";
  if (cfg.type !== DBType.SQLServer) out.trustServerCertificate = false;
  if (cfg.type === DBType.SQLServer && out.tls)
    out.tls = { caCertPath: out.tls.caCertPath };
  if (
    out.tls &&
    !out.tls.caCertPath &&
    !out.tls.clientCertPath &&
    !out.tls.clientKeyPath
  )
    out.tls = null;
  if (out.ssh) out.ssh = cleanSSH(out.ssh);
  return out;
}

function cleanSSH(s: SSHTunnel): SSHTunnel {
  switch (s.auth) {
    case SSHAuthMethod.SSHAuthPassword:
      return { ...s, privateKeyPath: "", passphrase: "" };
    case SSHAuthMethod.SSHAuthPrivateKey:
      return { ...s, password: "" };
    default:
      return { ...s, password: "", privateKeyPath: "", passphrase: "" };
  }
}

// Secrets that no longer apply to the chosen SSH auth method; marking them
// dirty (with empty values) deletes them from the keychain on save.
export function unusedSSHSecrets(auth: SSHAuthMethod): SecretName[] {
  switch (auth) {
    case SSHAuthMethod.SSHAuthPassword:
      return [SECRET.sshPassphrase];
    case SSHAuthMethod.SSHAuthPrivateKey:
      return [SECRET.sshPassword];
    default:
      return [SECRET.sshPassword, SECRET.sshPassphrase];
  }
}

function applyTransport(f: FormState): ConnectionConfig {
  const cfg = cleanConfig(f.useSSH ? f.config : { ...f.config, ssh: null });
  if (!cfg.ssh || !f.viaSocket || !supportsSocket(cfg.type))
    return { ...cfg, socketPath: "" };
  // Socket auth maps the OS user to the database user of the same name.
  return { ...cfg, host: "", tls: null, user: cfg.user.trim() || cfg.ssh.user };
}

export const toDraft = (f: FormState): ConnectionDraft => ({
  id: f.id,
  config: applyTransport(f),
  // A new connection has nothing stored, so every typed secret counts.
  updateSecrets: f.id === "" ? ALL_SECRETS : f.dirtySecrets,
});
