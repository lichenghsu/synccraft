import { CancelError } from "@wailsio/runtime";
import { ConnectionService, FileKind, FileService } from "@bindings/internal/app";
import type { ConnectionDraft } from "@bindings/internal/app";
import { DBType } from "@bindings/internal/conn";
import type { ConnectionConfig, TLSFiles } from "@bindings/internal/conn";
import type { Connection } from "@bindings/internal/store";

export { DBType, FileKind };
export type { Connection, ConnectionConfig, ConnectionDraft, TLSFiles };

export const arr = <T,>(x: T[] | null | undefined): T[] => x ?? [];

export const api = {
  // Resolves to "" when the user cancels. Never build file paths in the frontend.
  pickFile: (kind: FileKind) => FileService.PickFile(kind),

  // Return the CancellablePromise as-is so callers can .cancel() a slow server.
  testConnection: (d: ConnectionDraft) => ConnectionService.TestConnection(d),
  listDatabases: async (d: ConnectionDraft) => arr(await ConnectionService.ListDatabases(d)),

  // Saved connections never come back with passwords; refer to them by id.
  listSaved: async () => arr(await ConnectionService.ListSaved()),
  // updateSecrets=false keeps the stored passwords (the form left them untouched).
  saveConnection: (c: Connection, updateSecrets: boolean) => ConnectionService.SaveConnection(c, updateSecrets),
  deleteConnection: (id: string) => ConnectionService.DeleteConnection(id),
  testSaved: (id: string) => ConnectionService.TestSavedConnection(id),
  listSavedDatabases: async (id: string) => arr(await ConnectionService.ListSavedDatabases(id)),
};

export const isCancel = (e: unknown) => e instanceof CancelError;

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));
