// Doory design (Fabric JSON, one page) -> our layout page ({bg, overrides, items}) + a report. A pure function: no clock, no random, no I/O.
// ADR 0005 question 1 and 2. Item ids come from a hash of (slug, page, object id), so the same input gives the same bytes.
import { createHash } from "node:crypto";

export const PAPERS = { A5: [148, 210], A4: [210, 297], Letter: [215.9, 279.4] };
export const REF_RATIO = 210 / 148; // the ratio the validator's range check uses (internal/booklayout pageRatioPermille 1419)
export const DESIGN_W = 816; // design px of a page width (web/src/book/sizes.ts)

// Colour tokens of memory-book (internal/templates/embed/memory-book.json design.layout.colors), valued from the default theme `lavender`
// (web/src/book/templates/memory.css: ink, soft, paper, white, accent = --mb-a1).
export const TOKENS = { ink: "#3b2f4a", soft: "#5e5170", paper: "#fffbf6", white: "#ffffff", accent: "#d9668c" };
// Assets that draw a filled shape (internal/assets registry, tintable) and how much of their 100x100 viewBox the shape covers.
// AssetSvg draws a registry SVG with the default preserveAspectRatio ("meet"): in a box that is not square a shape stays square/round and is centred,
// so today only a (near) square box can show a rectangle or an ellipse. With opts.stretch the converter assumes the proposed fix of ADR 0005: two registry
// shapes `shape.rect` and `shape.ellipse` that fill their box edge to edge (preserveAspectRatio="none"); those ids are NOT in the registry yet.
export const SHAPES = { rect: { asset: "shape.rounded-square", cover: 0.88 }, ellipse: { asset: "shape.circle", cover: 0.92 } };
export const PROPOSED = { rect: { asset: "shape.rect", cover: 1 }, ellipse: { asset: "shape.ellipse", cover: 1 } };
const SQUARE_TOLERANCE = 1.08; // longest side over shortest side still counted as square
const DISPLAY = new Set(["Anton", "Oswald", "Great Vibes", "Pinyon Script", "Playfair Display"]); // other families -> the body font
const FONT_MIN = 8, FONT_MAX = 96, TEXT_MAX = 500, ITEMS_MAX = 40, BOX_MIN = 10, BOX_MAX = 2000;

// every control and format character except the newline (the validator's text rule, internal/textx)
const BAD_CHARS = new RegExp("[\\u0000-\\u0009\\u000b-\\u001f\\u007f-\\u009f\\u200b-\\u200f\\u2028-\\u202e\\u2060-\\u206f\\ufeff]", "g");
const hex = (s) => { const m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(s ?? ""); if (!m) return null; let h = m[1]; if (h.length === 3) h = [...h].map((c) => c + c).join(""); return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16)); };
function rgba(s) { // "#rrggbb", "rgb(a)(...)" -> [r,g,b,a] | null
  const h = hex(s); if (h) return [...h, 1];
  const m = /^rgba?\(([^)]+)\)$/.exec(String(s ?? "").trim()); if (!m) return null;
  const p = m[1].split(",").map(Number); return [p[0], p[1], p[2], p.length > 3 ? p[3] : 1];
}
const nearest = (rgb) => Object.entries(TOKENS).map(([t, v]) => [t, hex(v).reduce((a, c, i) => a + (c - rgb[i]) ** 2, 0)]).sort((a, b) => a[1] - b[1] || (a[0] < b[0] ? -1 : 1))[0][0];

/** Plain colour of a Fabric fill: a string, or a gradient (mean of its stops). Returns {rgb, gradient} or null (none / transparent). */
export function plainFill(fill) {
  if (fill == null) return null;
  if (typeof fill === "string") { const c = rgba(fill); return c && c[3] > 0 ? { rgb: c.slice(0, 3), gradient: false } : null; }
  const stops = (fill.colorStops ?? []).map((s) => rgba(s.color)).filter(Boolean);
  if (!stops.length) return null;
  return { rgb: [0, 1, 2].map((i) => Math.round(stops.reduce((a, s) => a + s[i], 0) / stops.length)), gradient: true };
}

const id12 = (...parts) => createHash("sha1").update(parts.join("|")).digest("hex").slice(0, 12);
const clamp = (v, lo, hi) => Math.min(Math.max(v, lo), hi);
const r1 = (v) => Math.round(v);

/** Flattens groups into one list in z-order. Children are relative to the group's centre (or origin); only the exact case is supported: a group
 * without rotation or scale (the one group of the starter templates). */
function flatten(objects) {
  const out = [];
  const walk = (o) => {
    out.push({ o, group: o.type === "group" });
    if (o.type !== "group") return;
    const gw = o.width * (o.scaleX ?? 1), gh = o.height * (o.scaleY ?? 1);
    const cx = o.originX === "center" ? o.left : o.left + gw / 2, cy = o.originY === "center" ? o.top : o.top + gh / 2;
    for (const c of o.objects ?? []) walk({ ...c, left: c.left + cx, top: c.top + cy });
  };
  objects.forEach(walk);
  return out;
}

/**
 * convertPage(design, pageIndex, opts) -> { page, report }
 *   opts: { slug, kind: cover|profile|notes|back, ratio (page height / width, default A5), mode: "fit" | "cover" }
 *   page: { bg: {color}, overrides?, items }  (the value of pages.<kind> in a layout document)
 */
export function convertPage(design, pageIndex, { slug, kind = "cover", ratio = REF_RATIO, mode = "fit", stretch = false } = {}) {
  const pg = design.pages[pageIndex];
  const cw = design.width, ch = design.height;
  // page size policy (Q1): "fit" scales the canvas to lie wholly on the page and centres it (letterbox, the page background fills the rest);
  // "cover" fills the page and crops what falls outside (an object wholly outside is dropped and counted).
  const s = (mode === "cover" ? Math.max : Math.min)(1000 / cw, (1000 * ratio) / ch); // thousandths of the page width per canvas px
  const ox = (1000 - cw * s) / 2, oy = (1000 * ratio - ch * s) / 2;
  const px = DESIGN_W / 1000; // design px per thousandth
  const records = [];
  const items = [], overrides = {};
  let nextN = 0;
  const flat = flatten(pg.objects);
  for (const { o, group } of flat) {
    const rec = { object: o.id, type: o.type, name: o.name ?? "", notes: [], cls: "exact", out: null, _area: 0 };
    records.push(rec);
    const note = (c) => { if (!rec.notes.includes(c)) rec.notes.push(c); };
    const cannot = (c) => { note(c); rec.cls = "cannot"; };
    if (group) { rec.out = "flattened"; note("group_flattened"); continue; }
    if (o.visible === false) { cannot("invisible"); continue; }
    const sx = o.scaleX ?? 1, sy = o.scaleY ?? 1;
    const w0 = o.width * sx, h0 = o.height * sy;
    if (sx !== sy) note("nonuniform_scale");
    if (o.skewX || o.skewY) note("skew_dropped");
    if (o.flipX || o.flipY) note("flip_dropped");
    if ((o.opacity ?? 1) < 1) note("opacity_dropped");
    if (o.shadow) note("shadow_dropped");
    if (o.effects) note(o.effects.outline ? "effect_outline_dropped" : "effect_shadow_dropped");
    // centre of the object on the canvas (origin left/top or centre), rotating about the origin as Fabric does
    const a = ((o.angle ?? 0) * Math.PI) / 180, cos = Math.cos(a), sin = Math.sin(a);
    const [hx, hy] = [o.originX === "center" ? 0 : w0 / 2, o.originY === "center" ? 0 : h0 / 2];
    const ccx = o.left + hx * cos - hy * sin, ccy = o.top + hx * sin + hy * cos;
    let w = w0 * s, h = h0 * s; // thousandths of the page width
    let cxm = ox + ccx * s, cym = oy + ccy * s; // centre in thousandths of the page width (both axes)
    let r = ((((o.angle ?? 0) % 360) + 540) % 360) - 180;
    const finish = (item, extra = {}) => {
      // fit the box into what the validator allows; wholly off the page = dropped
      const x0 = cxm - w / 2, y0 = cym - h / 2;
      if (x0 + w <= 0 || x0 >= 1000 || y0 + h <= 0 || y0 >= 1000 * ratio) { cannot("off_page"); return null; }
      if (w < BOX_MIN || h < BOX_MIN || w > BOX_MAX || h > BOX_MAX) { note("box_clamped"); w = clamp(w, BOX_MIN, BOX_MAX); h = clamp(h, BOX_MIN, BOX_MAX); }
      let x = x0, cy = (cym / ratio);
      if (x < -w / 2 || x > 1000 - w / 2) { note("box_clamped"); x = clamp(x, -w / 2, 1000 - w / 2); }
      const lim = (h * 1.419) / 2; // validator bound: -h*k/2 <= cy <= 1000 + h*k/2 (page-height thousandths, k = A5 ratio)
      if (cy < -lim || cy > 1000 + lim) { note("box_clamped"); cy = clamp(cy, -lim, 1000 + lim); }
      const it = { ...item, x: r1(x), w: r1(w), h: r1(h), cy: r1(cy), ...(r1(r) ? { r: r1(r) } : {}), ...extra };
      rec._area = w * h;
      return it;
    };
    if (o.type === "textbox") {
      let text = String(o.text ?? "").normalize("NFC").replace(/\r\n?/g, "\n").replace(/[^\S\n]+$/gm, "").trim();
      const clean = text.replace(BAD_CHARS, "");
      if (clean !== text) { text = clean; note("control_chars_removed"); }
      if (!text) { cannot("empty_text"); continue; }
      const cps = [...text]; if (cps.length > TEXT_MAX) { text = cps.slice(0, TEXT_MAX).join(""); note("text_truncated"); }
      const fam = /^"?([^",]+)"?/.exec(o.fontFamily ?? "")?.[1] ?? "";
      const fp = plainFill(o.fill); const color = fp ? nearest(fp.rgb) : "ink";
      if (fp) note("color_snap"); if (fp?.gradient) note("gradient_fill_averaged");
      note("font_map");
      let size = o.fontSize * sy * s * px; // design px
      const sz = clamp(r1(size), FONT_MIN, FONT_MAX); if (sz !== r1(size)) note("size_clamped");
      if (o.fontStyle === "italic") note("italic_dropped");
      if (o.charSpacing) note("letter_spacing_dropped");
      if (o.lineHeight && Math.abs(o.lineHeight - 1.2) > 0.05) note("line_height_dropped");
      if (o.underline || o.linethrough || o.overline) note("decoration_dropped");
      if (o.styles?.length) note("run_styles_dropped");
      if (o.textBackgroundColor) note("text_background_dropped");
      if (o.stroke && o.strokeWidth) note("text_outline_dropped");
      if (o.pathSide && o.path) note("text_on_path_dropped");
      const align = ["left", "center", "right"].includes(o.textAlign) ? o.textAlign : "left";
      const it = finish({ id: "", t: "text", text, font: DISPLAY.has(fam) ? "display" : "body", size: sz, color, align, ...((o.fontWeight ?? 400) >= 600 ? { bold: true } : {}) });
      if (it) { rec.out = "text"; items.push({ it, rec, n: nextN++ }); }
      continue;
    }
    if (o.type === "rect" || o.type === "ellipse") {
      const sh = (stretch ? PROPOSED : SHAPES)[o.type];
      const fp = plainFill(o.fill);
      if (!fp) { cannot(o.stroke && o.strokeWidth ? "outline_only_shape" : "no_fill"); continue; }
      if (!stretch && Math.max(w, h) / Math.min(w, h) > SQUARE_TOLERANCE) { cannot("non_square_shape"); continue; }
      note("color_snap"); if (fp.gradient) note("gradient_fill_averaged");
      if (o.stroke && o.strokeWidth) note("stroke_dropped");
      note("shape_substituted"); // fixed corner radius / the registry's shape
      w /= sh.cover; h /= sh.cover; // the asset draws its shape inside the viewBox; enlarge the box so the shape covers the original rectangle
      const it = finish({ id: "", t: "asset", asset: sh.asset, tint: nearest(fp.rgb) });
      if (it) { rec.out = "asset"; items.push({ it, rec, n: nextN++ }); }
      continue;
    }
    if (o.type === "doryline") {
      const sw = Math.max(o.strokeWidth ?? 1, 1);
      const fp = plainFill(o.stroke);
      if (!fp) { cannot("no_stroke"); continue; }
      if (o.width > sw * 2 && o.height > sw * 2) { cannot("diagonal_line"); continue; }
      if (!stretch) { cannot("non_square_shape"); continue; } // a line is the extreme non-square box
      note("color_snap"); note("line_as_thin_box"); if (o.arrowStart || o.arrowEnd) note("arrowhead_dropped"); if (o.strokeDashArray) note("dash_dropped");
      const horizontal = o.width >= o.height;
      const len = (horizontal ? o.width : o.height) * s, thick = Math.max(sw * s, BOX_MIN);
      if (thick > sw * s) note("box_clamped");
      w = (horizontal ? len : thick) / PROPOSED.rect.cover; h = (horizontal ? thick : len) / PROPOSED.rect.cover;
      cxm = ox + (o.left + (horizontal ? o.width / 2 : 0)) * s; cym = oy + (o.top + (horizontal ? 0 : o.height / 2)) * s;
      const it = finish({ id: "", t: "asset", asset: PROPOSED.rect.asset, tint: nearest(fp.rgb) });
      if (it) { rec.out = "asset"; items.push({ it, rec, n: nextN++ }); }
      continue;
    }
    if (o.type === "image") {
      if (o.qr) { cannot("qr_code"); continue; }
      const sl = o.slot;
      if (!sl) { cannot("image_without_slot"); continue; }
      // a photo slot becomes the position of the template's own photo (cover.photo / profile.photo, an `overrides` entry); the other kinds have none
      const target = kind === "cover" || kind === "profile" ? `${kind}.photo` : null;
      if (!target) { cannot("no_photo_element_on_kind"); continue; }
      note("photo_slot_mapped"); if (sl.mask) note("mask_dropped"); if (o.angle) note("rotation_kept");
      cxm = ox + sl.cx * s; cym = oy + sl.cy * s; w = sl.width * s; h = sl.height * s; r = ((((sl.angle ?? 0) % 360) + 540) % 360) - 180;
      const it = finish({}, {});
      if (it) { const { id, t, ...box } = it; overrides[target] = { ...box, hidden: false }; rec.out = "override:" + target; rec.cls = "approx"; }
      continue;
    }
    if (o.type === "path") { cannot("vector_path"); continue; }
    cannot("unknown_type");
  }
  // item cap: drop the smallest shapes first, then the smallest texts (deterministic)
  const kept = items;
  if (kept.length > ITEMS_MAX) {
    const order = [...kept].sort((a, b) => (a.it.t === "text") - (b.it.t === "text") || a.rec._area - b.rec._area || a.n - b.n);
    const drop = new Set(order.slice(0, kept.length - ITEMS_MAX).map((e) => e.n));
    for (const e of kept) if (drop.has(e.n)) { e.rec.cls = "cannot"; e.rec.out = null; if (!e.rec.notes.includes("over_item_cap")) e.rec.notes.push("over_item_cap"); }
    items.splice(0, items.length, ...kept.filter((e) => !drop.has(e.n)));
  }
  const outItems = items.map(({ it, rec }) => ({ ...it, id: "x" + id12(slug, pageIndex, rec.object) }));
  for (const rec of records) {
    if (rec.cls === "exact" && rec.notes.some((n) => !["color_snap", "font_map", "group_flattened"].includes(n))) rec.cls = "approx";
    rec.palette_cls = rec.cls; // exact once colour and font snapping to the template's tokens are accepted
    rec.strict_cls = rec.cls === "cannot" ? "cannot" : rec.notes.some((n) => n !== "group_flattened") ? "approx" : "exact";
    delete rec._area;
  }
  const bgc = plainFill(pg.background?.color ?? (pg.background?.type === "gradient" ? pg.background : null));
  const page = { bg: { color: bgc ? nearest(bgc.rgb) : "paper" }, ...(Object.keys(overrides).length ? { overrides } : {}), items: outItems };
  return { page, report: { slug, page: pageIndex, canvas: [cw, ch], mode, scale: Math.round(s * 1000) / 1000, bg: pg.background?.type, bg_notes: pg.background?.type === "gradient" ? ["gradient_bg_averaged"] : [], objects: records } };
}

/** Canonical JSON: keys sorted, compact. Same input, same bytes. */
export function canonical(v) {
  const sort = (x) => Array.isArray(x) ? x.map(sort) : x && typeof x === "object" ? Object.fromEntries(Object.keys(x).sort().map((k) => [k, sort(x[k])])) : x;
  return JSON.stringify(sort(v));
}
