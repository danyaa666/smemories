import type { TFunction } from "i18next";
import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ComponentType,
  type CSSProperties,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import type { BookData } from "../../api/book";
import { BookPages, type OnePage } from "../BookPages";
import { pageBox, type Rect } from "../layout/geometry";
import { kindOf, type TemplateLayout, type TextEdit } from "../layout/runtime";
import { DESIGN_W, designZoom, pagePx, SIZES, type PageSize } from "../sizes";
import { pageName, targetName } from "./names";
import { useGestures, type Api, type GestureStore } from "./gestures/useGestures";
import { PageTextField } from "./PageTextField";
import type { Handle } from "./gestures/math";
import { targetsOf, unrotate, type Target } from "./targets";
import "../book.css";

const ZOOMS = [50, 75, 100, 125, 150, 200];
const PAD = 28; // screen px around the page: room for handles of an element flush to the edge
const MM = 96 / 25.4;
// edge handles first, so where two 44-px hit areas overlap on a small box the corner wins
const HANDLES: readonly Handle[] = ["n", "e", "s", "w", "nw", "ne", "se", "sw"];
const HIT = 44; // screen px: the smallest hit target of a handle (WCAG 2.5.8 asks 24, touch guidelines 44)
const SMALL = HIT * 2; // a box with a side shorter than this (screen px) gets its handles pushed outside the corners
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
  const [measured, setMeasured] = useState<{ rects: Record<string, Rect>; order: string[] }>({
    rects: {},
    order: [],
  });
  useLayoutEffect(() => {
    let live = true;
    const measure = () => {
      const el = root.current;
      const sheet = el?.querySelector(".bk-sheet");
      const sr = sheet?.getBoundingClientRect();
      const next: Record<string, Rect> = {};
      // the layout ids in the page's DOM order = the order the elements are drawn in (a moved element is appended to the page when it first moves)
      const order = el
        ? [...el.querySelectorAll<HTMLElement>("[data-layout-id]")].map((n) => n.dataset.layoutId!)
        : [];
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
          order.join() === prev.order.join() &&
          Object.keys(next).length === Object.keys(prev.rects).length &&
          Object.entries(next).every(([id, r]) => {
            const q = prev.rects[id];
            return (
              q &&
              Math.abs(q.left - r.left) < 0.1 &&
              Math.abs(q.top - r.top) < 0.1 &&
              Math.abs(q.width - r.width) < 0.1 &&
              Math.abs(q.height - r.height) < 0.1
            );
          });
        return same ? prev : { rects: next, order };
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
  store,
  announce,
  Fit,
  editing,
  onEdit,
  onEditDone,
  cut,
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
  /** The T-158 store as the gestures use it (begin, preview, commit, cancel, apply, select). */
  store: GestureStore;
  announce: (s: string) => void;
  /** Collects what the fit rule did to each text (`useFitReport`), so the editor can warn about a text that is cut. */
  Fit: ComponentType<{ children: ReactNode }>;
  /** The text item that has its field open (T-161), and what to do when a text item asks for one or the field is done. */
  editing: { id: string; all: boolean } | null;
  onEdit: (id: string) => void;
  onEditDone: (id: string, to: { escape: true } | { blurred: EventTarget | null }) => void;
  /** The selected text is still too long at the smallest size the fit rule allows. */
  cut: boolean;
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
  const measured = useMeasured(box, kind, spec, data, page, s);
  const targets = useMemo(
    () =>
      targetsOf(data.layout ?? { v: 1, pages: {} }, kind, spec, pb, measured.rects, measured.order),
    [data.layout, kind, spec, pb, measured],
  );
  useEffect(() => onTargets(targets), [onTargets, targets]);
  const { guides, api } = useGestures({
    targets,
    pb,
    safe: (safeMm * DESIGN_W) / SIZES[size].w,
    selection,
    readOnly,
    pageEl: box,
    store,
    announce,
    t,
  });
  const textEdit = useMemo<TextEdit | undefined>(
    () =>
      editing && !readOnly
        ? {
            id: editing.id,
            render: (item, fit, style) => (
              <PageTextField
                item={item}
                kind={kind}
                style={style}
                fit={fit}
                store={store}
                selectAll={editing.all}
                readOnly={readOnly}
                onDone={onEditDone}
              />
            ),
          }
        : undefined,
    [editing, readOnly, kind, store, onEditDone],
  );
  // a click on the stage outside any selectable box clears the selection
  useEffect(() => {
    const el = scroll.current;
    if (!el) return;
    const click = (e: MouseEvent) => {
      if (
        !(e.target as Element).closest(".ed-box, .ed-chrome, .ed-bar, textarea") &&
        !api.justDragged()
      )
        onSelect([]);
    };
    el.addEventListener("click", click);
    return () => el.removeEventListener("click", click);
  }, [onSelect, api]);

  const selected = targets.find((x) => x.id === selection[0]);
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
      {cut && (
        <p role="status" className="ed-warn">
          {t("editor.textCut")}
        </p>
      )}
      <div ref={scroll} className="ed-scroll">
        <div ref={box} className="ed-pagebox" style={vars} data-zoom={pct} tabIndex={-1}>
          <Fit>
            <BookPages
              data={data}
              size={size}
              t={bookT}
              eager={false}
              track={noop}
              only={page}
              edit={textEdit}
            />
          </Fit>
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
                editing={!readOnly && editing?.id === x.id}
                canEdit={!readOnly && x.item?.t === "text"}
                onSelect={onSelect}
                onEdit={onEdit}
                api={api}
              />
            ))}
            {selected && !selected.hidden && (
              <Chrome x={selected} k={k} t={t} readOnly={readOnly} api={api} />
            )}
            {guides.x.map((v) => (
              <div key={`x${v}`} className="ed-guide ed-guide-v" style={{ left: v * k }} />
            ))}
            {guides.y.map((v) => (
              <div key={`y${v}`} className="ed-guide ed-guide-h" style={{ top: v * k }} />
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

const boxStyle = (x: Target, k: number): CSSProperties => ({
  left: x.rect.left * k,
  top: x.rect.top * k,
  width: x.rect.width * k,
  height: x.rect.height * k,
  transform: x.r ? `rotate(${x.r}deg)` : undefined,
});

/** The hit area of one target (a focusable button over its drawn box). Later boxes are drawn later, so a click on an overlap picks the topmost. */
const Box = memo(
  function Box({
    x,
    k,
    t,
    selected,
    editing,
    canEdit,
    onSelect,
    onEdit,
    api,
  }: {
    x: Target;
    k: number;
    t: TFunction;
    selected: boolean;
    editing: boolean;
    canEdit: boolean;
    onSelect: (ids: readonly string[]) => void;
    onEdit: (id: string) => void;
    api: Api;
  }) {
    // a pointer press on an item that was already selected (a tap on a selected text, or the second click of a double click) opens its field
    const wasSelected = useRef(false);
    return (
      <div
        className={`ed-box${selected ? " is-selected" : ""}${x.hidden ? " is-hidden" : ""}${editing ? " is-editing" : ""}`}
        style={boxStyle(x, k)}
        data-target={x.id}
      >
        <button
          type="button"
          className="ed-hit"
          aria-pressed={selected}
          aria-label={targetName(t, x)}
          onFocus={() => !selected && onSelect([x.id])}
          onPointerDownCapture={() => {
            wasSelected.current = selected;
          }}
          onClick={(e) => {
            if (api.justDragged()) return;
            const again = wasSelected.current && e.detail > 0;
            wasSelected.current = false;
            if (again && canEdit) onEdit(x.id);
            else onSelect([x.id]);
          }}
          onPointerDown={(e) => api.down(e, x, "move")}
          onDoubleClick={() => canEdit && onEdit(x.id)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && canEdit && !e.ctrlKey && !e.metaKey) {
              e.preventDefault();
              onEdit(x.id);
            } else api.key(e, x);
          }}
          onBlur={() => {
            wasSelected.current = false;
            api.blur();
          }}
        />
      </div>
    );
  },
  (a, b) =>
    a.k === b.k &&
    a.t === b.t &&
    a.selected === b.selected &&
    a.editing === b.editing &&
    a.canEdit === b.canEdit &&
    a.onSelect === b.onSelect &&
    a.onEdit === b.onEdit &&
    a.api === b.api &&
    a.x.id === b.x.id &&
    a.x.item === b.x.item &&
    a.x.r === b.x.r &&
    a.x.hidden === b.x.hidden &&
    a.x.rect.left === b.x.rect.left &&
    a.x.rect.top === b.x.rect.top &&
    a.x.rect.width === b.x.rect.width &&
    a.x.rect.height === b.x.rect.height,
);

/**
 * The handles of the selection, drawn after every hit area so that no box above can cover them, and outside the page's clip. A handle's hit area is at least
 * 44 x 44 screen px at any zoom (CSS ::before); on a box too small for that the handles sit outside the corners and an edge handle is left out where its
 * side is shorter than a hit area (the panel and the keyboard still resize it).
 */
function Chrome({
  x,
  k,
  t,
  readOnly,
  api,
}: {
  x: Target;
  k: number;
  t: TFunction;
  readOnly: boolean;
  api: Api;
}) {
  const w = x.rect.width * k;
  const h = x.rect.height * k;
  const small = Math.min(w, h) < SMALL;
  return (
    <div className={`ed-chrome${small ? " is-small" : ""}`} style={boxStyle(x, k)}>
      {HANDLES.filter((c) =>
        c === "n" || c === "s" ? w >= HIT : c === "e" || c === "w" ? h >= HIT : true,
      ).map((c) => (
        <button
          key={c}
          type="button"
          tabIndex={-1}
          className={`ed-handle ed-h-${c}`}
          aria-label={t(`editor.handle.${c}`)}
          aria-disabled={readOnly}
          onPointerDown={(e) => api.down(e, x, "resize", c)}
        />
      ))}
      <button
        type="button"
        tabIndex={-1}
        className="ed-handle ed-rotate"
        aria-label={t("editor.handle.rotate")}
        aria-disabled={readOnly}
        onPointerDown={(e) => api.down(e, x, "rotate")}
      />
    </div>
  );
}
