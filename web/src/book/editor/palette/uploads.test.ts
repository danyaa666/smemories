import { describe, expect, it } from "vitest";
import { ApiError } from "../../../api/client";
import type { Media } from "../../../api/yearbooks";
import { failureOf, MAX_IN_FLIGHT, Uploads } from "./uploads";

// T-162: the upload state machine (client checks, progress, queue, cancel, double click) and the words for each server error.
const file = (name: string, type = "image/png", size = 100, modified = 1) => {
  const f = new File([new Uint8Array(Math.min(size, 10))], name, { type, lastModified: modified });
  Object.defineProperty(f, "size", { value: size });
  return f;
};
const media = (id: string): Media => ({
  id,
  yearbook_id: "Y",
  content_type: "image/png",
  bytes: 1,
  width: 10,
  height: 10,
  created_at: 1,
  purpose: "design",
});

/** An upload fake that waits for the test to finish each call. */
function setup(have = 0) {
  const calls: {
    file: File;
    progress: (f: number) => void;
    ok: (m: Media) => void;
    fail: (e: unknown) => void;
    signal: AbortSignal;
  }[] = [];
  const done: Media[] = [];
  const up = new Uploads({
    yearbookId: "Y",
    upload: (_y, f, progress, o) =>
      new Promise<Media>((ok, fail) => {
        o.signal.addEventListener("abort", () => fail(new DOMException("x", "AbortError")));
        calls.push({ file: f, progress, ok, fail, signal: o.signal });
      }),
  });
  up.bind({ have: () => have, onDone: (m) => done.push(m) });
  return { up, calls, done, jobs: () => up.getSnapshot() };
}
const tick = () => new Promise((r) => setTimeout(r, 0));

describe("Uploads", () => {
  it("queued -> uploading with progress -> done, and tells the screen", async () => {
    const { up, calls, done, jobs } = setup();
    up.add([file("a.png")]);
    expect(jobs()[0]).toMatchObject({ state: "uploading", progress: 0 });
    calls[0]!.progress(0.5);
    expect(jobs()[0]!.progress).toBe(0.5);
    calls[0]!.ok(media("M1"));
    await tick();
    expect(jobs()[0]).toMatchObject({ state: "done", progress: 1 });
    expect(done.map((m) => m.id)).toEqual(["M1"]);
  });
  it("runs at most MAX_IN_FLIGHT at once and starts the next when one ends", async () => {
    const { up, calls, jobs } = setup();
    up.add([1, 2, 3, 4, 5].map((i) => file(`f${i}.png`, "image/png", 100, i)));
    expect(MAX_IN_FLIGHT).toBeLessThanOrEqual(4);
    expect(calls).toHaveLength(MAX_IN_FLIGHT);
    expect(jobs().filter((j) => j.state === "queued")).toHaveLength(5 - MAX_IN_FLIGHT);
    calls[0]!.ok(media("M1"));
    await tick();
    expect(calls).toHaveLength(MAX_IN_FLIGHT + 1);
  });
  it("a double click on the same file is one upload; after it failed the file can be sent again", async () => {
    const { up, calls, jobs } = setup();
    const f = file("a.png");
    up.add([f]);
    up.add([f]);
    up.add([file("a.png")]); // the same name, size and time
    expect(jobs()).toHaveLength(1);
    expect(calls).toHaveLength(1);
    calls[0]!.fail(new ApiError(500, "internal_error", "x"));
    await tick();
    up.add([f]);
    expect(calls).toHaveLength(2);
  });
  it("checks the type and the 10 MiB cap before sending anything", () => {
    const { up, calls, jobs } = setup();
    up.add([
      file("a.gif", "image/gif"),
      file("big.png", "image/png", 10 * 1024 * 1024 + 1, 2),
      file("note.txt", "text/plain", 5, 3),
    ]);
    expect(calls).toHaveLength(0);
    expect(jobs().map((j) => j.failure?.code)).toEqual(["unsupported", "tooLarge", "unsupported"]);
  });
  it("cancelling a running or a queued upload removes it and aborts the request", async () => {
    const { up, calls, jobs } = setup();
    up.add([1, 2, 3].map((i) => file(`f${i}.png`, "image/png", 100, i)));
    const queued = jobs()[2]!.key;
    up.cancel(queued);
    expect(jobs()).toHaveLength(2);
    up.cancel(jobs()[0]!.key);
    await tick();
    expect(calls[0]!.signal.aborted).toBe(true);
    expect(jobs().map((j) => j.name)).toEqual(["f2.png"]);
  });
  it("a failed upload keeps its reason until dismissed", async () => {
    const { up, calls, jobs } = setup();
    up.add([file("a.png")]);
    calls[0]!.fail(new ApiError(0, "network_error", "x"));
    await tick();
    expect(jobs()[0]).toMatchObject({ state: "failed", failure: { code: "offline" } });
    up.dismiss(jobs()[0]!.key);
    expect(jobs()).toHaveLength(0);
  });
  it("the cap is told from the quota by how many images the book has", async () => {
    const { up, calls, jobs } = setup(30);
    up.add([file("a.png")]);
    calls[0]!.fail(new ApiError(400, "quota_exceeded", "x"));
    await tick();
    expect(jobs()[0]!.failure).toEqual({ code: "cap", count: 30 });
  });
});

describe("failureOf: each error has its own reason", () => {
  const e = (status: number, code: string, retry?: number) =>
    new ApiError(status, code, "m", undefined, retry);
  it.each([
    [e(0, "network_error"), 3, { code: "offline" }],
    [e(413, "payload_too_large"), 3, { code: "tooLarge" }],
    [e(415, "unsupported_media_type"), 3, { code: "unsupported" }],
    [e(400, "invalid_image"), 3, { code: "corrupt" }],
    [e(400, "quota_exceeded"), 30, { code: "cap", count: 30 }],
    [e(400, "quota_exceeded"), 12, { code: "quota" }],
    [e(429, "rate_limited", 7), 3, { code: "rateLimited", seconds: 7 }],
    [e(429, "rate_limited"), 3, { code: "rateLimited", seconds: 5 }],
    [e(503, "busy", 9), 3, { code: "busy", seconds: 9 }],
    [e(403, "email_not_verified"), 3, { code: "notVerified" }],
    [e(408, "request_timeout"), 3, { code: "timeout" }],
    [e(500, "storage_error"), 3, { code: "generic" }],
    [new Error("boom"), 3, { code: "generic" }],
  ])("%#", (err, have, want) => {
    expect(failureOf(err, have)).toEqual(want);
  });
  it("every reason has a message in both languages", async () => {
    const en = (await import("../../../locales/en.json")).default.editor.upload.error as Record<
      string,
      string
    >;
    const vi = (await import("../../../locales/vi.json")).default.editor.upload.error as Record<
      string,
      string
    >;
    for (const c of [
      "generic",
      "offline",
      "tooLarge",
      "unsupported",
      "corrupt",
      "cap",
      "quota",
      "rateLimited",
      "busy",
      "notVerified",
      "timeout",
    ]) {
      expect(en[c], c).toBeTruthy();
      expect(vi[c], c).toBeTruthy();
    }
    expect(vi.cap).toContain("{{count}}");
    expect(en.rateLimited).toContain("{{seconds}}");
  });
});
