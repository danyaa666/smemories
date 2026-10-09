// Prints the prototype with Firefox through WebDriver BiDi (browsingContext.print): the Gecko print-to-PDF engine behind
// "Save as PDF", driven without a dialog and without geckodriver. Needs Node 22 (global WebSocket).
import { spawn } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { SIZES_MM, treeRssMb, url } from "./lib.mjs";

const FIREFOX =
  process.env.FIREFOX ??
  "/Applications/Firefox Developer Edition.app/Contents/MacOS/firefox";

export async function printWithFirefox(design, size, lang, file, { extra = "", before = "" } = {}) {
  const profile = mkdtempSync(path.join(os.tmpdir(), "ff-spike-"));
  const port = 9333;
  const child = spawn(
    FIREFOX,
    [
      "--headless",
      "--no-remote",
      "--profile",
      profile,
      "--remote-debugging-port",
      String(port),
      "about:blank",
    ],
    { stdio: "ignore" },
  );
  try {
    let ws;
    for (let i = 0; i < 100 && !ws; i++) {
      await new Promise((r) => setTimeout(r, 200));
      ws = await new Promise((res) => {
        const s = new WebSocket(`ws://127.0.0.1:${port}/session`);
        s.onopen = () => res(s);
        s.onerror = () => res(null);
      });
    }
    if (!ws) throw new Error("Firefox BiDi endpoint did not come up");
    let id = 0;
    const pending = new Map();
    ws.onmessage = (m) => {
      const d = JSON.parse(m.data);
      pending.get(d.id)?.(d);
    };
    const send = (method, params = {}) =>
      new Promise((res, rej) => {
        const n = ++id;
        pending.set(n, (d) =>
          d.error ? rej(new Error(`${d.error}: ${d.message}`)) : res(d.result),
        );
        ws.send(JSON.stringify({ id: n, method, params }));
      });
    const sess = await send("session.new", { capabilities: {} });
    const { contexts } = await send("browsingContext.getTree");
    const context = contexts[0].context;
    const t0 = performance.now();
    await send("browsingContext.navigate", {
      context,
      url: url(design, size, lang),
      wait: "complete",
    });
    for (let i = 0; i < 300; i++) {
      const r = await send("script.evaluate", {
        expression: "document.documentElement.dataset.ready ?? ''",
        target: { context },
        awaitPromise: false,
      });
      if (r.result?.value === "1") break;
      await new Promise((r) => setTimeout(r, 100));
    }
    const readyMs = Math.round(performance.now() - t0);
    if (before) await send("script.evaluate", { expression: before, target: { context }, awaitPromise: false }); // experiments only
    const { w, h } = SIZES_MM[size];
    const t1 = performance.now();
    let peak = treeRssMb(child.pid);
    const timer = setInterval(() => {
      peak = Math.max(peak, treeRssMb(child.pid));
    }, 100);
    const { data } = await send("browsingContext.print", {
      context,
      background: true,
      shrinkToFit: false,
      page: { width: w / 10, height: h / 10 },
      margin: { top: 0, bottom: 0, left: 0, right: 0 },
    });
    clearInterval(timer);
    const pdfMs = Math.round(performance.now() - t1);
    writeFileSync(file, Buffer.from(data, "base64"));
    const version = sess.capabilities?.browserVersion;
    ws.close();
    return { readyMs, pdfMs, peakMb: peak, version };
  } finally {
    child.kill();
  }
}
