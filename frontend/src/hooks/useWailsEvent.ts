import { useEffect, useRef } from "react";
import { Events } from "@wailsio/runtime";

type CustomEvents = Events.CustomEvents;

export function useWailsEvent<E extends keyof CustomEvents>(
  name: E,
  handler: (data: CustomEvents[E]) => void,
) {
  const ref = useRef(handler);
  ref.current = handler;

  // The cast keeps this compiling while no custom events are registered.
  useEffect(() => Events.On(name, (ev) => ref.current(ev.data as CustomEvents[E])), [name]);
}
