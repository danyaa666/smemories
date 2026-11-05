// Question 1 and 2: converts every page of the 10 starter templates, writes results/coverage.json, results/objects.json and out/layouts/*.json
//   node scripts/coverage.mjs
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { PAPERS, REF_RATIO, canonical, convertPage } from "../src/convert.mjs";

const dir = new URL("../doory/backend/data/templates/", import.meta.url);
const outDir = new URL("../out/layouts/", import.meta.url);
mkdirSync(outDir, { recursive: true });
const slugs = readdirSync(dir).filter((f) => f.endsWith(".json") && f !== "manifest.json").map((f) => f.slice(0, -5)).sort();
export const KINDS = ["cover", "profile", "notes", "back"];

const summary = { doory_commit: readFileSync(new URL("../doory/.git/HEAD", import.meta.url), "utf8").trim(), templates: {}, totals: {}, by_mode: {} };
const objects = {};
const tally = () => ({ objects: 0, exact: 0, approx: 0, cannot: 0, strict_exact: 0 });
for (const variant of ["fit", "cover", "fit+stretch"]) {
  const mode = variant.split("+")[0], stretch = variant.endsWith("+stretch");
  const total = tally();
  for (const slug of slugs) {
    const design = JSON.parse(readFileSync(new URL(`${slug}.json`, dir), "utf8"));
    const t = (summary.templates[slug] ??= { canvas: [design.width, design.height], pages: design.pages.length });
    const m = (t[variant] = tally());
    t.notes = t.notes ?? {};
    design.pages.forEach((_, i) => {
      for (const kind of KINDS) {
        const { page, report } = convertPage(design, i, { slug, kind, mode, stretch });
        if (variant === "fit") writeFileSync(new URL(`${slug}-p${i}-${kind}.json`, outDir), canonical({ v: 1, pages: { [kind]: page } }) + "\n");
        if (kind !== "cover") continue; // counts are given for the cover (the profile differs only by its photo slot)
        const recs = report.objects.filter((o) => o.type !== "group");
        for (const o of recs) {
          for (const x of [m, total]) { x.objects++; x[o.cls]++; if (o.strict_cls === "exact") x.strict_exact++; }
          for (const n of o.notes) t.notes[n] = (t.notes[n] ?? 0) + (variant === "fit" ? 1 : 0);
        }
        if (variant === "fit") (objects[slug] ??= []).push({ page: i, canvas_scale: report.scale, bg: report.bg, objects: recs.map(({ object, type, name, cls, notes, out }) => ({ object, type, name, cls, out, notes })) });
        else if (variant === "cover") (t.cover_pages ??= []).push({ page: i, scale: report.scale });
      }
    });
    const pct = (x) => Math.round((100 * (x.exact + x.approx)) / x.objects);
    m.converted_pct = pct(m); m.exact_pct = Math.round((100 * m.exact) / m.objects);
  }
  total.converted_pct = Math.round((100 * (total.exact + total.approx)) / total.objects);
  total.exact_pct = Math.round((100 * total.exact) / total.objects);
  summary.by_mode[variant] = total;
}
summary.totals = summary.by_mode.fit;
mkdirSync(new URL("../results/", import.meta.url), { recursive: true });
writeFileSync(new URL("../results/coverage.json", import.meta.url), JSON.stringify(summary, null, 1) + "\n");
writeFileSync(new URL("../results/objects.json", import.meta.url), JSON.stringify(objects, null, 1) + "\n");
for (const [v, t] of Object.entries(summary.by_mode)) console.log(v.padEnd(12), JSON.stringify(t));
for (const [s, t] of Object.entries(summary.templates)) console.log(s.padEnd(34), JSON.stringify(t.fit));
