// Question 4: undo for a layout document of up to 128 KiB. node --expose-gc scripts/undo.mjs -> results/undo.json
import { writeFileSync } from "node:fs";

const RUNES = "Việt Nam đường về nhà, những ngày xưa ơi. ";
const mkDoc = () => {
  const items = [];
  for (let i = 0; ; i++) {
    items.push({ id: `i${i}`, kind: i % 3 ? "sticker" : "text", ref: `sticker.heart${i % 9}`, x: i * 5, y: i * 7, w: 120, h: 80, r: i % 30, z: i, text: i % 3 ? undefined : RUNES.repeat(11).slice(0, 460) });
    if (JSON.stringify({ items }).length > 126 * 1024) break;
  }
  return { v: 1, pages: { cover: { items, overrides: {}, background: null } } };
};
const doc0 = mkDoc();
const bytes = JSON.stringify(doc0).length;
const COUNT = doc0.pages.cover.items.length;
const GESTURES = 2000, DEPTH = 100;
const heap = () => { global.gc(); return process.memoryUsage().heapUsed; };
const move = (d, n) => ({ ...d, pages: { ...d.pages, cover: { ...d.pages.cover, items: d.pages.cover.items.map((it, i) => (i === n % COUNT ? { ...it, x: it.x + 1 } : it)) } } });
const out = { doc_bytes: bytes, items: COUNT, gestures: GESTURES, depth: DEPTH };

// A. snapshot stack of references (React state is immutable already: a snapshot is the old object, shared structure)
{
  let doc = doc0; const stack = [];
  const h0 = heap(), t0 = performance.now();
  for (let g = 0; g < GESTURES; g++) { stack.push(doc); if (stack.length > DEPTH) stack.shift(); doc = move(doc, g); }
  const ms = performance.now() - t0;
  out.snapshot_refs = { total_ms: Math.round(ms), per_gesture_us: Math.round((ms / GESTURES) * 1000), heap_mb: Math.round((heap() - h0) / 1048.576) / 1000 };
  const t1 = performance.now(); let d = doc; for (let i = 0; i < DEPTH; i++) d = stack.pop(); out.snapshot_refs.undo_100_steps_ms = Math.round((performance.now() - t1) * 100) / 100; void d;
}
// B. snapshot stack of JSON strings (what you need if state is mutated in place or stored deep-cloned)
{
  let doc = JSON.parse(JSON.stringify(doc0)); const stack = [];
  const h0 = heap(), t0 = performance.now();
  for (let g = 0; g < GESTURES; g++) { stack.push(JSON.stringify(doc)); if (stack.length > DEPTH) stack.shift(); doc = move(doc, g); }
  const ms = performance.now() - t0;
  out.snapshot_json = { total_ms: Math.round(ms), per_gesture_us: Math.round((ms / GESTURES) * 1000), heap_mb: Math.round((heap() - h0) / 1048.576) / 1000 };
  const t1 = performance.now(); JSON.parse(stack.pop()); out.snapshot_json.undo_one_step_ms = Math.round((performance.now() - t1) * 100) / 100;
}
// C. command stack: {item, before, after} for a move; needs an inverse per operation kind
{
  let doc = doc0; const stack = [];
  const h0 = heap(), t0 = performance.now();
  for (let g = 0; g < GESTURES; g++) { const n = g % COUNT; stack.push({ op: "box", n, before: doc.pages.cover.items[n].x, after: doc.pages.cover.items[n].x + 1 }); if (stack.length > DEPTH) stack.shift(); doc = move(doc, g); }
  const ms = performance.now() - t0;
  out.command_stack = { total_ms: Math.round(ms), per_gesture_us: Math.round((ms / GESTURES) * 1000), heap_mb: Math.round((heap() - h0) / 1048.576) / 1000, operation_kinds_needing_an_inverse: "move/resize/rotate, add, delete, reorder (z), text edit, hide/show, background, paste of many, restore version (about 9)" };
}
writeFileSync(new URL("../results/undo.json", import.meta.url), JSON.stringify(out, null, 1));
console.log(out);
