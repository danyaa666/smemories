// In-house drag / resize / rotate / snap with pointer events (no library). Counted for the "lines of glue code" comparison.
// Move, resize and keyboard work on the box AS DRAWN on the page being edited (design px of that page, clamped inside it) and write back
// UNITS[unit].unplace(size, box): the stored form. So a pointer delta is the same displayed distance on A5, A4 and Letter in every unit.
import { useCallback, useRef, useState } from "react";
import { Item, rectOf } from "./items.jsx";
import { clamp, H, UNITS, W } from "./units.mjs";

const SAFE = 28; // safe margin in design px (about 5 mm)
const SNAP = new URLSearchParams(location.search).get("snap") === "off" ? 0 : 6; // snap distance in design px (snap=off: the measurements of pure pointer-to-item distance)
const handle = { position: "absolute", width: 22, height: 22, borderRadius: 11, background: "#8e6ccb", border: "2px solid #fff", touchAction: "none" };

function snapAxis(lo, size, lines) {
  // returns the offset that brings the left edge, centre or right edge of [lo, lo+size] onto the nearest line within SNAP
  let best = null;
  for (const l of lines) for (const e of [lo, lo + size / 2, lo + size]) {
    const d = l - e;
    if (Math.abs(d) <= SNAP && (best === null || Math.abs(d) < Math.abs(best.d))) best = { d, l };
  }
  return best;
}

export function InHouseLayer({ items, setItems, unit, size, onAnnounce }) {
  const root = useRef(null);
  const [sel, setSel] = useState(null);
  const [guides, setGuides] = useState([]);
  const ref = useRef(items);
  ref.current = items;
  const selRef = useRef(null);
  selRef.current = sel;
  const patch = useCallback((id, p) => setItems((all) => all.map((i) => (i.id === id ? { ...i, ...p } : i))), [setItems]);
  const active = useRef(null); // pointerId of the gesture in progress: a second finger or button is ignored until it ends
  // write a box drawn on this page (clamped inside it) back as the stored form; box (A5 reference) is derived from it
  const commit = useCallback((id, rect) => {
    const r = clamp(size, rect);
    const st = UNITS[unit].unplace(size, r);
    patch(id, { st, box: UNITS[unit].unstore(st) });
  }, [patch, size, unit]);

  const start = useCallback((e, id, mode) => {
    e.stopPropagation();
    if (active.current !== null) return; // another pointer is mid-gesture
    const wasSelected = selRef.current === id;
    setSel(id);
    if (e.pointerType === "touch" && mode === "move" && !wasSelected) return; // touch: the first tap only selects, so a swipe over an item still scrolls the page
    const it = ref.current.find((i) => i.id === id);
    const k = root.current.getBoundingClientRect().width / W; // screen px per design px (zoom and stage scale included)
    const r0 = rectOf(it, unit, size); // the box as drawn now, on this page
    const sx = e.clientX, sy = e.clientY;
    const el = document.getElementById(id).getBoundingClientRect();
    const c = { x: el.left + el.width / 2, y: el.top + el.height / 2 };
    const others = ref.current.filter((i) => i.id !== id).map((i) => rectOf(i, unit, size));
    const xs = [0, SAFE, W / 2, W - SAFE, W, ...others.flatMap((o) => [o.left, o.left + o.width / 2, o.left + o.width])];
    const ys = [0, SAFE, H(size) / 2, H(size) - SAFE, H(size), ...others.flatMap((o) => [o.top, o.top + o.height / 2, o.top + o.height])];
    const pid = e.pointerId;
    active.current = pid;
    const move = (ev) => {
      if (ev.pointerId !== pid) return;
      const dx = (ev.clientX - sx) / k, dy = (ev.clientY - sy) / k;
      if (mode === "move") {
        let left = r0.left + dx, top = r0.top + dy;
        const sxn = snapAxis(left, r0.width, xs), syn = snapAxis(top, r0.height, ys);
        if (sxn) left += sxn.d;
        if (syn) top += syn.d;
        setGuides([sxn && { v: sxn.l }, syn && { h: syn.l }].filter(Boolean));
        commit(id, { ...r0, left, top });
      } else if (mode === "resize") {
        const width = Math.min(W, Math.max(24, r0.width + dx));
        commit(id, { ...r0, width, height: it.kind === "text" ? Math.min(H(size), Math.max(24, r0.height + dy)) : (width * r0.height) / r0.width });
      } else {
        let a = (Math.atan2(ev.clientY - c.y, ev.clientX - c.x) * 180) / Math.PI + 90;
        a = Math.round(a * 10) / 10;
        const near = Math.round(a / 15) * 15;
        patch(id, { r: Math.abs(a - near) < 3 ? near : a });
      }
    };
    const up = (ev) => {
      if (ev.pointerId !== pid) return;
      active.current = null;
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      setGuides([]);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
  }, [patch, commit, size, unit]);

  const key = useCallback((e, id) => {
    const it = ref.current.find((i) => i.id === id);
    const step = e.shiftKey ? 10 : 1; // design px of the displayed page
    const arrows = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[e.key];
    const r = rectOf(it, unit, size);
    if (arrows && e.altKey) commit(id, { ...r, width: Math.max(24, r.width + arrows[0] * step), height: Math.max(24, r.height + arrows[1] * step) });
    else if (arrows) commit(id, { ...r, left: r.left + arrows[0] * step, top: r.top + arrows[1] * step });
    else if (e.key === "[" || e.key === "]") patch(id, { r: (it.r ?? 0) + (e.key === "[" ? -5 : 5) });
    else return;
    e.preventDefault();
    onAnnounce?.(`${it.label}`);
  }, [patch, commit, onAnnounce, size, unit]);

  return (
    <div ref={root} style={{ position: "absolute", inset: 0 }} onPointerDown={(e) => e.isPrimary && setSel(null)} data-layer="inhouse">
      {items.map((it) => (
        <Item key={it.id} item={it} unit={unit} size={size} selected={sel === it.id}
          onPointerDown={(e) => start(e, it.id, "move")} onKeyDown={(e) => key(e, it.id)}>
          {sel === it.id && (
            <>
              <div data-handle="resize" onPointerDown={(e) => start(e, it.id, "resize")} style={{ ...handle, right: -11, bottom: -11, cursor: "nwse-resize" }} />
              <div data-handle="rotate" onPointerDown={(e) => start(e, it.id, "rotate")} style={{ ...handle, left: "calc(50% - 11px)", top: -40, cursor: "grab" }} />
            </>
          )}
        </Item>
      ))}
      {guides.map((g, i) => <div key={i} style={g.v !== undefined ? { position: "absolute", left: g.v, top: 0, bottom: 0, width: 1, background: "#e0457b" } : { position: "absolute", top: g.h, left: 0, right: 0, height: 1, background: "#e0457b" }} />)}
    </div>
  );
}
