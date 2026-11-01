import type { TFunction } from "i18next";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { useTranslation } from "react-i18next";
import type { BookData } from "../../api/book";
import { BookPages, type OnePage } from "../BookPages";
import { pageBox, type Rect } from "../layout/geometry";
import { kindOf, type TemplateLayout } from "../layout/runtime";
import { DESIGN_W, designZoom, pagePx, SIZES, type PageSize } from "../sizes";
import { pageName, targetName } from "./names";
import { targetsOf, unrotate, type Target } from "./targets";
import "../book.css";

const ZOOMS = [50, 75, 100, 125, 150, 200];
const PAD = 28; // screen px around the page: room for handles of an element flush to the edge
const MM = 96 / 25.4;
const CORNERS = ["nw", "n", "ne", "e", "se", "s", "sw", "w"] as const;
const noop = () => {};

/** Where the template draws each unmoved element of the page, measured on the stage (design px, unrotated); the template's natural place is the fallback. */
function useMeasured(
  root: React.RefObject<HTMLElement | null>,
  kind: string,
  spec: TemplateLayout | undefined,
  data: BookData,
  page: OnePage,
  scale: number,
) {
  const [measured, setMeasured] = useState<Record<string, Rect>>({});
  useLayoutEffect(() => {
    let live = true;
    const measure = () => {
      const el = root.current;
      const sheet = el?.querySelector(".bk-sheet");
      const sr = sheet?.getBoundingClientRect();
      const next: Record<string, Rect> = {};
      if (el && sr && sr.width > 0) {
        const k = sr.width / DESIGN_W; // screen px per design px
        for (const [id, nat] of Object.entries(spec?.natural ?? {})) {
          if (kindOf(id) !== kind) continue;
          const wrap = el.querySelector<HTMLElement>(`[data-layout-id="${id}"]`);
          if (!wrap) continue;
          const parts = wrap.style.display === "contents" ? [...wrap.children] : [wrap];
          const rs = parts.map((p) => p.getBoundingClientRect()).filter((r) => r.width > 0);
          if (rs.length === 0) continue;
          const l = Math.min(...rs.map((r) => r.left));
          const t = Math.min(...rs.map((r) => r.top));
          const w = Math.max(...rs.map((r) => r.right)) - l;
          const h = Math.max(...rs.map((r) => r.bottom)) - t;
          const own = unrotate(w / k, h / k, nat.r ?? 0);
          const cx = (l - sr.left + w / 2) / k;
          const cy = (t - sr.top + h / 2) / k;
          next[id] = { left: cx - own.w / 2, top: cy - own.h / 2, width: own.w, height: own.h };
        }
      }
      // keep the old object when nothing moved by a tenth of a design px: no render loop
      setMeasured((prev) => {
        const same =
          Object.keys(next).length === Object.keys(prev).length &&
          Object.entries(next).every(([id, r]) => {
            const q = prev[id];
            return (
              q &&
              Math.abs(q.left - r.left) < 0.1 &&
              Math.abs(q.top - r.top) < 0.1 &&
              Math.abs(q.width - r.width) < 0.1 &&
              Math.abs(q.height - r.height) < 0.1
            );
          });
        return same ? prev : next;
      });
    };
    measure();
    void document.fonts?.ready.then(() => live && measure()); // text that fits after the fonts arrive can change a box
    return () => {
      live = false;
    };
  }, [root, kind, spec, data, page, scale]);
  return measured;
}

/**
 * The stage (T-159): the selected page drawn by the real book renderer with the layout runtime, a zoom (fit, or 50 to 200 percent), the safe margin
 * guide and, over the page and OUTSIDE its clip, the selection chrome: an outline, eight resize handles and a rotate handle on the selection (shapes only;
 * the gestures are T-160). Each selectable thing is a focusable button in draw order, so Tab cycles through them and a click picks the topmost.
 */
export function Stage({
  data,
  size,
  bookT,
  page,
  kind,
  spec,
  selection,
  onSelect,
  onTargets,
  readOnly,
  safeMm,
}: {
  /** The book with the editor's layout document in `layout`. */
  data: BookData;
  size: PageSize;
  bookT: TFunction;
  page: OnePage;
  kind: string;
  spec: TemplateLayout | undefined;
  selection: readonly string[];
  onSelect: (ids: readonly string[]) => void;
  /** Told where everything on the page is drawn (the panel shows the same numbers). */
  onTargets: (targets: Target[]) => void;
  readOnly: boolean;
  safeMm: number;
}) {
  const { t } = useTranslation();
  const scroll = useRef<HTMLDivElement>(null);
  const box = useRef<HTMLDivElement>(null);
  const [zoom, setZoom] = useState<number | "fit">("fit");
  const [margin, setMargin] = useState(true);
  const [avail, setAvail] = useState({ w: 0, h: 0 });

  useLayoutEffect(() => {
    const el = scroll.current;
    if (!el) return;
    const update = () => setAvail({ w: el.clientWidth - 2 * PAD, h: el.clientHeight - 2 * PAD });
    update();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const px = pagePx(size);
  const fit =
    avail.w > 0 && avail.h > 0
      ? Math.round(Math.min(2, Math.max(0.1, Math.min(avail.w / px.w, avail.h / px.h))) * 1000) /
        1000
      : 0.5;
  const s = zoom === "fit" ? fit : zoom / 100;
  const pct = Math.round(s * 100);
  const k = designZoom(size) * s; // screen px per design px
  const pb = useMemo(() => pageBox(size), [size]);

  const step = useCallback(
    (dir: 1 | -1) =>
      setZoom((z) => {
        const now = z === "fit" ? pct : z;
        const next =
          dir > 0 ? ZOOMS.find((v) => v > now) : [...ZOOMS].reverse().find((v) => v < now);
        return next ?? (z === "fit" ? pct : z);
      }),
    [pct],
  );
  // Ctrl or Cmd + wheel zooms the stage instead of the browser (needs a listener that may call preventDefault)
  useEffect(() => {
    const el = scroll.current;
    if (!el) return;
    const wheel = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return;
      e.preventDefault();
      step(e.deltaY < 0 ? 1 : -1);
    };
    el.addEventListener("wheel", wheel, { passive: false });
    return () => el.removeEventListener("wheel", wheel);
  }, [step]);
  // a click on the stage outside any selectable box clears the selection
  useEffect(() => {
    const el = scroll.current;
    if (!el) return;
    const click = (e: MouseEvent) => {
      if (!(e.target as Element).closest(".ed-box, .ed-bar")) onSelect([]);
    };
    el.addEventListener("click", click);
    return () => el.removeEventListener("click", click);
  }, [onSelect]);

  const measured = useMeasured(box, kind, spec, data, page, s);
  const targets = useMemo(
    () => targetsOf(data.layout ?? { v: 1, pages: {} }, kind, spec, pb, measured),
    [data.layout, kind, spec, pb, measured],
  );
  useEffect(() => onTargets(targets), [onTargets, targets]);
  const vars = {
    "--pw": `${SIZES[size].w}mm`,
    "--ph": `${SIZES[size].h}mm`,
    "--s": s,
  } as CSSProperties;

  return (
    <section className="ed-stage" aria-label={t("editor.stage", { page: pageName(t, page) })}>
      <div className="ed-bar">
        <button type="button" onClick={() => step(-1)} disabled={pct <= ZOOMS[0]!}>
          {t("editor.zoomOut")}
        </button>
        <output aria-label={t("editor.zoomLevel")}>{pct} %</output>
        <button type="button" onClick={() => step(1)} disabled={pct >= ZOOMS[ZOOMS.length - 1]!}>
          {t("editor.zoomIn")}
        </button>
        <button type="button" aria-pressed={zoom === "fit"} onClick={() => setZoom("fit")}>
          {t("editor.zoomFit")}
        </button>
        <button type="button" aria-pressed={margin} onClick={() => setMargin((m) => !m)}>
          {t("editor.safeMargin")}
        </button>
      </div>
      <div ref={scroll} className="ed-scroll">
        <div ref={box} className="ed-pagebox" style={vars} data-zoom={pct}>
          <BookPages data={data} size={size} t={bookT} eager={false} track={noop} only={page} />
          <div className="ed-overlay">
            {margin && (
              <div
                className="ed-safe"
                data-testid="safe-margin"
                style={{ inset: safeMm * MM * s }}
              />
            )}
            {targets.map((x) => (
              <Box
                key={x.id}
                x={x}
                k={k}
                t={t}
                selected={selection.includes(x.id)}
                onSelect={onSelect}
                readOnly={readOnly}
              />
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

function Box({
  x,
  k,
  t,
  selected,
  onSelect,
  readOnly,
}: {
  x: Target;
  k: number;
  t: TFunction;
  selected: boolean;
  onSelect: (ids: readonly string[]) => void;
  readOnly: boolean;
}) {
  const style: CSSProperties = {
    left: x.rect.left * k,
    top: x.rect.top * k,
    width: x.rect.width * k,
    height: x.rect.height * k,
    transform: x.r ? `rotate(${x.r}deg)` : undefined,
  };
  return (
    <div
      className={`ed-box${selected ? " is-selected" : ""}${x.hidden ? " is-hidden" : ""}`}
      style={style}
      data-target={x.id}
    >
      <button
        type="button"
        className="ed-hit"
        aria-pressed={selected}
        aria-label={targetName(t, x)}
        onFocus={() => !selected && onSelect([x.id])}
        onClick={() => onSelect([x.id])}
      />
      {selected && !x.hidden && (
        <>
          {CORNERS.map((c) => (
            <button
              key={c}
              type="button"
              tabIndex={-1}
              className={`ed-handle ed-h-${c}`}
              aria-label={t(`editor.handle.${c}`)}
              aria-disabled={readOnly}
            />
          ))}
          <button
            type="button"
            tabIndex={-1}
            className="ed-handle ed-rotate"
            aria-label={t("editor.handle.rotate")}
            aria-disabled={readOnly}
          />
        </>
      )}
    </div>
  );
}
