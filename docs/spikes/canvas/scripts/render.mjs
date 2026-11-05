// Question 1: before/after renders of the 3 best and 3 worst templates (by converted share, with the proposed shapes) on A5, A4 and Letter.
//   node scripts/render.mjs -> out/renders/<slug>.png (git-ignored: the pictures are derived from Doory's starter templates, see ADR 0005 licence section), results/selection.json
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { chromium } from "playwright";
import { PAPERS, canonical, convertPage } from "../src/convert.mjs";
import { sheetHtml } from "../src/render.mjs";
import { serve } from "../src/serve.mjs";

const cov = JSON.parse(readFileSync(new URL("../results/coverage.json", import.meta.url), "utf8"));
const rank = Object.entries(cov.templates).map(([slug, t]) => ({ slug, objects: t["fit+stretch"].objects, pct: t["fit+stretch"].converted_pct, exact: t["fit+stretch"].exact_pct }))
  .sort((a, b) => b.pct - a.pct || b.objects - a.objects || a.slug.localeCompare(b.slug));
const selection = { best: rank.slice(0, 3), worst: rank.slice(-3).reverse() };
writeFileSync(new URL("../results/selection.json", import.meta.url), JSON.stringify(selection, null, 1) + "\n");
mkdirSync(new URL("../out/renders/", import.meta.url), { recursive: true });
const srv = await serve();
const browser = await chromium.launch();
const shot = async (page, html, paper) => {
  const f = `docs/spikes/canvas/out/renders/tmp-${paper}.html`;
  writeFileSync(new URL(`../out/renders/tmp-${paper}.html`, import.meta.url), html);
  await page.setViewportSize({ width: 900, height: 1300 });
  await page.goto(`${srv.base}/${f}`); await page.waitForSelector("body[data-ready]");
  return (await page.locator("#sheet").screenshot()).toString("base64");
};
const page = await browser.newPage({ deviceScaleFactor: 1 });
for (const slug of [...selection.best, ...selection.worst].map((x) => x.slug)) {
  const design = JSON.parse(readFileSync(new URL(`../doory/backend/data/templates/${slug}.json`, import.meta.url), "utf8"));
  const panels = [{ label: "Doory (its own thumbnail)", src: `${srv.base}/docs/spikes/canvas/doory/backend/data/templates/thumbs/${slug}.webp` }];
  for (const [label, stretch, paper] of [["A5, today's shapes", false, "A5"], ["A5, proposed shapes", true, "A5"], ["A4, proposed shapes", true, "A4"], ["Letter, proposed shapes", true, "Letter"]]) {
    const { page: p } = convertPage(design, 0, { slug, kind: "cover", stretch });
    panels.push({ label, src: "data:image/png;base64," + (await shot(page, sheetHtml(p, paper), paper)) });
  }
  const html = `<style>body{margin:8px;font:12px sans-serif;background:#eee}div{display:flex;gap:10px;align-items:flex-start}figure{margin:0}img{height:560px;border:1px solid #999;background:#fff}</style><h3>${slug}</h3><div>${panels.map((q) => `<figure><img src="${q.src}"><figcaption>${q.label}</figcaption></figure>`).join("")}</div>`;
  writeFileSync(new URL(`../out/renders/${slug}.html`, import.meta.url), html);
  const sheet = await browser.newPage({ viewport: { width: 2000, height: 640 } });
  await sheet.goto(`${srv.base}/docs/spikes/canvas/out/renders/${slug}.html`); await sheet.waitForLoadState("load"); await sheet.waitForTimeout(300);
  writeFileSync(new URL(`../out/renders/${slug}.png`, import.meta.url), await sheet.screenshot({ fullPage: true })); await sheet.close();
  console.log("rendered", slug);
}
await browser.close(); srv.close();
console.log(JSON.stringify(selection));
