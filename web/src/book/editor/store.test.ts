import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  addItem,
  emptyDoc,
  setItemBox,
  setText,
  type LayoutDoc,
  type LayoutItem,
} from "./document";
import { mockServer } from "./mockServer";
import { DEBOUNCE_MS, EditorStore } from "./store";

const ID = "01J9Z3K6V8Q4M7N2P5R8T0W1XY";
const item = (id: string, over: Partial<LayoutItem> = {}): LayoutItem => ({
  id,
  t: "text",
  text: "hi",
  x: 100,
  w: 400,
  h: 120,
  cy: 500,
  ...over,
});
const doc1 = (id = "aaaaaa"): LayoutDoc => ({ v: 1, pages: { cover: { items: [item(id)] } } });
const tick = (ms: number) => vi.advanceTimersByTimeAsync(ms);

async function open(init: { version: number; layout: LayoutDoc | null }) {
  const srv = mockServer(init);
  const store = new EditorStore(ID);
  store.start();
  await tick(0);
  return { srv, store, snap: () => store.getSnapshot() };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("load", () => {
  it("takes base_version from book-layout/get; a null layout is an empty document, not dirty", async () => {
    const { srv, snap } = await open({ version: 7, layout: null });
    expect(snap()).toMatchObject({
      loaded: "ready",
      version: 7,
      dirty: false,
      saveState: "idle",
      doc: emptyDoc(),
    });
    expect(srv.s.gets).toBe(1);
    await tick(10_000);
    expect(srv.s.saves).toHaveLength(0); // nothing edited, nothing saved
  });
  it("404 on load is `gone`", async () => {
    const { srv } = { srv: mockServer({ version: 0, layout: null }) };
    srv.s.gets = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => srv.fail(404, "ERROR_NOT_FOUND")),
    );
    const store = new EditorStore(ID);
    store.start();
    await tick(0);
    expect(store.getSnapshot()).toMatchObject({ loaded: "failed", saveState: "gone" });
  });
});

describe("autosave", () => {
  it("waits 1.5 s after the last commit, then saves once with base_version", async () => {
    const { srv, store, snap } = await open({ version: 3, layout: null });
    store.apply(addItem("cover", item("aaaaaa")));
    await tick(1000);
    store.apply(setItemBox("cover", "aaaaaa", { x: 200 })); // restarts the countdown
    await tick(DEBOUNCE_MS - 1);
    expect(srv.s.saves).toHaveLength(0);
    expect(snap().dirty).toBe(true);
    await tick(1);
    expect(srv.s.saves).toHaveLength(1);
    expect(srv.s.saves[0]).toMatchObject({ yearbook_id: ID, base_version: 3 });
    expect(srv.s.saves[0]!.layout.pages.cover?.items?.[0]).toMatchObject({ x: 200 });
    expect(snap()).toMatchObject({ version: 4, dirty: false, saveState: "saved" });
  });

  it("never has two saves in flight; a commit during a save queues one more of the newest document", async () => {
    const { srv, store, snap } = await open({ version: 0, layout: null });
    let release!: () => void;
    const held = new Promise<void>((r) => (release = r));
    srv.s.script.push(async () => {
      await held;
      srv.s.version = 1;
      return srv.body({ version: 1, updated_at: 1 });
    });
    store.apply(addItem("cover", item("aaaaaa")));
    await tick(DEBOUNCE_MS);
    expect(snap().saveState).toBe("saving");
    store.apply(setText("cover", "aaaaaa", { text: "two" }));
    store.apply(setText("cover", "aaaaaa", { text: "three" }));
    await tick(DEBOUNCE_MS * 2);
    expect(srv.s.saves).toHaveLength(1); // still the first one only
    expect(snap().dirty).toBe(true);
    release();
    await tick(0);
    expect(srv.s.saves).toHaveLength(2);
    expect(srv.s.saves[1]).toMatchObject({ base_version: 1 }); // the version the first answer gave
    expect(srv.s.saves[1]!.layout.pages.cover?.items?.[0]).toMatchObject({ text: "three" });
    await tick(DEBOUNCE_MS * 2);
    expect(srv.s.saves).toHaveLength(2);
    expect(snap()).toMatchObject({ version: 2, dirty: false, saveState: "saved" });
  });

  it("a gesture saves nothing until commit, and is one history step", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    store.begin();
    for (let x = 101; x <= 150; x++) store.preview(setItemBox("cover", "aaaaaa", { x }));
    await tick(5000);
    expect(srv.s.saves).toHaveLength(0);
    expect(snap().doc.pages.cover?.items?.[0]).toMatchObject({ x: 150 });
    expect(snap().dirty).toBe(false);
    store.commit();
    expect(snap().dirty).toBe(true);
    await tick(DEBOUNCE_MS);
    expect(srv.s.saves).toHaveLength(1);
    expect(srv.s.saves[0]!.layout.pages.cover?.items?.[0]).toMatchObject({ x: 150 });
    store.undo();
    expect(snap().doc.pages.cover?.items?.[0]).toMatchObject({ x: 100 });
    expect(snap().canUndo).toBe(false);
  });

  it("a refused preview leaves the document as it was and says why", async () => {
    const { store, snap } = await open({ version: 1, layout: doc1() });
    store.begin();
    const before = snap().doc;
    expect(store.preview(setItemBox("cover", "aaaaaa", { w: 5 }))).toBe("bad_box");
    expect(snap().doc).toBe(before);
  });

  it("undo and redo mark the document dirty and are saved like any edit", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    store.apply(setText("cover", "aaaaaa", { text: "changed" }));
    await tick(DEBOUNCE_MS);
    store.undo();
    expect(snap()).toMatchObject({ dirty: true, canRedo: true });
    await tick(DEBOUNCE_MS);
    expect(srv.s.saves).toHaveLength(2);
    expect(srv.s.saves[1]!.layout.pages.cover?.items?.[0]).toMatchObject({ text: "hi" });
    store.redo();
    await tick(DEBOUNCE_MS);
    expect(srv.s.layout?.pages.cover?.items?.[0]).toMatchObject({ text: "changed" });
  });

  it("flush saves now", async () => {
    const { srv, store } = await open({ version: 1, layout: doc1() });
    store.apply(setText("cover", "aaaaaa", { text: "now" }));
    store.flush();
    await tick(0);
    expect(srv.s.saves).toHaveLength(1);
    await tick(DEBOUNCE_MS * 2);
    expect(srv.s.saves).toHaveLength(1); // the countdown was cancelled
  });

  it("stop() gives unsaved edits one last save and leaves no timer", async () => {
    const { srv, store } = await open({ version: 1, layout: doc1() });
    store.apply(setText("cover", "aaaaaa", { text: "bye" }));
    store.stop();
    await tick(0);
    expect(srv.s.saves).toHaveLength(1);
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("conflict (409)", () => {
  async function conflicted() {
    const t = await open({ version: 1, layout: doc1() });
    t.srv.s.otherTab(doc1("bbbbbb")); // another tab saved: version 2
    t.store.apply(setText("cover", "aaaaaa", { text: "mine" }));
    await tick(DEBOUNCE_MS);
    return t;
  }

  it("enters conflict, overwrites nothing and stops autosaving", async () => {
    const { srv, store, snap } = await conflicted();
    expect(snap().saveState).toBe("conflict");
    expect(srv.s.layout).toEqual(doc1("bbbbbb")); // the stored document is untouched
    expect(srv.s.saves).toHaveLength(1);
    store.apply(setText("cover", "aaaaaa", { text: "more" })); // keeps editing locally
    store.flush();
    await tick(DEBOUNCE_MS * 5);
    expect(srv.s.saves).toHaveLength(1);
    expect(snap().dirty).toBe(true);
    expect(srv.s.layout).toEqual(doc1("bbbbbb"));
  });

  it("useTheirs replaces the document with the stored one and clears the history", async () => {
    const { srv, store, snap } = await conflicted();
    await store.useTheirs();
    expect(snap()).toMatchObject({
      doc: doc1("bbbbbb"),
      version: 2,
      dirty: false,
      saveState: "idle",
      canUndo: false,
      canRedo: false,
    });
    await tick(DEBOUNCE_MS * 2);
    expect(srv.s.saves).toHaveLength(1);
  });

  it("keepMine saves my document on purpose, naming the stored version", async () => {
    const { srv, store, snap } = await conflicted();
    await store.keepMine();
    await tick(0);
    expect(srv.s.saves).toHaveLength(2);
    expect(srv.s.saves[1]).toMatchObject({ base_version: 2 });
    expect(srv.s.layout?.pages.cover?.items?.[0]).toMatchObject({ text: "mine" });
    expect(snap()).toMatchObject({ version: 3, dirty: false, saveState: "saved", canUndo: true });
  });

  it("after a reset in another tab the conflict carries layout null; useTheirs gives an empty document", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    srv.s.otherTab(null);
    store.apply(setText("cover", "aaaaaa", { text: "mine" }));
    await tick(DEBOUNCE_MS);
    await store.useTheirs();
    expect(snap()).toMatchObject({ doc: emptyDoc(), version: 2 });
  });

  it("ABA: a stale base_version never wins, even when the stored document equals ours", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    srv.s.otherTab(doc1("bbbbbb"));
    srv.s.otherTab(doc1()); // back to the document we loaded, but version 3
    store.apply(setText("cover", "aaaaaa", { text: "mine" }));
    await tick(DEBOUNCE_MS);
    expect(snap().saveState).toBe("conflict");
    expect(srv.s.version).toBe(3);
    expect(srv.s.layout).toEqual(doc1());
  });
});

describe("other failures", () => {
  it("429 waits Retry-After, then retries", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    srv.s.script.push(() => srv.fail(429, "ERROR_RATE_LIMITED", undefined, { "Retry-After": "7" }));
    store.apply(setText("cover", "aaaaaa", { text: "x" }));
    await tick(DEBOUNCE_MS);
    expect(srv.s.saves).toHaveLength(1);
    expect(snap().dirty).toBe(true);
    await tick(6900);
    expect(srv.s.saves).toHaveLength(1);
    await tick(100);
    expect(srv.s.saves).toHaveLength(2);
    expect(snap()).toMatchObject({ dirty: false, saveState: "saved" });
  });

  it("network errors and 5xx retry after 1, 2, 4, 8 s, then every 30 s, and keep the document dirty", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    const down = () => {
      throw new TypeError("Failed to fetch");
    };
    srv.s.script.push(down, () => srv.fail(503, "ERROR_INTERNAL"), down, down, down, down);
    store.apply(setText("cover", "aaaaaa", { text: "x" }));
    await tick(DEBOUNCE_MS);
    let n = 1;
    for (const wait of [1000, 2000, 4000, 8000, 30_000]) {
      expect(srv.s.saves, `before ${wait}`).toHaveLength(n);
      expect(snap()).toMatchObject({ saveState: "offline", dirty: true });
      await tick(wait - 1);
      expect(srv.s.saves).toHaveLength(n);
      await tick(1);
      n++;
    }
    expect(srv.s.saves).toHaveLength(6);
    expect(snap().saveState).toBe("offline"); // the sixth failed too
    await tick(30_000);
    expect(snap()).toMatchObject({ saveState: "saved", dirty: false, version: 2 });
    expect(srv.s.saves.every((s) => s.base_version === 1)).toBe(true); // every retry names the same version
  });

  it("400 goes to invalid with the problem path and sends nothing until the next commit", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    const problems = [{ path: "pages.cover.items[0].x", code: "out_of_range" }];
    srv.s.script.push(() => srv.fail(400, "ERROR_INVALID_LAYOUT", { problems }));
    store.apply(setText("cover", "aaaaaa", { text: "x" }));
    await tick(DEBOUNCE_MS);
    expect(snap()).toMatchObject({ saveState: "invalid", problems, dirty: true });
    store.flush();
    await tick(60_000);
    expect(srv.s.saves).toHaveLength(1);
    store.apply(setText("cover", "aaaaaa", { text: "y" }));
    expect(snap().saveState).toBe("idle");
    await tick(DEBOUNCE_MS);
    expect(srv.s.saves).toHaveLength(2);
    expect(snap()).toMatchObject({ saveState: "saved", problems: [] });
  });

  it("413 is invalid too (no problem list)", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    srv.s.script.push(() => srv.fail(413, "ERROR_TOO_LARGE"));
    store.apply(setText("cover", "aaaaaa", { text: "x" }));
    await tick(DEBOUNCE_MS);
    expect(snap()).toMatchObject({ saveState: "invalid", problems: [] });
  });

  it("404 and 403 go to gone and are not retried", async () => {
    for (const status of [404, 403]) {
      const { srv, store, snap } = await open({ version: 1, layout: doc1() });
      srv.s.script.push(() => srv.fail(status, "ERROR_NOT_FOUND"));
      store.apply(setText("cover", "aaaaaa", { text: "x" }));
      await tick(DEBOUNCE_MS);
      expect(snap().saveState).toBe("gone");
      store.apply(setText("cover", "aaaaaa", { text: "y" }));
      await tick(120_000);
      expect(srv.s.saves).toHaveLength(1);
      expect(vi.getTimerCount()).toBe(0);
    }
  });
});

describe("reset", () => {
  it("clears history, takes the version the reset moved on to, and the next save names it", async () => {
    const { srv, store, snap } = await open({ version: 4, layout: doc1() });
    store.apply(setText("cover", "aaaaaa", { text: "x" }));
    await store.reset();
    expect(srv.s.resets).toBe(1);
    expect(snap()).toMatchObject({
      doc: emptyDoc(),
      version: 5,
      dirty: false,
      canUndo: false,
      saveState: "idle",
    });
    await tick(DEBOUNCE_MS * 2);
    expect(srv.s.saves).toHaveLength(0); // the pending autosave was cancelled
    store.apply(addItem("cover", item("cccccc")));
    await tick(DEBOUNCE_MS);
    expect(srv.s.saves[0]).toMatchObject({ base_version: 5 });
  });
});

describe("scripted session (AC2)", () => {
  it("30 edits, 10 undos, 5 redos and a conflict end with the expected document, version and state, and no lost update", async () => {
    const { srv, store, snap } = await open({ version: 1, layout: doc1() });
    const edit = (i: number) => store.apply(setText("cover", "aaaaaa", { text: `edit ${i}` }));
    for (let i = 1; i <= 30; i++) edit(i);
    await tick(DEBOUNCE_MS);
    expect(srv.s.saves).toHaveLength(1); // 30 quick edits are one save
    for (let i = 0; i < 10; i++) store.undo();
    for (let i = 0; i < 5; i++) store.redo();
    expect(snap().doc.pages.cover?.items?.[0]).toMatchObject({ text: "edit 25" }); // 30 - 10 + 5
    await tick(DEBOUNCE_MS);
    expect(srv.s.layout?.pages.cover?.items?.[0]).toMatchObject({ text: "edit 25" });
    expect(snap()).toMatchObject({ version: 3, dirty: false, saveState: "saved" });
    srv.s.otherTab(doc1("bbbbbb")); // someone else saves: version 4
    edit(31);
    await tick(DEBOUNCE_MS);
    expect(snap()).toMatchObject({ saveState: "conflict", version: 3, dirty: true });
    expect(srv.s.layout).toEqual(doc1("bbbbbb")); // theirs is still stored
    await store.keepMine();
    await tick(0);
    expect(srv.s.saves.map((s) => s.base_version)).toEqual([1, 2, 3, 4]);
    expect(srv.s.layout?.pages.cover?.items?.[0]).toMatchObject({ text: "edit 31" });
    expect(snap()).toMatchObject({ version: 5, dirty: false, saveState: "saved" });
    expect(srv.s.saves.length).toBe(4); // never two at once, never during the conflict
  });
});

describe("selection", () => {
  it("is not part of the history", async () => {
    const { store, snap } = await open({ version: 1, layout: doc1() });
    store.select(["aaaaaa"]);
    expect(snap().selection).toEqual(["aaaaaa"]);
    expect(snap().canUndo).toBe(false);
    expect(snap().dirty).toBe(false);
  });
});
