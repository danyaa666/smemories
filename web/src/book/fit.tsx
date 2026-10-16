import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";

/** How a text box ended up: as designed, set in a smaller font, or still too long at the smallest font (clipped). */
export type FitState = "ok" | "shrunk" | "overflow";

export const FIT_STEP_PX = 0.5;

/** What fitting a box ended with: the state, the template's own font size and the size used (both in design px). */
export type FitInfo = { state: FitState; base: number; px: number };

/**
 * Shrinks the font of `el` in 0.5 px steps until its content fits, but not below `minScale` of the starting size
 * (`startSize` is the template's own inline size). Returns what happened. Needs real layout, so tests give it metrics.
 */
export function fitElement(el: HTMLElement, startSize: string, minScale: number): FitInfo {
  el.style.fontSize = startSize;
  const base = parseFloat(getComputedStyle(el).fontSize);
  // The last line's blank leading (0.2 em; line heights here are 1.4 and more) may be cut: emoji and other fallback fonts make a line a
  // few px taller than its line height, which would otherwise count as text that does not fit.
  let px = base;
  const over = () =>
    el.scrollHeight > el.clientHeight + 1 + 0.2 * px || el.scrollWidth > el.clientWidth + 1;
  let state: FitState = "ok";
  for (; over() && px - FIT_STEP_PX >= base * minScale;) {
    px -= FIT_STEP_PX;
    el.style.fontSize = `${px}px`;
    state = "shrunk";
  }
  return { state: over() ? "overflow" : state, base, px };
}

type Report = (key: string, info: FitInfo | undefined) => void;
const FitContext = createContext<Report | null>(null);

/** Collects the fit result of every FitText below it, keyed by the text's own key. */
export function useFitReport() {
  const [states, setStates] = useState<Record<string, FitInfo>>({});
  const report = useCallback<Report>((key, info) => {
    setStates((prev) => {
      const old = prev[key];
      if (old === info || (old && info && old.state === info.state && old.px === info.px))
        return prev;
      const next = { ...prev };
      if (info === undefined) delete next[key];
      else next[key] = info;
      return next;
    });
  }, []);
  const Provider = useMemo(
    () =>
      function FitProvider({ children }: { children: ReactNode }) {
        return <FitContext value={report}>{children}</FitContext>;
      },
    [report],
  );
  return { states, Provider };
}

/**
 * A box of fixed size whose text shrinks to fit (JavaScript, not CSS) and reports the outcome. `fitKey` names the text in the
 * summary: "note:<id>" or "note:<id>:<field>" for a friend's note (a note may have several boxes), anything else for the owner's own pages. Re-fits when web fonts finish loading,
 * because the fallback font has other metrics. Children are React nodes: user text is always escaped.
 */
export function FitText({
  fitKey,
  children,
  minScale = 0.5,
  className,
  style,
}: {
  fitKey: string;
  children: ReactNode;
  minScale?: number;
  className?: string;
  style?: CSSProperties;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const report = useContext(FitContext);
  const size = style?.fontSize;
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    let live = true;
    const start = typeof size === "number" ? `${size}px` : (size ?? "");
    const fit = () => {
      const info = fitElement(el, start, minScale);
      el.dataset.fit = info.state;
      report?.(fitKey, info);
    };
    fit();
    const fonts = typeof document === "undefined" ? undefined : document.fonts;
    const again = () => live && fit();
    void fonts?.ready.then(again);
    fonts?.addEventListener?.("loadingdone", again);
    return () => {
      live = false;
      fonts?.removeEventListener?.("loadingdone", again);
    };
  }, [children, minScale, size, fitKey, report]);
  // Only when the text leaves the page (not on every re-fit, which would make the summary flicker and loop).
  useLayoutEffect(() => () => report?.(fitKey, undefined), [fitKey, report]);
  return (
    <div ref={ref} className={`bk-fit ${className ?? ""}`} style={style}>
      {children}
    </div>
  );
}
