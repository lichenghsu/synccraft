import { CancelError } from "@wailsio/runtime";
import { HelloService } from "@bindings/internal/app";

export const arr = <T,>(x: T[] | null | undefined): T[] => x ?? [];

export const api = {
  hello: (name: string) => HelloService.Hello(name),
  // Return the CancellablePromise as-is; wrapping in async would drop .cancel().
  count: (total: number, intervalMs = 100) => HelloService.Count(total, intervalMs),
};

export const isCancel = (e: unknown) => e instanceof CancelError;

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));
