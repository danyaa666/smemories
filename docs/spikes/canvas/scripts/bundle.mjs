// Question 4/7: the studio chunk's cost and proof that it is absent from the main and print bundles.
//   node scripts/bundle.mjs -> results/bundle.json (runs `vite build` for studio/)
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { gzipSync } from "node:zlib";

execFileSync("npx", ["vite", "build", "--config", "studio/vite.config.mjs", "--logLevel", "error"], { stdio: "inherit" });
const dist = new URL("../studio/dist/", import.meta.url);
const man = JSON.parse(readFileSync(new URL(".vite/manifest.json", dist), "utf8"));
const size = (f) => { const b = readFileSync(new URL(f, dist)); return { bytes: b.length, gzip: gzipSync(b, { level: 9 }).length, text: b.toString("utf8") }; };
const closure = (key, seen = new Set()) => { if (seen.has(key)) return seen; seen.add(key); for (const i of man[key].imports ?? []) closure(i, seen); return seen; }; // static imports only: what loads with the page
const studioKey = Object.keys(man).find((k) => /studio\.jsx$/.test(k));
const out = { fabric_version: JSON.parse(readFileSync(new URL("../node_modules/fabric/package.json", import.meta.url), "utf8")).version, studio_chunk: { file: man[studioKey].file, ...(({ bytes, gzip }) => ({ bytes, gzip }))(size(man[studioKey].file)) }, entries: {} };
for (const [name, entry] of [["main (editor/book route)", "index.html"], ["print route", "print.html"]]) {
  const files = [...closure(entry)].map((k) => man[k].file);
  const parts = files.map((f) => ({ f, ...size(f) }));
  out.entries[name] = {
    static_chunks: files, gzip_total: parts.reduce((a, p) => a + p.gzip, 0), bytes_total: parts.reduce((a, p) => a + p.bytes, 0),
    contains_studio_chunk: files.includes(man[studioKey].file),
    mentions_fabric: parts.some((p) => /fabric/i.test(p.text)),
    loads_studio_on_start: (man[entry].dynamicImports ?? []).length > 0 ? "dynamic import only (not fetched until the button)" : "no reference",
  };
}
writeFileSync(new URL("../results/bundle.json", import.meta.url), JSON.stringify(out, null, 1) + "\n");
console.log(JSON.stringify(out, null, 1));
if (Object.values(out.entries).some((e) => e.contains_studio_chunk || e.mentions_fabric)) process.exit(1);
