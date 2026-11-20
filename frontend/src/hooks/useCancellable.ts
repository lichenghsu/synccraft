import { useCallback, useEffect, useRef, useState } from "react";
import type { CancellablePromise } from "@wailsio/runtime";

export function useCancellable<A extends unknown[], T>(fn: (...args: A) => CancellablePromise<T>) {
  const ref = useRef<CancellablePromise<T> | null>(null);
  const [running, setRunning] = useState(false);

  const run = useCallback(
    async (...args: A): Promise<T> => {
      ref.current?.cancel();
      const p = fn(...args);
      ref.current = p;
      setRunning(true);
      try {
        return await p;
      } finally {
        if (ref.current === p) {
          ref.current = null;
          setRunning(false);
        }
      }
    },
    [fn],
  );

  const cancel = useCallback(() => {
    ref.current?.cancel();
  }, []);

  useEffect(
    () => () => {
      ref.current?.cancel();
    },
    [],
  );

  return { run, cancel, running };
}
