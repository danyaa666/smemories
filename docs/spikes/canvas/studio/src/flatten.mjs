// Flatten rules (ADR 0005 question 4): the studio's picture leaves as one raster. Pure sizing, so it can be unit-checked without a canvas.
export const PAPERS = { A5: 210 / 148, A4: 297 / 210, Letter: 279.4 / 215.9 };
export const MAX_PIXELS = 4_500_000; // memory cap of one flatten: 4.5 MP is 18 MB of RGBA; A5 at 300 dpi (1748 x 2480 = 4.33 MP) fits
export const MAX_EDGE = 3000; // the server keeps at most 3000 px on the long edge (docs/media.md), so more is wasted
/** Size of the flattened picture for a page-shaped canvas of `paper`: 300 dpi on the paper's width when it fits the caps, else the largest that does. */
export function flattenSize(paper, widthMm) {
  const ratio = PAPERS[paper];
  let w = Math.round((widthMm / 25.4) * 300), h = Math.round(w * ratio);
  const k = Math.min(1, Math.sqrt(MAX_PIXELS / (w * h)), MAX_EDGE / Math.max(w, h));
  if (k < 1) { w = Math.floor(w * k); h = Math.floor(w * ratio); }
  return { w, h, dpi: Math.round((w * 25.4) / widthMm) };
}
/** canvas -> Blob with the failure a phone gives (toBlob answers null when it cannot allocate). */
export function toBlob(canvas, type, quality) {
  return new Promise((res, rej) => canvas.toBlob((b) => (b ? res(b) : rej(new Error("export_failed"))), type, quality));
}
