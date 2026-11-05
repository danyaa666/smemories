// A small static renderer of one layout page, following the product's draw rule (web/src/book/layout/geometry.ts `place`, runtime.tsx): the page is
// 816 design px wide, drawn on paper with CSS zoom; a text is a pre-wrap div with line-height 1.25 that shrinks to 50 % then clips (this prototype's
// own fit loop, not the product's FitText); an asset is the registry SVG in a box with the default preserveAspectRatio. Not the product's code: the
// product's own print-check measures the real runtime (docs/templates.md). Used for the before/after renders and the print-parity numbers.
import { readFileSync } from "node:fs";
import { DESIGN_W, PAPERS, TOKENS } from "./convert.mjs";

const svgDir = new URL("../../../../web/src/book/assets/svg/", import.meta.url);
const MM_TO_PX = 96 / 25.4;
const designH = (paper) => Math.round((DESIGN_W * PAPERS[paper][1]) / PAPERS[paper][0]);
const zoom = (paper) => (PAPERS[paper][0] * MM_TO_PX) / DESIGN_W;
const inside = (pos, len, max) => (len >= max ? (max - len) / 2 : Math.min(Math.max(pos, 0), max - len));
/** geometry.ts place(): left = x*W, top = cy*H - h/2, kept inside the page. */
export function place(b, paper) {
  const W = DESIGN_W, H = designH(paper), w = (b.w * W) / 1000, h = (b.h * W) / 1000;
  return { left: inside((b.x * W) / 1000, w, W), top: inside((b.cy * H) / 1000 - h / 2, h, H), width: w, height: h };
}
const esc = (s) => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
const PROPOSED = {
  "shape.rect": '<svg viewBox="0 0 100 100" preserveAspectRatio="none"><rect width="100" height="100" fill="currentColor"/></svg>',
  "shape.ellipse": '<svg viewBox="0 0 100 100" preserveAspectRatio="none"><ellipse cx="50" cy="50" rx="50" ry="50" fill="currentColor"/></svg>',
};
const svgOf = (id) => PROPOSED[id] ?? readFileSync(new URL(`${id}.svg`, svgDir), "utf8");

export const THEME = `--mb-ink:${TOKENS.ink};--mb-soft:${TOKENS.soft};--mb-paper:${TOKENS.paper};--mb-white:${TOKENS.white};--mb-a1:${TOKENS.accent};--mb-body:Nunito,sans-serif;--mb-disp:"Baloo 2",Nunito,sans-serif`;
const COLOR = { ink: "var(--mb-ink)", soft: "var(--mb-soft)", paper: "var(--mb-paper)", white: "var(--mb-white)", accent: "var(--mb-a1)" };
const FONT = { display: "var(--mb-disp)", body: "var(--mb-body)" };

/** The HTML of one sheet. Photo overrides are drawn as a grey stand-in for the template's own photo element. */
export function sheetHtml(page, paper, { id = "sheet", fontsUrl = "/web/src/book/fonts/fonts.css" } = {}) {
  const H = designH(paper);
  const box = (b) => { const r = place(b, paper); return `left:${r.left}px;top:${r.top}px;width:${r.width}px;height:${r.height}px;${b.r ? `transform:rotate(${b.r}deg);` : ""}`; };
  const parts = [];
  for (const [k, o] of Object.entries(page.overrides ?? {})) if (!o.hidden) parts.push(`<div class="it ph" data-photo="${k}" style="${box(o)}"></div>`);
  for (const it of page.items) {
    let body = "";
    if (it.t === "text") body = `<div class="tx" style="font-size:${it.size}px;font-family:${FONT[it.font] ?? FONT.body};font-weight:${it.bold ? 700 : 400};color:${COLOR[it.color] ?? COLOR.ink};text-align:${it.align ?? "left"}">${esc(it.text)}</div>`;
    else if (it.t === "asset") body = `<div class="as" style="color:${COLOR[it.tint] ?? "inherit"}">${svgOf(it.asset)}</div>`;
    parts.push(`<div class="it" data-item="${it.id}" data-t="${it.t}" style="${box(it)}">${body}</div>`);
  }
  const bg = page.bg?.color ? COLOR[page.bg.color] : COLOR.paper;
  return `<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="${fontsUrl}">
<style>@page{size:${PAPERS[paper][0]}mm ${PAPERS[paper][1]}mm;margin:0}
html,body{margin:0;background:#fff}
#${id}{${THEME};width:${PAPERS[paper][0]}mm;height:${PAPERS[paper][1]}mm;overflow:hidden;position:relative}
#${id} .pg{zoom:${zoom(paper)};position:absolute;left:0;top:0;width:${DESIGN_W}px;height:${H}px;background:${bg};overflow:hidden}
.it{position:absolute;box-sizing:border-box} .ph{background:#cfd3da}
.tx{width:100%;height:100%;line-height:1.25;white-space:pre-wrap;overflow-wrap:anywhere;overflow:hidden}
.as{width:100%;height:100%} .as svg{width:100%;height:100%;display:block}</style>
<div id="${id}"><div class="pg">${parts.join("")}</div></div>
<script>document.fonts.ready.then(()=>{for(const el of document.querySelectorAll(".tx")){const f0=parseFloat(el.style.fontSize);let f=1;while(el.scrollHeight>el.clientHeight+1&&f>0.5){f-=0.02;el.style.fontSize=f0*f+"px"}}document.body.dataset.ready="1"})</script>`;
}
