// Runs the test matrix of spike T-054 on this machine and writes docs/spikes/print/results.json plus the PDFs that fit the 5 MB limit (the PDFs are git-ignored).
//   cd tools/print-spike && npm install && npx playwright install chromium && node measure.mjs
// Browsers other than Playwright's Chromium are used if installed: Chrome, Edge Dev, Edge Canary (Playwright channels) and Firefox
// (FIREFOX=/path/to/firefox; a stock build, `npx @puppeteer/browsers install firefox@stable`). Safari, Android and iOS cannot be automated here.
import { chromium } from "playwright";
import { copyFileSync, mkdirSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { printWithFirefox } from "./firefox-bidi.mjs";
import {
  inspect,
  open,
  ROOT,
  savePdf,
  startServer,
  text,
  treeRssMb,
} from "./lib.mjs";

const OUT = path.join(ROOT, "docs/spikes/print");
const TMP = path.join(os.tmpdir(), "print-spike");
const LIMIT = 5 * 1024 * 1024;
const LARGE = process.env.LARGE_PHOTOS; // directory made by `make-fixtures.py large DIR`
mkdirSync(OUT, { recursive: true });
mkdirSync(TMP, { recursive: true });

const NAMES = [
  "Nguyễn Thị Hồng Ánh",
  "Trần Quốc Việt Hưng",
  "Đặng Ngọc Bích Trâm",
  "Huỳnh Ngọc Diệp",
  "Tạ Thị Thu Hiền",
  "Lương Bảo Châu",
];
// What must be extractable as text from the PDF (Vietnamese diacritics, emoji). The classic design prints names and titles in capitals.
const check = (pdf, design) => {
  const t = text(pdf);
  const names =
    design === "memory" ? NAMES : NAMES.slice(0, 1).map((n) => n.toUpperCase());
  const sentence =
    design === "memory" ? "Chuyến dã ngoại" : "Gửi các bạn khóa 2026";
  return {
    names: `${names.filter((n) => t.includes(n)).length}/${names.length}`,
    emoji: `${["😄", "❤️", "🎂", "🍲", "🎓"].filter((e) => t.includes(e)).length}/5`,
    sentence: t.includes(sentence),
  };
};

/** Peak resident memory (MB, sum over the process tree, shared pages counted once per process so an upper bound) while fn runs. */
async function withPeak(pid, fn) {
  let peak = treeRssMb(pid);
  const timer = setInterval(() => {
    peak = Math.max(peak, treeRssMb(pid));
  }, 100);
  try {
    return {
      value: await fn(),
      peakMb: (peak = Math.max(peak, treeRssMb(pid))),
      get: () => peak,
    };
  } finally {
    clearInterval(timer);
  }
}

const results = {
  machine: `${os.cpus()[0].model}, ${os.cpus().length} cores, ${Math.round(os.totalmem() / 2 ** 30)} GB, macOS ${os.release()} (Darwin)`,
  node: process.version,
  runs: [],
};
const record = (r) => {
  results.runs.push(r);
  console.log(JSON.stringify(r));
};

async function viaPlaywright(id, launchOpts, jobs) {
  let server;
  try {
    server = await chromium.launchServer(launchOpts);
  } catch (e) {
    record({ browser: id, error: String(e.message).split("\n")[0] });
    return;
  }
  const browser = await chromium.connect(server.wsEndpoint());
  const version = browser.version();
  const pid = server.process().pid;
  for (const {
    design,
    size,
    lang,
    extra = "",
    tag = "",
    keep = false,
    large = false,
  } of jobs) {
    const page = await browser.newPage();
    const hosts = new Set();
    page.on("request", (r) => hosts.add(new URL(r.url()).host));
    if (large && LARGE)
      await page.route("**/fixtures/photo-*.jpg", (route) =>
        route.fulfill({
          path: path.join(
            LARGE,
            /photo-\d+\.jpg/.exec(route.request().url())[0],
          ),
        }),
      );
    let peak = treeRssMb(pid);
    const timer = setInterval(() => {
      peak = Math.max(peak, treeRssMb(pid));
    }, 50);
    const opened = await open(page, design, size, lang, extra);
    const file = path.join(TMP, `${id}-${design}-${size}-${lang}${tag}.pdf`);
    const saved = await savePdf(page, file);
    clearInterval(timer);
    const info = inspect(file);
    const kept = keep && saved.bytes < LIMIT;
    if (kept) copyFileSync(file, path.join(OUT, path.basename(file)));
    record({
      browser: id,
      version,
      design,
      size,
      lang,
      tag,
      ...opened,
      ...saved,
      peakMb: peak,
      committed: kept,
      ...info,
      fonts: info.fonts.length,
      ...check(file, design),
      externalHosts: [...hosts].filter((h) => h !== "localhost:5199"),
    });
    await page.close();
  }
  await browser.close();
  await server.close();
}

const stop = await startServer();
try {
  const sizes = ["a5", "a4", "letter"];
  await viaPlaywright("chromium-shell", {}, [
    ...sizes.map((size) => ({
      design: "memory",
      size,
      lang: "vi",
      keep: size !== "a4",
    })),
    ...sizes.map((size) => ({
      design: "classic",
      size,
      lang: "vi",
      keep: true,
    })),
    { design: "memory", size: "a5", lang: "en", keep: false },
    {
      design: "memory",
      size: "a5",
      lang: "vi",
      extra: "&scale=transform",
      tag: "-transform",
    },
    ...(LARGE
      ? sizes.map((size) => ({
          design: "memory",
          size,
          lang: "vi",
          large: true,
          tag: "-3000px",
        }))
      : []),
  ]);
  for (const [id, channel] of [
    ["chrome", "chrome"],
    ["edge-dev", "msedge-dev"],
    ["edge-canary", "msedge-canary"],
  ]) {
    await viaPlaywright(id, { channel }, [
      { design: "memory", size: "a5", lang: "vi" },
      { design: "classic", size: "a5", lang: "vi", keep: true },
    ]);
  }
  if (process.env.FIREFOX) {
    for (const [design, keep] of [
      ["memory", false],
      ["classic", true],
    ]) {
      const file = path.join(TMP, `firefox-${design}-a5-vi.pdf`);
      const r = await printWithFirefox(design, "a5", "vi", file);
      const info = inspect(file);
      const bytes = (await import("node:fs")).statSync(file).size;
      if (keep && bytes < LIMIT)
        copyFileSync(file, path.join(OUT, path.basename(file)));
      record({
        browser: "firefox",
        design,
        size: "a5",
        lang: "vi",
        ...r,
        bytes,
        committed: keep && bytes < LIMIT,
        ...info,
        fonts: info.fonts.length,
        ...check(file, design),
      });
    }
  }
} finally {
  stop();
}
writeFileSync(
  path.join(OUT, "results.json"),
  JSON.stringify(results, null, 1) + "\n",
);
