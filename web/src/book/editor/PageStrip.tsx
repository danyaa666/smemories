import type { TFunction } from "i18next";
import { memo, useEffect, useRef, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import type { BookData } from "../../api/book";
import { BookPages, type OnePage } from "../BookPages";
import { pagePx, SIZES, type PageSize } from "../sizes";
import { pageName } from "./names";

const THUMB_W = 88; // screen px

/** True once the element has been near the viewport (always true where IntersectionObserver does not exist): a 300-page book does not draw 300 thumbnails at once. */
function useSeen<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [seen, setSeen] = useState(typeof IntersectionObserver === "undefined");
  useEffect(() => {
    const el = ref.current;
    if (seen || !el) return;
    const io = new IntersectionObserver((e) => e.some((x) => x.isIntersecting) && setSeen(true), {
      rootMargin: "200px",
    });
    io.observe(el);
    return () => io.disconnect();
  }, [seen]);
  return { ref, seen };
}

const noop = () => {};

const Thumb = memo(function Thumb({
  data,
  size,
  bookT,
  page,
}: {
  data: BookData;
  size: PageSize;
  bookT: TFunction;
  page: OnePage;
}) {
  const { ref, seen } = useSeen<HTMLSpanElement>();
  const style = {
    "--pw": `${SIZES[size].w}mm`,
    "--ph": `${SIZES[size].h}mm`,
    "--s": THUMB_W / pagePx(size).w,
  } as CSSProperties;
  return (
    <span ref={ref} className="ed-thumb-view" style={style} aria-hidden="true">
      {seen && (
        <BookPages data={data} size={size} t={bookT} eager={false} track={noop} only={page} />
      )}
    </span>
  );
});

/** The page strip (T-159): one thumbnail per page in book order, drawn by the real book components; arrow keys move between pages. */
export function PageStrip({
  data,
  size,
  bookT,
  pages,
  current,
  onPick,
}: {
  data: BookData;
  size: PageSize;
  bookT: TFunction;
  pages: OnePage[];
  current: number;
  onPick: (i: number) => void;
}) {
  const { t } = useTranslation();
  const buttons = useRef<(HTMLButtonElement | null)[]>([]);
  const move = (to: number) => {
    const i = Math.min(Math.max(to, 0), pages.length - 1);
    onPick(i);
    buttons.current[i]?.focus();
  };
  const key = (e: React.KeyboardEvent, i: number) => {
    const to =
      e.key === "ArrowDown" || e.key === "ArrowRight"
        ? i + 1
        : e.key === "ArrowUp" || e.key === "ArrowLeft"
          ? i - 1
          : e.key === "Home"
            ? 0
            : e.key === "End"
              ? pages.length - 1
              : null;
    if (to === null) return;
    e.preventDefault();
    move(to);
  };
  return (
    <nav className="ed-strip" aria-label={t("editor.strip")}>
      <ol>
        {pages.map((p, i) => (
          <li key={`${p.section}${p.index ?? ""}`}>
            <button
              type="button"
              ref={(el) => {
                buttons.current[i] = el;
              }}
              className="ed-thumb"
              aria-current={i === current ? "page" : undefined}
              aria-label={t("editor.pageOf", {
                n: i + 1,
                total: pages.length,
                name: pageName(t, p),
              })}
              tabIndex={i === current ? 0 : -1}
              onClick={() => onPick(i)}
              onKeyDown={(e) => key(e, i)}
            >
              <Thumb data={data} size={size} bookT={bookT} page={p} />
              <span className="ed-thumb-name">{pageName(t, p)}</span>
            </button>
          </li>
        ))}
      </ol>
    </nav>
  );
}
