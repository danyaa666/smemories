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

/**
 * Shrinks the font of `el` in 0.5 px steps until its content fits, but not below `minScale` of the starting size
 * (`startSize` is the template's own inline size). Returns what happened. Needs real layout, so tests give it metrics.
 */
export function fitElement(el: HTMLElement, startSize: string, minScale: number): FitState {
  el.style.fontSize = startSize;
  const base = parseFloat(getComputedStyle(el).fontSize);
  const over = () => el.scrollHeight > el.clientHeight + 1 || el.scrollWidth > el.clientWidth + 1;
  let state: FitState = "ok";
  for (let s = base; over() && s - FIT_STEP_PX >= base * minScale;) {
    s -= FIT_STEP_PX;
    el.style.fontSize = `${s}px`;
    state = "shrunk";
  }
  return over() ? "overflow" : state;
}

type Report = (key: string, state: FitState | undefined) => void;
const FitContext = createContext<Report | null>(null);

/** Collects the fit state of every FitText below it, keyed by the text's own key. */
export function useFitReport() {
  const [states, setStates] = useState<Record<string, FitState>>({});
  const report = useCallback<Report>((key, state) => {
    setStates((prev) => {
      if (prev[key] === state) return prev;
      const next = { ...prev };
      if (state === undefined) delete next[key];
      else next[key] = state;
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
 * summary: "note:<id>" for a friend's note, anything else for the owner's own pages. Re-fits when web fonts finish loading,
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
      const state = fitElement(el, start, minScale);
      el.dataset.fit = state;
      report?.(fitKey, state);
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
