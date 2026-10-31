// In-house drag / resize / rotate / snap with pointer events (no library). Counted for the "lines of glue code" comparison.
import { useCallback, useRef, useState } from "react";
import { Item } from "./items.jsx";
import { H, W } from "./units.mjs";

const SAFE = 28; // safe margin in design px (about 5 mm)
const SNAP = 6; // snap distance in design px
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

  const start = useCallback((e, id, mode) => {
    e.stopPropagation();
    const wasSelected = selRef.current === id;
    setSel(id);
    if (e.pointerType === "touch" && mode === "move" && !wasSelected) return; // touch: the first tap only selects, so a swipe over an item still scrolls the page
    const it = ref.current.find((i) => i.id === id);
    const k = root.current.getBoundingClientRect().width / W; // screen px per design px (zoom and stage scale included)
    const b0 = it.box, r0 = it.r ?? 0, sx = e.clientX, sy = e.clientY;
    const el = document.getElementById(id).getBoundingClientRect();
    const c = { x: el.left + el.width / 2, y: el.top + el.height / 2 };
    const others = ref.current.filter((i) => i.id !== id).map((i) => i.box);
    const xs = [0, SAFE, W / 2, W - SAFE, W, ...others.flatMap((o) => [o.x, o.x + o.w / 2, o.x + o.w])];
    const ys = [0, SAFE, H(size) / 2, H(size) - SAFE, H(size), ...others.flatMap((o) => [o.y, o.y + o.h / 2, o.y + o.h])];
    const move = (ev) => {
      const dx = (ev.clientX - sx) / k, dy = (ev.clientY - sy) / k;
      if (mode === "move") {
        let box = { ...b0, x: b0.x + dx, y: b0.y + dy };
        const sxn = snapAxis(box.x, box.w, xs), syn = snapAxis(box.y, box.h, ys);
        if (sxn) box.x += sxn.d;
        if (syn) box.y += syn.d;
        setGuides([sxn && { v: sxn.l }, syn && { h: syn.l }].filter(Boolean));
        patch(id, { box });
      } else if (mode === "resize") {
        const w = Math.max(24, b0.w + dx);
        patch(id, { box: { ...b0, w, h: it.kind === "text" ? Math.max(24, b0.h + dy) : (w * b0.h) / b0.w } });
      } else {
        let a = (Math.atan2(ev.clientY - c.y, ev.clientX - c.x) * 180) / Math.PI + 90;
        a = Math.round(a * 10) / 10;
        const near = Math.round(a / 15) * 15;
        patch(id, { r: Math.abs(a - near) < 3 ? near : a });
      }
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      setGuides([]);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
    void r0;
  }, [patch, size]);

  const key = useCallback((e, id) => {
    const it = ref.current.find((i) => i.id === id);
    const step = e.shiftKey ? 10 : 1;
    const arrows = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[e.key];
    if (arrows && e.altKey) patch(id, { box: { ...it.box, w: Math.max(24, it.box.w + arrows[0] * step), h: Math.max(24, it.box.h + arrows[1] * step) } });
    else if (arrows) patch(id, { box: { ...it.box, x: it.box.x + arrows[0] * step, y: it.box.y + arrows[1] * step } });
    else if (e.key === "[" || e.key === "]") patch(id, { r: (it.r ?? 0) + (e.key === "[" ? -5 : 5) });
    else return;
    e.preventDefault();
    onAnnounce?.(`${it.label}`);
  }, [patch, onAnnounce]);

  return (
    <div ref={root} style={{ position: "absolute", inset: 0 }} onPointerDown={() => setSel(null)} data-layer="inhouse">
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
