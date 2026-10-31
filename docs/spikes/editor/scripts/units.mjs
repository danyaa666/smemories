// Question 1 numbers (pure arithmetic, no browser): node scripts/units.mjs  -> results/units.json and a table on stdout.
import { writeFileSync } from "node:fs";
import { clamp, H, MM, place, SIZES, UNITS } from "../src/units.mjs";

const H5 = H("A5");
// boxes on the A5 reference page, in design px: [name, x, y, w, h]
const BOXES = {
  photo: [190, 410, 370, 400],
  sticker_bottom: [560, 950, 150, 150], // box bottom 1100 = 58 px (11 mm) above the A5 bottom edge 1158
  sticker_flush: [560, H5 - 150, 150, 150], // touches the bottom edge on A5
  text_bottom: [80, 1040, 440, 90],
  title_top: [100, 64, 616, 120],
};
const PAIR = { upper: [100, 440, 300, 100], lower: [100, 560, 300, 80] }; // 20 px apart on A5, centres 160 px apart, either side of the page middle (579)
const mm = (px, s) => Math.round(px * MM(s) * 10) / 10;
const out = {};
for (const unit of Object.keys(UNITS)) {
  out[unit] = {};
  for (const s of Object.keys(SIZES)) {
    const row = { pageH_px: H(s), boxes: {} };
    for (const [name, [x, y, w, h]] of Object.entries(BOXES)) {
      const st = UNITS[unit].store({ x, y, w, h });
      const r = UNITS[unit].place(s, st); // as stored, no rescue
      const bottomOverflow = Math.max(0, r.top + r.height - H(s));
      row.boxes[name] = {
        aspect_change_pct: Math.round(((r.width / r.height) / (w / h) - 1) * 1000) / 10,
        bottom_overflow_mm: mm(bottomOverflow, s),
        bottom_overflow_pct_of_box: Math.round((bottomOverflow / r.height) * 1000) / 10,
        clamped_shift_mm: mm(Math.abs(clamp(s, r).top - r.top), s),
        gap_to_bottom_mm: mm(H(s) - (r.top + r.height), s),
        invisible: r.top >= H(s),
      };
    }
    const a = place(unit, s, UNITS[unit].store({ x: 100, y: PAIR.upper[1], w: 300, h: PAIR.upper[3] }), true);
    const b = place(unit, s, UNITS[unit].store({ x: 100, y: PAIR.lower[1], w: 300, h: PAIR.lower[3] }), true);
    row.pair_gap_px = Math.round(b.top - (a.top + a.height)); // 20 on A5; negative = the two boxes overlap
    out[unit][s] = row;
  }
}
writeFileSync(new URL("../results/units.json", import.meta.url), JSON.stringify(out, null, 1));
const f = (v) => String(v).padStart(7);
console.log("unit    size   | aspect% (sticker) | bottom overflow mm (flush sticker / bottom text) | gap to bottom mm (sticker_bottom) | pair gap px (A5: 20)");
for (const unit of Object.keys(out)) for (const s of Object.keys(SIZES)) {
  const r = out[unit][s];
  console.log(unit.padEnd(7), s.padEnd(6), "|", f(r.boxes.sticker_bottom.aspect_change_pct), "|", f(r.boxes.sticker_flush.bottom_overflow_mm), f(r.boxes.text_bottom.bottom_overflow_mm), "|",
    f(r.boxes.sticker_bottom.gap_to_bottom_mm), "|", f(r.pair_gap_px), r.boxes.sticker_bottom.invisible ? " INVISIBLE" : "");
}
