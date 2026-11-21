import { CancelError } from "@wailsio/runtime";
import { ConnectionService, FileKind, FileService } from "@bindings/internal/app";
import type { ConnectionDraft } from "@bindings/internal/app";
import { DBType, SSHAuthMethod } from "@bindings/internal/conn";
import type { ConnectionConfig, ServerInfo, SSHTunnel, TLSFiles } from "@bindings/internal/conn";
import type { Connection } from "@bindings/internal/store";

export { DBType, FileKind, SSHAuthMethod };
export type { Connection, ConnectionConfig, ConnectionDraft, ServerInfo, SSHTunnel, TLSFiles };

export const arr = <T,>(x: T[] | null | undefined): T[] => x ?? [];

export const api = {
  // Resolves to "" when the user cancels. Never build file paths in the frontend.
  pickFile: (kind: FileKind) => FileService.PickFile(kind),

  // Resolves to the detected server (e.g. MariaDB 10.11.6). Return the
  // CancellablePromise as-is so callers can .cancel() a slow server.
  testConnection: (d: ConnectionDraft) => ConnectionService.TestConnection(d),
  listDatabases: async (d: ConnectionDraft) => arr(await ConnectionService.ListDatabases(d)),
  // Tries the usual socket locations on the draft's SSH server.
  detectSocket: (d: ConnectionDraft) => ConnectionService.DetectSocket(d),

  // Saved connections never come back with passwords; refer to them by id.
  listSaved: async () => arr(await ConnectionService.ListSaved()),
  // updateSecrets names the secrets the user typed; the others keep their stored value.
  saveConnection: (c: Connection, updateSecrets: string[]) => ConnectionService.SaveConnection(c, updateSecrets),
  deleteConnection: (id: string) => ConnectionService.DeleteConnection(id),
  testSaved: (id: string) => ConnectionService.TestSavedConnection(id),
  listSavedDatabases: async (id: string) => arr(await ConnectionService.ListSavedDatabases(id)),
};

export const isCancel = (e: unknown) => e instanceof CancelError;

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));
