// Starts the prototype's dev server on SPIKE_PORT (default 5391) and runs every measurement; results go to results/*.json.
//   npm run measure   (needs `npm ci` here, `(cd ../../../web && npm ci)` and poppler for the parity step)
import { spawn } from "node:child_process";
import { createServer } from "vite";

const server = await createServer({ configFile: new URL("../vite.config.mjs", import.meta.url).pathname, logLevel: "error" });
await server.listen();
const base = server.resolvedUrls.local[0].replace(/\/$/, "");
const steps = [["units.mjs"], ["grid.mjs"], ["size.mjs"], ["interact.mjs"], ["text.mjs"], ["--expose-gc", "undo.mjs"], ["parity.mjs"], ["phone.mjs"]];
let failed = false;
for (const s of steps) {
  const file = s.at(-1);
  console.log(`\n== ${file}`);
  // async: the dev server lives in this process, so it must keep running while the step is measuring
  const status = await new Promise((done) => spawn("node", [...s.slice(0, -1), new URL(`./${file}`, import.meta.url).pathname], { stdio: "inherit", env: { ...process.env, SPIKE_URL: base } }).on("close", done));
  if (status !== 0) { failed = true; console.log(`${file} failed (${status})`); }
}
await server.close();
process.exit(failed ? 1 : 0);
