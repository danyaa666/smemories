import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { LayoutDoc } from "./document";
import { EditorStore } from "./store";
import { mockServer } from "./mockServer";
import { setItemBox } from "./document";
import en from "../../locales/en.json";
import vi_ from "../../locales/vi.json";
import {
  historyError,
  MAX_CACHED,
  missingCount,
  PreviewCache,
  relativeTime,
  restoreVersion,
} from "./versions";
import { restoreLayoutVersion } from "../../api/bookLayout";

const NOW = Date.UTC(2026, 10, 2, 12, 0, 0);
const text = (id: string, w: number): LayoutDoc["pages"][string]["items"] => [
  { id, t: "text", text: "x", x: 100, w, h: 100, cy: 500 },
];
const doc = (w: number): LayoutDoc => ({ v: 1, pages: { cover: { items: text("textAA", w) } } });

afterEach(() => vi.unstubAllGlobals());

describe("relativeTime", () => {
  const ago = (s: number, l = "en") => relativeTime(NOW - s * 1000, NOW, l);
  it("moves from now to minutes, hours and days at the boundaries, in the app language", () => {
    expect(ago(0)).toBe("now");
    expect(ago(59)).toBe("now");
    expect(ago(60)).toBe("1 minute ago");
    expect(ago(12 * 60)).toBe("12 minutes ago");
    expect(ago(3599)).toBe("59 minutes ago");
    expect(ago(3600)).toBe("1 hour ago");
    expect(ago(47 * 3600)).toBe("47 hours ago");
    expect(ago(48 * 3600)).toBe("2 days ago");
    expect(ago(-30)).toBe("now"); // a server clock a little ahead never reads "in the future"
    expect(ago(12 * 60, "vi")).toBe("12 phút trước");
  });
});

describe("kind labels and error texts", () => {
  it("has a label for each kind and a text for each error, in vi and en", () => {
    for (const l of [en, vi_]) {
      for (const k of ["auto", "reset", "restore"]) expect(l.editor.history.kind).toHaveProperty(k);
      for (const k of [
        "conflict",
        "invalid",
        "template",
        "gone",
        "busy",
        "offline",
        "unsaved",
        "other",
      ])
        expect(l.editor.history.err).toHaveProperty(k);
    }
    expect(en.editor.history.kind.restore).toBe("Before restore");
  });
  it("maps every failure to its own key", () => {
    const e = (status: number, code: string) => new ApiError(status, code, "m");
    expect(historyError(e(409, "version_conflict"))).toBe("conflict");
    expect(historyError(e(400, "invalid_layout"))).toBe("invalid");
    expect(historyError(e(409, "version_template_changed"))).toBe("template");
    expect(historyError(e(404, "not_found"))).toBe("gone");
    expect(historyError(e(429, "rate_limited"))).toBe("busy");
    expect(historyError(e(0, "network_error"))).toBe("offline");
    expect(historyError(e(500, "internal_error"))).toBe("other");
    expect(historyError(new Error("x"))).toBe("other");
  });
});

describe("missingCount", () => {
  const d: LayoutDoc = {
    v: 1,
    pages: {
      cover: {
        bg: { media: "gone1" },
        items: [
          { id: "aaaaaa", t: "asset", asset: "sticker.gold-star", x: 1, w: 100, h: 100, cy: 1 },
          { id: "bbbbbb", t: "asset", asset: "sticker.removed", x: 1, w: 100, h: 100, cy: 1 },
          { id: "cccccc", t: "image", media: "ok1", x: 1, w: 100, h: 100, cy: 1 },
          { id: "dddddd", t: "image", media: "gone2", x: 1, w: 100, h: 100, cy: 1 },
        ],
      },
      back: { bg: { asset: "bg.removed" } },
    },
  };
  const hasAsset = (id: string) => !id.includes("removed");
  it("counts items and backgrounds whose asset or design image is gone", () => {
    expect(missingCount(d, hasAsset, (id) => id === "ok1")).toBe(4);
  });
  it("counts no image while the image list is not complete", () => {
    expect(missingCount(d, hasAsset, null)).toBe(2);
  });
});

describe("PreviewCache", () => {
  it("keeps at most 5 bodies and drops the least recently used", () => {
    const c = new PreviewCache<number>();
    for (let i = 0; i < 5; i++) c.set(`k${i}`, i);
    c.get("k0"); // k0 is fresh again
    c.set("k5", 5);
    expect(c.size).toBe(MAX_CACHED);
    expect(c.get("k1")).toBeUndefined();
    expect(c.get("k0")).toBe(0);
  });
});

describe("restoreVersion", () => {
  const setup = async () => {
    const srv = mockServer({
      version: 4,
      layout: doc(300),
      versions: [
        { id: "V1".padEnd(26, "0"), version: 2, kind: "auto", created_at: 1, layout: doc(111) },
      ],
    });
    const store = new EditorStore("Y");
    store.start();
    await store.load();
    const deps = (over: Partial<Parameters<typeof restoreVersion>[1]> = {}) => ({
      settle: store.settle,
      version: store.currentVersion,
      restore: (id: string, base: number) => restoreLayoutVersion("Y", id, base),
      adopt: store.adopt.bind(store),
      enterConflict: store.enterConflict.bind(store),
      ...over,
    });
    return { srv, store, deps, id: "V1".padEnd(26, "0") };
  };

  it("saves the pending edit first, asks with the store's version, adopts the answer and clears undo and redo", async () => {
    const { srv, store, deps, id } = await setup();
    store.apply(setItemBox("cover", "textAA", { w: 250 }));
    store.undo();
    store.apply(setItemBox("cover", "textAA", { w: 260 })); // dirty, with an undo step
    expect(store.getSnapshot().canUndo).toBe(true);
    const r = await restoreVersion(id, deps());
    expect(r).toEqual({ ok: true, dropped: 0 });
    expect(srv.s.history).toEqual([`restore:${id}@5`]); // the save moved 4 to 5 before the restore asked
    expect(srv.s.calls).toEqual(["save"]);
    const snap = store.getSnapshot();
    expect(snap.version).toBe(6);
    expect(snap.doc).toEqual(doc(111));
    expect(snap.canUndo || snap.canRedo).toBe(false);
    expect(snap.dirty).toBe(false);
    expect(snap.saveState).toBe("idle");
    expect(srv.s.versions[0]?.kind).toBe("restore"); // the replaced document is kept by the server
  });

  it("does not ask when the pending edit cannot be saved, and changes nothing", async () => {
    const { srv, store, deps, id } = await setup();
    srv.s.script.push(() => Promise.reject(new TypeError("offline")));
    store.apply(setItemBox("cover", "textAA", { w: 250 }));
    const r = await restoreVersion(id, deps());
    expect(r).toEqual({ ok: false, error: "unsaved" });
    expect(srv.s.history).toEqual([]);
    expect(store.getSnapshot().doc).toEqual(doc(250));
    expect(store.getSnapshot().canUndo).toBe(true);
    store.stop();
  });

  it("starts the conflict flow on 409 and leaves the document and history alone", async () => {
    const { srv, store, deps, id } = await setup();
    srv.s.restoreScript.push(() =>
      srv.fail(409, "ERROR_VERSION_CONFLICT", { version: 9, layout: doc(7) }),
    );
    const r = await restoreVersion(id, deps());
    expect(r).toMatchObject({ ok: false, error: "conflict" });
    expect(store.getSnapshot().saveState).toBe("conflict");
    expect(store.getSnapshot().doc).toEqual(doc(300));
    await store.useTheirs(); // the stored document of the answer, nothing was overwritten
    expect(store.getSnapshot().doc).toEqual(doc(7));
    expect(store.getSnapshot().version).toBe(9);
  });

  it.each([
    ["invalid", "ERROR_INVALID_LAYOUT", 400, { problems: [{ path: "p", code: "c" }] }],
    ["template", "ERROR_VERSION_TEMPLATE_CHANGED", 409, undefined],
    ["gone", "ERROR_NOT_FOUND", 404, undefined],
  ] as [string, string, number, object | undefined][])(
    "maps %s and changes nothing",
    async (error, code, status, data) => {
      const { srv, store, deps, id } = await setup();
      store.apply(setItemBox("cover", "textAA", { w: 250 }));
      await store.settle();
      const before = store.getSnapshot();
      srv.s.restoreScript.push(() => srv.fail(status, code, data));
      expect(await restoreVersion(id, deps())).toMatchObject({ ok: false, error });
      expect(store.getSnapshot()).toEqual(before);
    },
  );

  it("reports busy with the Retry-After and offline", async () => {
    const { srv, deps, id } = await setup();
    srv.s.restoreScript.push(() =>
      srv.fail(429, "ERROR_RATE_LIMITED", undefined, { "Retry-After": "7" }),
    );
    expect(await restoreVersion(id, deps())).toEqual({ ok: false, error: "busy", retryAfter: 7 });
    srv.s.restoreScript.push(() => Promise.reject(new TypeError("offline")));
    expect(await restoreVersion(id, deps())).toEqual({ ok: false, error: "offline" });
  });

  it("passes the dropped count on", async () => {
    const { srv, deps, id } = await setup();
    srv.s.dropped = 3;
    expect(await restoreVersion(id, deps())).toEqual({ ok: true, dropped: 3 });
  });
});
