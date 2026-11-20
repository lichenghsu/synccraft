import { useEffect, useRef } from "react";
import { Events } from "@wailsio/runtime";

type CustomEvents = Events.CustomEvents;

export function useWailsEvent<E extends keyof CustomEvents>(
  name: E,
  handler: (data: CustomEvents[E]) => void,
) {
  const ref = useRef(handler);
  ref.current = handler;

  useEffect(() => Events.On(name, (ev) => ref.current(ev.data)), [name]);
}
