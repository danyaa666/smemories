import { useCallback, useEffect, useRef } from "react";

/**
 * Debounced save of the book order: `schedule(ids)` waits `ms` after the last call, `flush()` sends at once (before a status
 * change, and when the page is left, so a drop is saved and not lost). Only the newest order is ever sent.
 */
export function useOrderSave(save: (ids: string[]) => Promise<void>, ms: number) {
  const timer = useRef<number>(undefined);
  const latest = useRef<string[] | null>(null);
  const saveRef = useRef(save);
  useEffect(() => {
    saveRef.current = save; // the timer always calls the newest closure
  });
  const flush = useCallback(async () => {
    window.clearTimeout(timer.current);
    const ids = latest.current;
    latest.current = null;
    if (ids) await saveRef.current(ids);
  }, []);
  const schedule = useCallback(
    (ids: string[]) => {
      latest.current = ids;
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => void flush(), ms);
    },
    [flush, ms],
  );
  useEffect(() => () => void flush(), [flush]);
  return { schedule, flush };
}
