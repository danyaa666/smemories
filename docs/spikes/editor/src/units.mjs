// Question 1: how a layout stores a box so that one layout prints well on A5, A4 and Letter.
// The book draws every page in a box W = 816 design px wide whose height follows the page ratio (web/src/book/sizes.ts).
// A candidate = {store(box on the A5 reference page, in design px) -> stored form, place(stored, size) -> box in design px of that page}.
export const W = 816;
export const SIZES = { A5: { w: 148, h: 210 }, A4: { w: 210, h: 297 }, Letter: { w: 215.9, h: 279.4 } };
export const H = (s) => Math.round((W * SIZES[s].h) / SIZES[s].w); // same as designHeight()
export const MM = (s) => SIZES[s].w / W; // mm per design px on that page
const H5 = H("A5");

export const UNITS = {
  // A: fractions of the page width and height (x, y, w, h all in 0..1)
  pct: {
    store: (b) => ({ x: b.x / W, y: b.y / H5, w: b.w / W, h: b.h / H5 }),
    place: (s, p) => ({ left: p.x * W, top: p.y * H(s), width: p.w * W, height: p.h * H(s) }),
  },
  // B: millimetres on the A5 reference page, uniform scale by page width, top-left anchor
  mm: {
    store: (b) => ({ x: (b.x * 148) / W, y: (b.y * 148) / W, w: (b.w * 148) / W, h: (b.h * 148) / W }),
    place: (s, p) => ({ left: (p.x * W) / 148, top: (p.y * W) / 148, width: (p.w * W) / 148, height: (p.h * W) / 148 }),
  },
  // C: width units for x, w, h (uniform, shape kept); the vertical place is the centre as a fraction of the page height
  centre: {
    store: (b) => ({ x: b.x / W, cy: (b.y + b.h / 2) / H5, w: b.w / W, h: b.h / W }),
    place: (s, p) => ({ left: p.x * W, top: p.cy * H(s) - (p.h * W) / 2, width: p.w * W, height: p.h * W }),
  },
  // D: width units everywhere; a box in the lower half counts its y from the bottom edge, a box in the upper half from the top edge
  anchor: {
    store: (b) => ({ x: b.x, y: b.y, w: b.w, h: b.h, ay: b.y + b.h / 2 > H5 / 2 ? "b" : "t" }),
    place: (s, p) => ({ left: p.x, top: p.ay === "b" ? p.y + H(s) - H5 : p.y, width: p.w, height: p.h }),
  },
};
// Render rule for every candidate: a box that would leave the page is moved back inside it (clamp) before it is drawn.
export const clamp = (s, r) => ({ ...r, left: Math.min(Math.max(r.left, 0), W - r.width), top: Math.min(Math.max(r.top, 0), H(s) - r.height) });
export const place = (unit, s, stored, doClamp = false) => {
  const r = UNITS[unit].place(s, stored);
  return doClamp ? clamp(s, r) : r;
};
