import { CancelError } from "@wailsio/runtime";
import { FileKind, FileService, HelloService } from "@bindings/internal/app";

export { FileKind };

export const arr = <T,>(x: T[] | null | undefined): T[] => x ?? [];

export const api = {
  hello: (name: string) => HelloService.Hello(name),
  // Return the CancellablePromise as-is; wrapping in async would drop .cancel().
  count: (total: number, intervalMs = 100) => HelloService.Count(total, intervalMs),
  // Resolves to "" when the user cancels. Never build file paths in the frontend.
  pickFile: (kind: FileKind) => FileService.PickFile(kind),
};

export const isCancel = (e: unknown) => e instanceof CancelError;

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));
