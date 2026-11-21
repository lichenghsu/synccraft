import { CancelError } from "@wailsio/runtime";
import { ConnectionService, FileKind, FileService, HelloService } from "@bindings/internal/app";
import type { ConnectionConfig } from "@bindings/internal/conn";
import type { Connection } from "@bindings/internal/store";

export { FileKind };
export type { Connection, ConnectionConfig };

export const arr = <T,>(x: T[] | null | undefined): T[] => x ?? [];

export const api = {
  hello: (name: string) => HelloService.Hello(name),
  // Return the CancellablePromise as-is; wrapping in async would drop .cancel().
  count: (total: number, intervalMs = 100) => HelloService.Count(total, intervalMs),
  // Resolves to "" when the user cancels. Never build file paths in the frontend.
  pickFile: (kind: FileKind) => FileService.PickFile(kind),

  // Unsaved config typed into the form (includes the password).
  testConnection: (cfg: ConnectionConfig) => ConnectionService.TestConnection(cfg),
  listDatabases: async (cfg: ConnectionConfig) => arr(await ConnectionService.ListDatabases(cfg)),

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
