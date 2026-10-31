import { memo } from "react";
import { place, UNITS } from "./units.mjs";

// An item keeps its box {x,y,w,h} in design px of the A5 reference page; the unit candidate stores and places it for the page size drawn.
// After a gesture the item also keeps `st`, the stored form written by the gesture (UNITS[unit].unplace of the box drawn on the edited page); `st` wins over box.
export const HEART = "M12 20.5C5 15.5 2.5 12 2.5 8.6 2.5 5.9 4.6 4 7 4c2.1 0 3.9 1.3 5 3 1.1-1.7 2.9-3 5-3 2.4 0 4.5 1.9 4.5 4.6 0 3.4-2.5 6.9-9.5 11.9z";
export const STAR = "M12 2.5l2.9 6.1 6.6.8-4.9 4.5 1.3 6.6L12 17.2l-5.9 3.3 1.3-6.6-4.9-4.5 6.6-.8z";

export const Sticker = ({ d = HEART, color = "#d9668c" }) => (
  <svg viewBox="0 0 24 24" width="100%" height="100%" preserveAspectRatio="none" aria-hidden="true">
    <path d={d} fill={color} stroke="#3b2f4a" strokeWidth="0.8" strokeLinejoin="round" />
  </svg>
);

/** The text box CSS is shared by the printed page, the editor and the textarea: same font, size, line height, wrapping. */
export const TEXT_CSS = {
  fontFamily: "var(--mb-cute, 'Patrick Hand', 'Be Vietnam Pro', sans-serif)",
  fontSize: 30,
  lineHeight: 1.25,
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  color: "#3b2f4a",
  padding: 0,
  margin: 0,
  boxSizing: "border-box",
};

/** The box of an item as drawn on page `size` (design px, origin at the page corner). */
export const rectOf = (item, unit, size, clamp = true) => place(unit, size, item.st ?? UNITS[unit].store(item.box), clamp);

export const Item = memo(function Item({ item, unit, size, selected, onPointerDown, onKeyDown, children, clamp = true }) {
  const r = rectOf(item, unit, size, clamp);
  return (
    <div
      id={item.id}
      data-item
      tabIndex={onKeyDown ? 0 : undefined}
      role={onKeyDown ? "button" : undefined}
      aria-label={onKeyDown ? item.label : undefined}
      onPointerDown={onPointerDown}
      onKeyDown={onKeyDown}
      style={{
        position: "absolute",
        left: r.left,
        top: r.top,
        width: r.width,
        height: r.height,
        transform: item.r ? `rotate(${item.r}deg)` : undefined,
        zIndex: item.z ?? 1,
        touchAction: selected ? "none" : undefined,
        outline: selected ? "2px solid #8e6ccb" : undefined,
        cursor: onPointerDown ? "move" : undefined,
      }}
    >
      {item.kind === "text" ? <div style={{ ...TEXT_CSS, width: "100%", height: "100%", overflow: "hidden" }}>{item.text}</div> : <Sticker d={item.d} color={item.color} />}
      {children}
    </div>
  );
});

/** Draws every item, no interaction: this is the print / read-only layer. */
export function StaticLayer({ items, unit, size, clamp = true }) {
  return items.map((it) => <Item key={it.id} item={it} unit={unit} size={size} clamp={clamp} />);
}
