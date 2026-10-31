// Bundle cost of each candidate (minified, gzip) on top of react-dom: node scripts/size.mjs -> results/size.json
import { writeFileSync } from "node:fs";
import { gzipSync } from "node:zlib";
import react from "@vitejs/plugin-react";
import { build } from "vite";

const out = {};
for (const name of ["base", "inhouse", "moveable"]) {
  const res = await build({ configFile: false, logLevel: "silent", plugins: [react()], define: { "process.env.NODE_ENV": JSON.stringify("production") }, build: { write: false, minify: true, lib: { entry: `src/size/${name}.jsx`, formats: ["es"], fileName: name } } });
  const code = [res].flat().flatMap((r) => r.output).filter((o) => o.type === "chunk").map((o) => o.code).join("");
  out[name] = { min_kb: Math.round(code.length / 102.4) / 10, gzip_kb: Math.round(gzipSync(code).length / 102.4) / 10 };
}
for (const k of ["inhouse", "moveable"]) out[`${k}_over_base`] = { min_kb: Math.round((out[k].min_kb - out.base.min_kb) * 10) / 10, gzip_kb: Math.round((out[k].gzip_kb - out.base.gzip_kb) * 10) / 10 };
writeFileSync(new URL("../results/size.json", import.meta.url), JSON.stringify(out, null, 1));
console.log(out);
