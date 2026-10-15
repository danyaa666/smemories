import { useSyncExternalStore } from "react";

/** True while the media query matches; false where matchMedia does not exist. */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (notify) => {
      const m = window.matchMedia?.(query);
      m?.addEventListener("change", notify);
      return () => m?.removeEventListener("change", notify);
    },
    () => window.matchMedia?.(query).matches ?? false,
  );
}
