// Question 2: the converter is a pure function. Converts everything in two separate processes (different TZ, LANG and working directory) and compares sha256.
//   node scripts/determinism.mjs -> results/determinism.json
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import os from "node:os";

const child = `
import { readdirSync, readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { canonical, convertPage } from ${JSON.stringify(new URL("../src/convert.mjs", import.meta.url).href)};
const dir = new URL(${JSON.stringify(new URL("../doory/backend/data/templates/", import.meta.url).href)});
const h = createHash("sha256"); let n = 0, bytes = 0;
for (const f of readdirSync(dir).filter((f) => f.endsWith(".json") && f !== "manifest.json").sort()) {
  const d = JSON.parse(readFileSync(new URL(f, dir), "utf8"));
  d.pages.forEach((_, i) => { for (const kind of ["cover", "profile", "notes", "back"]) for (const stretch of [false, true]) for (const mode of ["fit", "cover"]) {
    const { page, report } = convertPage(d, i, { slug: f.slice(0, -5), kind, mode, stretch }); const s = canonical({ page, report }); h.update(s); n++; bytes += s.length; } });
}
console.log(JSON.stringify({ sha256: h.digest("hex"), conversions: n, bytes }));`;
const run = (env, cwd) => JSON.parse(execFileSync(process.execPath, ["--input-type=module", "-e", child], { env: { ...process.env, ...env }, cwd, encoding: "utf8" }));
const a = run({ TZ: "UTC", LANG: "C" }, process.cwd());
const b = run({ TZ: "Pacific/Auckland", LANG: "vi_VN.UTF-8" }, os.tmpdir());
const out = { run_a: a, run_b: b, identical: a.sha256 === b.sha256 };
writeFileSync(new URL("../results/determinism.json", import.meta.url), JSON.stringify(out, null, 1) + "\n");
console.log(JSON.stringify(out));
if (!out.identical) process.exit(1);
