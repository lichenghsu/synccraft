import { useCallback, useState } from "react";
import type { AsyncState } from "../types";
import { errMsg } from "../api/wails";

export function useAsync<A extends unknown[], T>(fn: (...args: A) => PromiseLike<T>) {
  const [state, setState] = useState<AsyncState<T>>({ status: "idle" });

  const run = useCallback(
    async (...args: A): Promise<T | undefined> => {
      setState({ status: "loading" });
      try {
        const data = await fn(...args);
        setState({ status: "success", data });
        return data;
      } catch (e) {
        setState({ status: "error", error: errMsg(e) });
        return undefined;
      }
    },
    [fn],
  );

  return { state, run };
}
