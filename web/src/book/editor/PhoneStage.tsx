import {
  memo,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { BookPages } from "../BookPages";
import { pageBox } from "../layout/geometry";
import { designZoom, pagePx, SIZES } from "../sizes";
import { boxStyle, useMeasured } from "./measure";
import { pageName, targetName } from "./names";
import { targetsOf, type Target } from "./targets";
import type { StageProps } from "./EditorBody";
import "../book.css";

const PAD = 8; // screen px of the screen's margin around the page
const noop = () => {};

/**
 * The page on a phone (T-181): the same page renderer and the same targets as the desktop stage, fit to the screen width and read-only. There are no pointer
 * handlers, no handles, no snapping and no gesture code at all (nothing is imported from `./gestures`, so a phone never loads it): a tap on an item selects it,
 * a tap beside the items clears the selection, and a swipe always scrolls the screen. Moving, sizing and typing happen in the bottom sheet.
 */
export function PhoneStage({
  data,
  size,
  bookT,
  page,
  kind,
  spec,
  selection,
  onSelect,
  onTargets,
  Fit,
  cut,
}: StageProps) {
  const { t } = useTranslation();
  const scroll = useRef<HTMLDivElement>(null);
  const box = useRef<HTMLDivElement>(null);
  const [availW, setAvailW] = useState(0);
  useLayoutEffect(() => {
    const el = scroll.current;
    if (!el) return;
    const update = () => setAvailW(el.clientWidth - 2 * PAD);
    update();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const px = pagePx(size);
  const s = availW > 0 ? Math.round(Math.min(2, Math.max(0.1, availW / px.w)) * 1000) / 1000 : 0.4;
  const k = designZoom(size) * s;
  const pb = useMemo(() => pageBox(size), [size]);
  const measured = useMeasured(box, kind, spec, data, page, s);
  const targets = useMemo(
    () =>
      targetsOf(data.layout ?? { v: 1, pages: {} }, kind, spec, pb, measured.rects, measured.order),
    [data.layout, kind, spec, pb, measured],
  );
  useEffect(() => onTargets(targets), [onTargets, targets]);
  // a tap beside every item clears the selection
  useEffect(() => {
    const el = scroll.current;
    if (!el) return;
    const click = (e: MouseEvent) => {
      if (!(e.target as Element).closest(".ed-box")) onSelect([]);
    };
    el.addEventListener("click", click);
    return () => el.removeEventListener("click", click);
  }, [onSelect]);
  const vars = {
    "--pw": `${SIZES[size].w}mm`,
    "--ph": `${SIZES[size].h}mm`,
    "--s": s,
  } as CSSProperties;
  return (
    <section
      className="ed-stage ed-pstage"
      aria-label={t("editor.stage", { page: pageName(t, page) })}
    >
      {cut && (
        <p role="status" className="ed-warn">
          {t("editor.textCut")}
        </p>
      )}
      <div ref={scroll} className="ed-scroll">
        <div ref={box} className="ed-pagebox" style={vars} data-zoom={Math.round(s * 100)}>
          <Fit>
            <BookPages
              data={data}
              size={size}
              t={bookT}
              eager={false}
              track={noop}
              only={page}
              edit={undefined}
            />
          </Fit>
          <div className="ed-overlay">
            {targets.map((x) => (
              <Tap
                key={x.id}
                x={x}
                k={k}
                t={t}
                selected={selection.includes(x.id)}
                onSelect={onSelect}
              />
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

/** The tap area of one target: a button over its drawn box. Later boxes are drawn later, so a tap on an overlap picks the topmost. */
const Tap = memo(function Tap({
  x,
  k,
  t,
  selected,
  onSelect,
}: {
  x: Target;
  k: number;
  t: TFunction;
  selected: boolean;
  onSelect: (ids: readonly string[]) => void;
}) {
  return (
    <div
      className={`ed-box${selected ? " is-selected" : ""}${x.hidden ? " is-hidden" : ""}`}
      style={boxStyle(x, k)}
      data-target={x.id}
    >
      <button
        type="button"
        className="ed-hit"
        aria-pressed={selected}
        aria-label={targetName(t, x)}
        onClick={() => onSelect([x.id])}
      />
    </div>
  );
});
