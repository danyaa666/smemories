// Locale parity check: every key exists in both locales and no value is empty.
// Run as `npm run lint:i18n`; exits non-zero and names each offending key.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

function flatten(obj, prefix = "", out = {}) {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (v !== null && typeof v === "object") flatten(v, key, out);
    else out[key] = v;
  }
  return out;
}

/** Returns a list of human-readable problems (empty when the locales agree). */
export function compareLocales(a, b, nameA = "en", nameB = "vi") {
  const problems = [];
  const fa = flatten(a);
  const fb = flatten(b);
  for (const [name, flat, other, otherName] of [
    [nameA, fa, fb, nameB],
    [nameB, fb, fa, nameA],
  ]) {
    for (const [key, value] of Object.entries(flat)) {
      if (!(key in other)) problems.push(`${key}: missing in ${otherName}.json`);
      if (typeof value !== "string" || value.trim() === "") {
        problems.push(`${key}: empty or non-string value in ${name}.json`);
      }
    }
  }
  return problems;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const load = (n) =>
    JSON.parse(readFileSync(new URL(`../src/locales/${n}.json`, import.meta.url)));
  const problems = compareLocales(load("en"), load("vi"));
  if (problems.length > 0) {
    console.error(`i18n check failed:\n${problems.map((p) => `  ${p}`).join("\n")}`);
    process.exit(1);
  }
  console.log("i18n check ok");
}
