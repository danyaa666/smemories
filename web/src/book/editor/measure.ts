import { useLayoutEffect, useState, type CSSProperties, type RefObject } from "react";
import type { BookData } from "../../api/book";
import type { OnePage } from "../BookPages";
import type { Rect } from "../layout/geometry";
import { kindOf, type TemplateLayout } from "../layout/runtime";
import { DESIGN_W } from "../sizes";
import { unrotate, type Target } from "./targets";

// What the desktop stage (Stage.tsx) and the phone stage (PhoneStage.tsx, T-181) share: where the template draws its own elements, and the box of a target on screen.

/** Where the template draws each unmoved element of the page, measured on the stage (design px, unrotated); the template's natural place is the fallback. */
export function useMeasured(
  root: RefObject<HTMLElement | null>,
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

export const boxStyle = (x: Target, k: number): CSSProperties => ({
  left: x.rect.left * k,
  top: x.rect.top * k,
  width: x.rect.width * k,
  height: x.rect.height * k,
  transform: x.r ? `rotate(${x.r}deg)` : undefined,
});
