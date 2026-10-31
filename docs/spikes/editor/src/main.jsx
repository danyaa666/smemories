import { StrictMode, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { Sheet } from "./book.jsx";
import { InHouseLayer } from "./inhouse.jsx";
import { HEART, STAR, StaticLayer } from "./items.jsx";
import { MoveableLayer } from "./moveable.jsx";
import { TextLab } from "./text.jsx";
import { H, SIZES, UNITS, W } from "./units.mjs";

const q = new URLSearchParams(location.search);
const view = q.get("view") ?? "editor";
const size = q.get("size") ?? "A5";
const unit = q.get("unit") ?? "anchor";
const lib = q.get("lib") ?? "inhouse";
const scale = Number(q.get("scale") ?? 1);
const n = Number(q.get("n") ?? 2);

/** The two items of the question 1 test: one sticker near the bottom edge and one text box (boxes in design px of the A5 reference page). */
export const demo = () => [
  { id: "s1", kind: "sticker", label: "Heart sticker", box: { x: 560, y: 950, w: 150, h: 150 }, r: 12, d: HEART, color: "#d9668c" },
  { id: "t1", kind: "text", label: "Text box", box: { x: 80, y: 1040, w: 440, h: 90 }, r: -4, text: "Việt Nam — đường về nhà" },
];
/** n items: a grid of small stickers and text boxes, deterministic. */
export const many = (count) => {
  const out = [];
  for (let i = 0; i < count; i++) {
    const col = i % 10, row = Math.floor(i / 10);
    const isText = i % 7 === 3;
    out.push({ id: `m${i}`, kind: isText ? "text" : "sticker", label: `Item ${i}`, d: i % 2 ? STAR : HEART, color: ["#d9668c", "#8e6ccb", "#e09a2d"][i % 3],
      box: { x: 30 + col * 76, y: 120 + row * 96, w: isText ? 140 : 60, h: isText ? 60 : 60 }, r: (i * 13) % 40 - 20, text: "Việt Nam" });
  }
  return out;
};

function Editor() {
  const [items, setItems] = useState(() => (n <= 2 ? demo() : many(n)));
  const live = useRef(items);
  live.current = items;
  useEffect(() => { window.__items = () => live.current; });
  const Layer = lib === "moveable" ? MoveableLayer : InHouseLayer;
  return (
    <div style={{ padding: 8 }}>
      <Sheet size={size} scale={scale}>
        <Layer items={items} setItems={setItems} unit={unit} size={size} />
      </Sheet>
    </div>
  );
}

function Grid() {
  return (
    <div style={{ display: "grid", gridTemplateColumns: "auto repeat(3, auto)", gap: 8, padding: 8, font: "12px sans-serif" }}>
      {Object.keys(UNITS).flatMap((u) => [
        <b key={u}>{u}</b>,
        ...Object.keys(SIZES).map((s) => (
          <div key={u + s} data-cell={`${u}-${s}`}>
            {s} {W}x{H(s)}
            <Sheet size={s} scale={0.4}><StaticLayer items={demo()} unit={u} size={s} /></Sheet>
          </div>
        )),
      ])}
    </div>
  );
}

function Print() {
  return <Sheet size={size} print><StaticLayer items={demo()} unit={unit} size={size} /></Sheet>;
}

const App = { editor: Editor, grid: Grid, print: Print, text: TextLab }[view];
createRoot(document.getElementById("root")).render(<StrictMode><App /></StrictMode>);
