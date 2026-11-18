import { useEffect, useRef } from "react";

/**
 * A button the keyboard is on becomes disabled by its own action (Undo at the first step, "Send to back" at the bottom): the browser drops the focus
 * and a screen reader loses its place (T-182). Moves it to the nearest enabled button of the same group, or the page being edited. A pointer press
 * forgets the remembered button, so a click on the background is never answered by a focus jump.
 */
export function useKeepFocus() {
  const last = useRef<HTMLButtonElement | null>(null);
  useEffect(() => {
    const on = (e: FocusEvent) => {
      last.current = e.target instanceof HTMLButtonElement ? e.target : null;
    };
    const off = () => (last.current = null);
    document.addEventListener("focusin", on);
    document.addEventListener("pointerdown", off, true);
    return () => {
      document.removeEventListener("focusin", on);
      document.removeEventListener("pointerdown", off, true);
    };
  }, []);
  useEffect(() => {
    const el = last.current;
    const now = document.activeElement;
    if (!el || !el.isConnected || !el.disabled || (now !== el && now !== document.body)) return;
    const group = el.closest<HTMLElement>("[role=toolbar], [role=group], .ed-row, .ed-panel");
    const all = [...(group?.querySelectorAll<HTMLButtonElement>("button") ?? [])];
    const at = all.indexOf(el);
    const near = all
      .map((b, i) => ({ b, d: Math.abs(i - at) + (i < at ? 0 : 0.5) })) // a tie goes to the one before
      .filter(({ b }) => !b.disabled)
      .sort((x, y) => x.d - y.d)[0]?.b;
    (near ?? document.querySelector<HTMLElement>(".ed-pagebox"))?.focus();
  });
}
