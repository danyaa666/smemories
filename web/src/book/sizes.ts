// Page sizes in millimetres. A template is drawn in a box DESIGN_W px wide (the canvas designs are US Letter at 96 dpi);
// its height follows the page ratio, and the box is scaled to the paper with CSS zoom (ADR 0003: zoom keeps the PDF vector).
export const SIZES = {
  A5: { w: 148, h: 210 },
  A4: { w: 210, h: 297 },
  Letter: { w: 215.9, h: 279.4 },
} as const;
export type PageSize = keyof typeof SIZES;

export const DESIGN_W = 816;
export const designHeight = (s: PageSize) => Math.round((DESIGN_W * SIZES[s].h) / SIZES[s].w);

const MM_TO_PX = 96 / 25.4;
/** CSS px on screen at scale 1. */
export const pagePx = (s: PageSize) => ({ w: SIZES[s].w * MM_TO_PX, h: SIZES[s].h * MM_TO_PX });
/** Design px -> CSS px on paper. */
export const designZoom = (s: PageSize) => (SIZES[s].w * MM_TO_PX) / DESIGN_W;

/** The one @page rule of the document: every printed page has this size and no margin. */
export const pageCss = (s: PageSize) =>
  `@page { size: ${SIZES[s].w}mm ${SIZES[s].h}mm; margin: 0 }`;
