// Question 1: how a layout stores a box so that one layout prints well on A5, A4 and Letter.
// The book draws every page in a box W = 816 design px wide whose height follows the page ratio (web/src/book/sizes.ts).
// A candidate = {store(box on the A5 reference page, in design px) -> stored form, place(size, stored) -> box in design px of that page,
// unplace(size, box drawn on that page) -> stored form (the inverse of place), unstore(stored) -> box on the A5 reference page (the inverse of store)}.
// The editor's gestures work on the box as DRAWN on the page being edited and write back unplace(...): that is "editing in the stored form".
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
    unplace: (s, r) => ({ x: r.left / W, y: r.top / H(s), w: r.width / W, h: r.height / H(s) }),
    unstore: (p) => ({ x: p.x * W, y: p.y * H5, w: p.w * W, h: p.h * H5 }),
  },
  // B: millimetres on the A5 reference page, uniform scale by page width, top-left anchor
  mm: {
    store: (b) => ({ x: (b.x * 148) / W, y: (b.y * 148) / W, w: (b.w * 148) / W, h: (b.h * 148) / W }),
    place: (s, p) => ({ left: (p.x * W) / 148, top: (p.y * W) / 148, width: (p.w * W) / 148, height: (p.h * W) / 148 }),
    unplace: (s, r) => ({ x: (r.left * 148) / W, y: (r.top * 148) / W, w: (r.width * 148) / W, h: (r.height * 148) / W }),
    unstore: (p) => ({ x: (p.x * W) / 148, y: (p.y * W) / 148, w: (p.w * W) / 148, h: (p.h * W) / 148 }),
  },
  // C: width units for x, w, h (uniform, shape kept); the vertical place is the centre as a fraction of the page height
  centre: {
    store: (b) => ({ x: b.x / W, cy: (b.y + b.h / 2) / H5, w: b.w / W, h: b.h / W }),
    place: (s, p) => ({ left: p.x * W, top: p.cy * H(s) - (p.h * W) / 2, width: p.w * W, height: p.h * W }),
    unplace: (s, r) => ({ x: r.left / W, cy: (r.top + r.height / 2) / H(s), w: r.width / W, h: r.height / W }),
    unstore: (p) => ({ x: p.x * W, y: p.cy * H5 - (p.h * W) / 2, w: p.w * W, h: p.h * W }),
  },
  // D: width units everywhere; a box in the lower half counts its y from the bottom edge, a box in the upper half from the top edge
  anchor: {
    store: (b) => ({ x: b.x, y: b.y, w: b.w, h: b.h, ay: b.y + b.h / 2 > H5 / 2 ? "b" : "t" }),
    place: (s, p) => ({ left: p.x, top: p.ay === "b" ? p.y + H(s) - H5 : p.y, width: p.w, height: p.h }),
    // the anchor edge is decided on the page being edited (the half the box centre is in there), then y is counted from that edge
    unplace: (s, r) => {
      const ay = r.top + r.height / 2 > H(s) / 2 ? "b" : "t";
      return { x: r.left, y: ay === "b" ? r.top - H(s) + H5 : r.top, w: r.width, h: r.height, ay };
    },
    unstore: (p) => ({ x: p.x, y: p.y, w: p.w, h: p.h }), // the A5 y of an "b" box is the same number; the anchor itself stays in the stored form (item.st)
  },
};
// Render rule for every candidate: a box that would leave the page is moved back inside it (clamp) before it is drawn.
export const clamp = (s, r) => ({ ...r, left: Math.min(Math.max(r.left, 0), W - r.width), top: Math.min(Math.max(r.top, 0), H(s) - r.height) });
export const place = (unit, s, stored, doClamp = false) => {
  const r = UNITS[unit].place(s, stored);
  return doClamp ? clamp(s, r) : r;
};
