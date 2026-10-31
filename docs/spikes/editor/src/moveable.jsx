// The same editor on react-moveable (MIT). Counted for the "lines of glue code" comparison.
import { useCallback, useRef, useState } from "react";
import Moveable from "react-moveable";
import { Item } from "./items.jsx";
import { H, W } from "./units.mjs";

const SAFE = 28;

export function MoveableLayer({ items, setItems, unit, size }) {
  const [target, setTarget] = useState(null);
  const ref = useRef(items);
  ref.current = items;
  const patch = useCallback((id, p) => setItems((all) => all.map((i) => (i.id === id ? { ...i, ...p } : i))), [setItems]);
  const id = target?.id;
  const it = items.find((i) => i.id === id);
  return (
    <div style={{ position: "absolute", inset: 0 }} data-layer="moveable" onPointerDown={(e) => e.target === e.currentTarget && setTarget(null)}>
      {items.map((i) => <Item key={i.id} item={i} unit={unit} size={size} selected={id === i.id} onPointerDown={(e) => { e.stopPropagation(); setTarget(document.getElementById(i.id)); }} />)}
      {it && (
        <Moveable
          target={target}
          container={target.parentElement}
          draggable resizable rotatable snappable
          keepRatio={it.kind !== "text"}
          snapThreshold={6}
          snapDirections={{ left: true, center: true, right: true, top: true, middle: true, bottom: true }}
          verticalGuidelines={[0, SAFE, W / 2, W - SAFE, W]}
          horizontalGuidelines={[0, SAFE, H(size) / 2, H(size) - SAFE, H(size)]}
          elementGuidelines={items.filter((x) => x.id !== id).map((x) => document.getElementById(x.id)).filter(Boolean)}
          rotationPosition="top"
          throttleRotate={15}
          onDrag={(e) => { e.target.style.left = `${e.left}px`; e.target.style.top = `${e.top}px`; }}
          onDragEnd={(e) => e.lastEvent && patch(id, { box: { ...it.box, x: e.lastEvent.left, y: e.lastEvent.top } })}
          onResize={(e) => { e.target.style.width = `${e.width}px`; e.target.style.height = `${e.height}px`; e.target.style.left = `${e.drag.left}px`; e.target.style.top = `${e.drag.top}px`; }}
          onResizeEnd={(e) => e.lastEvent && patch(id, { box: { x: e.lastEvent.drag.left, y: e.lastEvent.drag.top, w: e.lastEvent.width, h: e.lastEvent.height } })}
          onRotate={(e) => { e.target.style.transform = e.drag.transform; }}
          onRotateEnd={(e) => e.lastEvent && patch(id, { r: e.lastEvent.rotate })}
        />
      )}
    </div>
  );
}
