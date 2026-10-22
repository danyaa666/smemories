// Handwritten signatures (E11, decision L-14, T-095 validator in internal/signature). A signature is a list of strokes, each a flat
// list of integers [x0, y0, x1, y1, ...] in a 1000 x 400 space. Only positions are kept: no timing, pressure or pointer data.
// This module is the single place that thins a drawing, serialises it (the T-095 canonical form) and turns a stored value into an
// SVG path; the path is built from validated integers only, never from markup.

export const W = 1000;
export const H = 400;
/** Canonical JSON size cap; it usually binds before the 600-point cap. */
export const MAX_BYTES = 3072;
export const MIN_BOX_PCT = 5;
const MIN_DIST = 2; // units between two stored points
const EPS_FROM = 1.5; // Ramer-Douglas-Peucker tolerance, widened in steps while the drawing is over a limit
const EPS_TO = 4;
const EPS_STEP = 0.5;

export type Stroke = number[];
export type Limits = { maxStrokes: number; maxPoints: number };
export type Signature = { v: 1; w: 1000; h: 400; s: Stroke[] };

/** The canonical JSON the server stores: key order v, w, h, s and no spaces. The server re-serialises, so this is what it compares. */
export const canonical = (s: Stroke[]): string => JSON.stringify({ v: 1, w: W, h: H, s });

/** A client position to the drawing space: rounded to an integer and kept inside it. */
export const quantise = (client: number, origin: number, size: number, space: number): number =>
  Math.min(space, Math.max(0, Math.round(((client - origin) / size) * space)));

/** Adds a point to a stroke unless it is closer than MIN_DIST to the last one. Returns true when it was added. */
export function addPoint(stroke: Stroke, x: number, y: number): boolean {
  const n = stroke.length;
  if (n >= 2 && Math.hypot(x - stroke[n - 2]!, y - stroke[n - 1]!) < MIN_DIST) return false;
  stroke.push(x, y);
  return true;
}

function distToSegment(
  px: number,
  py: number,
  ax: number,
  ay: number,
  bx: number,
  by: number,
): number {
  const dx = bx - ax;
  const dy = by - ay;
  const len2 = dx * dx + dy * dy;
  if (len2 === 0) return Math.hypot(px - ax, py - ay);
  const t = Math.max(0, Math.min(1, ((px - ax) * dx + (py - ay) * dy) / len2));
  return Math.hypot(px - (ax + t * dx), py - (ay + t * dy));
}

/** Ramer-Douglas-Peucker on a flat stroke (iterative). Keeps the ends; a one-point stroke (a tap) becomes a two-point dot. */
export function simplify(stroke: Stroke, eps: number): Stroke {
  const n = stroke.length / 2;
  if (n < 2) return stroke.length >= 2 ? [stroke[0]!, stroke[1]!, stroke[0]!, stroke[1]!] : [];
  const keep = new Uint8Array(n);
  keep[0] = keep[n - 1] = 1;
  const todo: [number, number][] = [[0, n - 1]];
  for (let job = todo.pop(); job; job = todo.pop()) {
    const [a, b] = job;
    let far = -1;
    let max = eps;
    for (let i = a + 1; i < b; i++) {
      const d = distToSegment(
        stroke[2 * i]!,
        stroke[2 * i + 1]!,
        stroke[2 * a]!,
        stroke[2 * a + 1]!,
        stroke[2 * b]!,
        stroke[2 * b + 1]!,
      );
      if (d > max) {
        max = d;
        far = i;
      }
    }
    if (far > 0) {
      keep[far] = 1;
      todo.push([a, far], [far, b]);
    }
  }
  const out: Stroke = [];
  keep.forEach((k, i) => k && out.push(stroke[2 * i]!, stroke[2 * i + 1]!));
  return out;
}

/** True when the strokes are within the stroke, point and canonical-byte limits. */
export function within(strokes: Stroke[], lim: Limits): boolean {
  const points = strokes.reduce((n, s) => n + s.length / 2, 0);
  return (
    strokes.length <= lim.maxStrokes &&
    points <= lim.maxPoints &&
    canonical(strokes).length <= MAX_BYTES // ASCII only, so length is the byte size
  );
}

/**
 * Thins the raw strokes (already MIN_DIST apart) with RDP at 1.5, then at larger tolerances (step 0.5, at most 4.0) until the
 * drawing fits every limit. Always starts from the raw points, so undoing a stroke gets the finer drawing back.
 * Returns null when even the coarsest tolerance does not fit: the caller refuses the stroke.
 */
export function fit(raw: Stroke[], lim: Limits): Stroke[] | null {
  for (let eps = EPS_FROM; eps <= EPS_TO; eps += EPS_STEP) {
    const strokes = raw.map((s) => simplify(s, eps));
    if (within(strokes, lim)) return strokes;
  }
  return null;
}

const isInt = (v: unknown, max: number): v is number =>
  typeof v === "number" && Number.isInteger(v) && v >= 0 && v <= max;

/**
 * A stored or received value checked against every rule of format v1 (the same rules as the Go validator): exactly the keys
 * v, w, h and s, integers only inside the drawing space, 1 to 12 strokes of at least 2 points, 600 points, 3072 canonical
 * bytes and a bounding box of at least 5 % of the width or height. Anything else is null.
 */
export function parseSignature(value: unknown): Signature | null {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return null;
  const o = value as Record<string, unknown>;
  const keys = Object.keys(o);
  if (keys.length !== 4 || !["v", "w", "h", "s"].every((k) => keys.includes(k))) return null;
  if (o.v !== 1 || o.w !== W || o.h !== H) return null;
  const s = o.s;
  if (!Array.isArray(s) || s.length < 1 || s.length > 12) return null;
  let points = 0;
  let minX = W;
  let maxX = 0;
  let minY = H;
  let maxY = 0;
  for (const st of s as unknown[]) {
    if (!Array.isArray(st) || st.length % 2 !== 0 || st.length < 4) return null;
    points += st.length / 2;
    if (points > 600) return null;
    for (let i = 0; i < st.length; i += 2) {
      const x: unknown = st[i];
      const y: unknown = st[i + 1];
      if (!isInt(x, W) || !isInt(y, H)) return null;
      minX = Math.min(minX, x);
      maxX = Math.max(maxX, x);
      minY = Math.min(minY, y);
      maxY = Math.max(maxY, y);
    }
  }
  if ((maxX - minX) * 100 < MIN_BOX_PCT * W && (maxY - minY) * 100 < MIN_BOX_PCT * H) return null;
  const strokes = s as Stroke[];
  return canonical(strokes).length > MAX_BYTES ? null : { v: 1, w: W, h: H, s: strokes };
}

/** The SVG path `d` of a stored value: "M x y L x y ..." per stroke, built only from validated integers. null when invalid. */
export function signaturePath(value: unknown): string | null {
  const sig = parseSignature(value);
  if (!sig) return null;
  return sig.s
    .map((st) => {
      let d = `M${st[0]} ${st[1]}`;
      for (let i = 2; i < st.length; i += 2) d += `L${st[i]} ${st[i + 1]}`;
      return d;
    })
    .join("");
}
