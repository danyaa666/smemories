// Fidelity of a designed template to its canvas (T-059 memory-book, T-060 navy-classic; AC4): opens the canvas pages of design/canvas/temp*.html
// and the same pages of the template (Letter, 816 x 1056 design px at scale 1), writes a side-by-side picture of each, and compares the boxes
// (elements with a background, a border or a ruled-lines gradient) of the canvas with the template's boxes of the same colour: the largest
// difference of an edge, in millimetres.
//   node fidelity.mjs OUT_DIR [memory-book|navy-classic]   (default memory-book; set PRINT_CHECK_PORT if 5199 is taken; dev tool, not part of `make print-check`)
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { gunzipSync } from "node:zlib";
import { chromium } from "playwright";
import { makeBook } from "./fixture.mjs";
import { open, ROOT, serveBook, startServer } from "./lib.mjs";

const out = path.resolve(process.argv[2] ?? "fidelity-out");
mkdirSync(out, { recursive: true });
const MM = 25.4 / 96;
const TOLERANCE_MM = 1.5;

// the canvas pages, unpacked from the bundle (page_order index -> kind)
// per template: the canvas file, its pages as [kind, page_order index, our sheet label], the root element of a canvas page, our page class
const TEMPLATES = {
  "memory-book": { file: "temp1", kinds: [["cover", 0, "cover"], ["profile", 1, "profile"], ["friend", 4, "notes"], ["back", 7, "back"]], root: "div.dots", cls: ".mb-page" },
  // the canvas has no back page: it is derived from the cover's frame, so the cover's boxes are the reference
  "navy-classic": { file: "temp2", kinds: [["cover", 0, "cover"], ["profile", 1, "profile"], ["autographs", 5, "notes"], ["back", 0, "back"]], root: "div.sc-host > div", cls: ".nc-page" },
};
const TEMPLATE = process.argv[3] ?? "memory-book";
const T = TEMPLATES[TEMPLATE];
if (!T) throw new Error(`unknown template ${TEMPLATE}`);
const src = readFileSync(path.join(ROOT, `design/canvas/${T.file}.html`), "utf8");
const block = (n) => JSON.parse(src.match(new RegExp(`<script type="__bundler/${n}">(.*?)</script>`, "s"))[1]);
const manifest = block("manifest");
const order = block("page_order");
const tmp = mkdtempSync(path.join(os.tmpdir(), "canvas-"));
const KINDS = T.kinds;
const canvasFile = (i) => {
  const e = manifest[order[i]];
  const f = path.join(tmp, `page-${i}.html`);
  writeFileSync(f, gunzipSync(Buffer.from(e.data, "base64")));
  return f;
};

/** Boxes of a root element: [{key, l, t, r, b}] in design px relative to the root; key = the colour that makes it a box. */
const BOXES = (root, isCanvas) => {
  const rr = root.getBoundingClientRect();
  const k = rr.width / 816;
  const res = [];
  for (const e of root.querySelectorAll("*")) {
    const cs = getComputedStyle(e);
    const r = e.getBoundingClientRect();
    if (r.width < 24 * k || r.height < 14 * k || e.tagName === "svg" || e.closest("svg")) continue;
    const bg = cs.backgroundColor;
    const solid = bg !== "rgba(0, 0, 0, 0)" && !/rgba\(255, 255, 255, 0\.\d+\)/.test(bg);
    const border = parseFloat(cs.borderTopWidth) > 0 && cs.borderTopStyle !== "none";
    const rules = /repeating-linear-gradient/.test(cs.backgroundImage) && !solid; // ruled lines (navy-classic notes)
    if (!solid && !border && !rules) continue;
    if (r.width >= rr.width - 2 && r.height >= rr.height - 2) continue; // the page itself (a frame inset from its edge is a box)
    res.push({ key: solid ? bg : border ? cs.borderTopColor : "rules", l: (r.left - rr.left) / k, t: (r.top - rr.top) / k, r: (r.right - rr.left) / k, b: (r.bottom - rr.top) / k, cls: e.className?.toString?.() ?? "" });
  }
  return res;
};

const stop = await startServer();
const browser = await chromium.launch();
const rows = [];
try {
  const ours = await browser.newPage({ viewport: { width: 1400, height: 1100 } });
  const book = makeBook({ size: "Letter", notes: 4, template: TEMPLATE });
  book.yearbook.language = "en";
  book.notes[0].photo_ids = []; // the canvas friend page has the dashed "stick your photo here" frame
  await serveBook(ours, book);
  await open(ours);
  await ours.addStyleTag({ content: ".bk-stage{--s:1 !important}" }); // design px = CSS px
  for (const [kind, idx, label] of KINDS) {
    const c = await browser.newPage({ viewport: { width: 1000, height: 1200 } });
    await c.route(/fonts\.(googleapis|gstatic)\.com/, (r) => r.abort());
    await c.goto("file://" + canvasFile(idx));
    await c.waitForSelector(T.root, { timeout: 20000 });
    await c.waitForTimeout(500);
    const root = c.locator(T.root).first();
    const canvasBoxes = await root.evaluate(`(${BOXES.toString()})(this, true)`.replace("this", "arguments[0]")).catch(async () => root.evaluate((el, f) => new Function("return " + f)()(el, true), BOXES.toString()));
    const sheet = ours.locator(`section[data-page="${label}"]`).first();
    const mine = sheet.locator(T.cls);
    const ourBoxes = await mine.evaluate((el, f) => new Function("return " + f)()(el, false), BOXES.toString());
    // side by side
    const a = await root.screenshot({ type: "jpeg", quality: 82 });
    const b = await sheet.screenshot({ type: "jpeg", quality: 82 });
    const both = await browser.newPage({ viewport: { width: 1700, height: 1080 } });
    await both.setContent(`<body style="margin:0;background:#888;display:flex;gap:16px;padding:12px"><figure style="margin:0"><img src="data:image/jpeg;base64,${a.toString("base64")}" width="816"><figcaption style="color:#fff;font:14px sans-serif">canvas (${T.file})</figcaption></figure><figure style="margin:0"><img src="data:image/jpeg;base64,${b.toString("base64")}" width="816"><figcaption style="color:#fff;font:14px sans-serif">template ${TEMPLATE} (Letter, sample data)</figcaption></figure>`);
    await both.screenshot({ path: path.join(out, `${TEMPLATE}-fidelity-${kind}.jpg`), type: "jpeg", quality: 80, fullPage: true });
    await both.close();
    for (const cb of canvasBoxes) {
      let best = null;
      for (const ob of ourBoxes.filter((o) => o.key === cb.key)) {
        const d = Math.max(Math.abs(cb.l - ob.l), Math.abs(cb.t - ob.t), Math.abs(cb.r - ob.r), Math.abs(cb.b - ob.b)) * MM;
        if (!best || d < best.d) best = { d, ob };
      }
      rows.push({ kind, key: cb.key, canvas: [cb.l, cb.t, cb.r, cb.b].map(Math.round), ours: best ? [best.ob.l, best.ob.t, best.ob.r, best.ob.b].map(Math.round) : null, mm: best ? Math.round(best.d * 10) / 10 : null });
    }
    await c.close();
  }
} finally {
  await browser.close();
  stop();
}
writeFileSync(path.join(out, "fidelity.json"), JSON.stringify(rows, null, 1));
for (const r of rows) console.log(`${r.kind.padEnd(8)} ${r.key.padEnd(22)} canvas ${r.canvas.join(",").padEnd(18)} ours ${(r.ours ?? ["-"]).join(",").padEnd(18)} ${r.mm === null ? "no box" : r.mm + " mm" + (r.mm > TOLERANCE_MM ? "  > 1.5" : "")}`);
