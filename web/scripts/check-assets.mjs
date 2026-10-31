// Asset pack check (E09, T-151): every SVG is plain and small, and registry.json lists exactly the files with their licence data.
// Run as `scripts/check-assets.sh [assets dir]` (CI web job); exits non-zero and names each problem.
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const MAX_BYTES = 20 * 1024;
const ELEMENTS = new Set([
  "svg",
  "g",
  "path",
  "circle",
  "ellipse",
  "rect",
  "line",
  "polyline",
  "polygon",
]);
const KINDS = ["sticker", "shape", "frame"];
const LICENCES = ["CC0", "OFL", "authored"];
const SVG_NS = "http://www.w3.org/2000/svg";

/** Returns the problems of one SVG text (empty when it is safe): shapes and paths only, no scripts, links, styles, ids or url() paints. */
export function checkSvg(text, bytes = Buffer.byteLength(text)) {
  const p = [];
  if (bytes > MAX_BYTES) p.push(`${bytes} bytes (max ${MAX_BYTES})`);
  if (/<!(?!--)|<\?(?!xml )|<!\[CDATA\[/i.test(text))
    p.push("doctype, entity, CDATA or processing instruction");
  if (/@import|javascript:|data:|url\s*\(/i.test(text))
    p.push("@import, javascript:, data: or url() reference");
  const tags = [...text.matchAll(/<\/?\s*([A-Za-z][\w:.-]*)([^>]*)>/g)];
  if (tags.length === 0) p.push("no element");
  for (const [, name, rest] of tags) {
    if (!ELEMENTS.has(name)) p.push(`element <${name}> not allowed`);
    for (const [, attr, value] of rest.matchAll(/([^\s=/"']+)\s*=\s*"([^"]*)"/g)) {
      if (/^on/i.test(attr)) p.push(`event attribute ${attr}`);
      else if (/href/i.test(attr)) p.push(`attribute ${attr} not allowed`);
      else if (/^(style|class|id)$/i.test(attr)) p.push(`attribute ${attr} not allowed`);
      else if (/^xmlns/.test(attr) && !(attr === "xmlns" && value === SVG_NS))
        p.push(`attribute ${attr} not allowed`);
    }
    const unquoted = rest.replace(/[^\s=/"']+\s*=\s*"[^"]*"/g, "").replace(/[/\s]/g, "");
    if (unquoted !== "") p.push(`malformed tag <${name}>`);
  }
  const root = text.match(/<svg\b([^>]*)>/);
  const vb = root?.[1]?.match(/\sviewBox="([^"]*)"/);
  if (!vb) p.push("root svg has no viewBox");
  return [...new Set(p)];
}

/** Returns the problems of an assets folder (registry.json, svg/, licences/); empty when everything agrees. */
export function checkAssets(dir) {
  const p = [];
  let reg;
  try {
    reg = JSON.parse(readFileSync(join(dir, "registry.json"), "utf8"));
  } catch (e) {
    return [`registry.json unreadable: ${e.message}`];
  }
  const list = Array.isArray(reg?.assets) ? reg.assets : [];
  if (list.length === 0) p.push("registry.json has no assets");
  const ids = new Set();
  const files = new Set();
  for (const a of list) {
    const at = `${a?.id}`;
    const bad = (m) => p.push(`${at}: ${m}`);
    if (typeof a?.id !== "string" || !/^(sticker|shape|frame)\.[a-z0-9]+(-[a-z0-9]+)*$/.test(a.id))
      bad("id must look like kind.name");
    if (ids.has(a.id)) bad("duplicate id");
    ids.add(a.id);
    if (!KINDS.includes(a.kind) || !String(a.id).startsWith(`${a.kind}.`))
      bad("kind missing or different from the id prefix");
    for (const f of ["label_en", "label_vi"])
      if (typeof a[f] !== "string" || a[f].trim() === "") bad(`${f} missing`);
    if (
      !Array.isArray(a.tags) ||
      a.tags.length === 0 ||
      a.tags.some((t) => typeof t !== "string" || t === "")
    )
      bad("tags must be a non-empty list of strings");
    if (typeof a.tintable !== "boolean") bad("tintable must be true or false");
    if (!LICENCES.includes(a.licence)) bad(`licence must be one of ${LICENCES.join(", ")}`);
    if (typeof a.author !== "string" || a.author === "") bad("author missing");
    if (!/^\d{4}-\d{2}-\d{2}$/.test(a.date_checked ?? "")) bad("date_checked must be YYYY-MM-DD");
    if (
      a.licence === "authored" ? a.source_url !== "" : !/^https:\/\/\S+$/.test(a.source_url ?? "")
    )
      bad("source_url must be an https URL (empty for authored)");
    if (a.file !== `svg/${a.id}.svg`) bad(`file must be svg/${a.id}.svg`);
    files.add(a.file);
    if (
      typeof a.licence_file !== "string" ||
      !/^licences\/[\w.-]+\.txt$/.test(a.licence_file) ||
      !existsSync(join(dir, a.licence_file))
    )
      bad("licence_file missing or not found");
    const path = join(dir, String(a.file));
    if (!existsSync(path)) {
      bad("svg file not found");
      continue;
    }
    const text = readFileSync(path, "utf8");
    for (const m of checkSvg(text, statSync(path).size)) bad(m);
    const vb = text.match(/<svg\b[^>]*\sviewBox="\s*0\s+0\s+(\d+)\s+(\d+)\s*"/);
    if (!vb || Number(vb[1]) !== a.width || Number(vb[2]) !== a.height)
      bad("width and height must equal the viewBox (0 0 width height)");
    if (a.tintable === true && !text.includes("currentColor"))
      bad("tintable but the SVG never uses currentColor");
    if (a.tintable === false && text.includes("currentColor"))
      bad("not tintable but the SVG uses currentColor");
  }
  const svgDir = join(dir, "svg");
  for (const f of existsSync(svgDir) ? readdirSync(svgDir) : [])
    if (!files.has(`svg/${f}`)) p.push(`svg/${f}: not listed in registry.json`);
  return p;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const dir = resolve(
    process.argv[2] ?? join(dirname(fileURLToPath(import.meta.url)), "../src/book/assets"),
  );
  const problems = checkAssets(dir);
  if (problems.length > 0) {
    console.error(`check-assets failed:\n${problems.map((x) => `  ${x}`).join("\n")}`);
    process.exit(1);
  }
  console.log(
    `check-assets ok: ${JSON.parse(readFileSync(join(dir, "registry.json"), "utf8")).assets.length} assets`,
  );
}
