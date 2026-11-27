import { CancelError } from "@wailsio/runtime";
import {
  ConnectionService,
  FileKind,
  FileService,
  SchemaService,
} from "@bindings/internal/app";
import type { ConnectionDraft, TableInfo } from "@bindings/internal/app";
import { CNPGUser, DBType, SSHAuthMethod } from "@bindings/internal/conn";
import type {
  ConnectionConfig,
  K8sContexts,
  K8sForward,
  K8sTargets,
  ServerInfo,
  SSHTunnel,
  TLSFiles,
} from "@bindings/internal/conn";
import type { Column, Index, RowPage, Table } from "@bindings/internal/dialect";
import type { Connection } from "@bindings/internal/store";

export { CNPGUser, DBType, FileKind, SSHAuthMethod };
export type {
  Connection,
  ConnectionConfig,
  Column,
  ConnectionDraft,
  Index,
  K8sContexts,
  K8sForward,
  K8sTargets,
  RowPage,
  ServerInfo,
  SSHTunnel,
  Table,
  TableInfo,
  TLSFiles,
};

export const arr = <T>(x: T[] | null | undefined): T[] => x ?? [];

export const api = {
  // Resolves to "" when the user cancels. Never build file paths in the frontend.
  pickFile: (kind: FileKind) => FileService.PickFile(kind),

  // Resolves to the detected server (e.g. MariaDB 10.11.6). Return the
  // CancellablePromise as-is so callers can .cancel() a slow server.
  testConnection: (d: ConnectionDraft) => ConnectionService.TestConnection(d),
  listDatabases: async (d: ConnectionDraft) =>
    arr(await ConnectionService.ListDatabases(d)),
  // Tries the usual socket locations on the draft's SSH server.
  detectSocket: (d: ConnectionDraft) => ConnectionService.DetectSocket(d),

  // Kubernetes discovery; kubeconfigPath "" = $KUBECONFIG or ~/.kube/config.
  k8sContexts: (kubeconfigPath: string) =>
    ConnectionService.K8sContexts(kubeconfigPath),
  k8sNamespaces: async (kubeconfigPath: string, context: string) =>
    arr(await ConnectionService.K8sNamespaces(kubeconfigPath, context)),
  k8sTargets: (kubeconfigPath: string, context: string, namespace: string) =>
    ConnectionService.K8sTargets(kubeconfigPath, context, namespace),

  // Saved connections never come back with passwords; refer to them by id.
  listSaved: async () => arr(await ConnectionService.ListSaved()),
  // updateSecrets names the secrets the user typed; the others keep their stored value.
  saveConnection: (c: Connection, updateSecrets: string[]) =>
    ConnectionService.SaveConnection(c, updateSecrets),
  deleteConnection: (id: string) => ConnectionService.DeleteConnection(id),
  testSaved: (id: string) => ConnectionService.TestSavedConnection(id),
  listSavedDatabases: async (id: string) =>
    arr(await ConnectionService.ListSavedDatabases(id)),

  // Browsing a saved connection; database "" = the connection's default.
  // These return the CancellablePromise so a newer selection can cancel them.
  listTables: (id: string, database: string) =>
    SchemaService.ListTables(id, database).then(arr),
  describeTable: (id: string, database: string, t: Table) =>
    SchemaService.DescribeTable(id, database, t),
  // limit is capped at 500 by the backend.
  previewRows: (
    id: string,
    database: string,
    t: Table,
    offset: number,
    limit: number,
  ) => SchemaService.PreviewRows(id, database, t, offset, limit),
  // May scan the whole table.
  countRows: (id: string, database: string, t: Table) =>
    SchemaService.CountRows(id, database, t),
};

export const isCancel = (e: unknown) => e instanceof CancelError;

export const errMsg = (e: unknown) =>
  e instanceof Error ? e.message : String(e);
