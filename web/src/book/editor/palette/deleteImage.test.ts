import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../../api/client";
import { deleteMedia } from "../../../api/yearbooks";
import { addItem, setBg, type LayoutDoc, type LayoutItem } from "../document";
import { mockServer } from "../mockServer";
import { EditorStore } from "../store";
import { deleteDesignImage } from "./deleteImage";

// T-162 requirement 7 / AC3: a design image in use is removed from the pages, the save is waited for, only then is the image deleted,
// and undo and redo are cleared; an unused image keeps the history. Real store, fake server.
const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0W1XY";
const IMG = "01J9Z3K6V8Q4M7N2P5R8IMG001";
const OTHER = "01J9Z3K6V8Q4M7N2P5R8IMG002";
const img = (id: string, media: string): LayoutItem => ({
  id,
  t: "image",
  media,
  x: 100,
  w: 400,
  h: 300,
  cy: 300,
});
const LAYOUT: LayoutDoc = {
  v: 1,
  pages: {
    cover: { bg: { media: IMG }, items: [img("aaaaaa", IMG), img("bbbbbb", OTHER)] },
    profile: { items: [img("cccccc", IMG)] },
  },
};

const mediaUsesNow = (store: EditorStore) => JSON.stringify(store.getSnapshot().doc).includes(IMG);

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

async function open(layout: LayoutDoc) {
  const srv = mockServer({
    version: 3,
    layout,
    media: [
      { id: IMG, width: 10, height: 10 },
      { id: OTHER, width: 10, height: 10 },
    ],
  });
  const store = new EditorStore(BOOK);
  store.start();
  await vi.waitFor(() => expect(store.getSnapshot().loaded).toBe("ready"));
  const deps = () => ({
    doc: () => store.getSnapshot().doc,
    apply: store.apply.bind(store),
    settle: store.settle,
    clearHistory: store.clearHistory.bind(store),
    usesInHistory: store.usesImageInHistory.bind(store),
    deleteMedia,
  });
  return { srv, store, deps };
}

describe("deleteDesignImage", () => {
  it("in use: removes items and background, saves, deletes the image, clears undo and redo", async () => {
    const { srv, store, deps } = await open(LAYOUT);
    store.apply(addItem("cover", img("dddddd", OTHER))); // a step in the history
    store.undo();
    expect(store.getSnapshot()).toMatchObject({ canUndo: false, canRedo: true });
    store.apply(addItem("cover", img("dddddd", OTHER)));
    expect(store.getSnapshot().canUndo).toBe(true);
    const r = await deleteDesignImage(IMG, deps());
    expect(r).toEqual({ ok: true, cleared: true });
    // the order: the layout without the image is saved BEFORE the image is deleted
    expect(srv.s.calls).toEqual(["save", `delete:${IMG}`]);
    const stored = srv.s.layout!;
    expect(JSON.stringify(stored)).not.toContain(IMG);
    expect(stored.pages.cover!.items!.map((i) => i.id)).toEqual(["bbbbbb", "dddddd"]); // the others stay
    expect(stored.pages.cover!.bg).toBeUndefined();
    expect(store.getSnapshot()).toMatchObject({ canUndo: false, canRedo: false, dirty: false });
    store.undo(); // nothing to undo: the image cannot come back
    expect(JSON.stringify(store.getSnapshot().doc)).not.toContain(IMG);
  });
  it("not in use: just deletes it and keeps the history", async () => {
    const { srv, store, deps } = await open({
      v: 1,
      pages: { cover: { items: [img("bbbbbb", OTHER)] } },
    });
    store.apply(setBg("cover", { color: "paper" }));
    const r = await deleteDesignImage(IMG, deps());
    expect(r).toEqual({ ok: true, cleared: false });
    expect(srv.s.calls).toEqual([`delete:${IMG}`]); // no save was needed for the delete
    expect(store.getSnapshot().canUndo).toBe(true);
  });
  // T-164 requirement 3 / AC3: only the history points at the image (the item was removed before), undo must not bring it back
  it("only the history uses it: no save, delete, and the history is cleared so undo cannot bring the item back", async () => {
    const { srv, store, deps } = await open({
      v: 1,
      pages: { cover: { items: [img("bbbbbb", OTHER)] } },
    });
    store.apply(addItem("cover", img("aaaaaa", IMG))); // add an image item ...
    store.apply((doc) => ({
      ok: true,
      doc: { ...doc, pages: { cover: { items: [img("bbbbbb", OTHER)] } } },
    })); // ... and remove it again
    expect(mediaUsesNow(store)).toBe(false);
    expect(store.usesImageInHistory(IMG)).toBe(true);
    const r = await deleteDesignImage(IMG, deps());
    expect(r).toEqual({ ok: true, cleared: true });
    expect(srv.s.calls).toEqual([`delete:${IMG}`]); // nothing to save: no page uses it
    expect(store.getSnapshot()).toMatchObject({ canUndo: false, canRedo: false });
    store.undo();
    expect(JSON.stringify(store.getSnapshot().doc)).not.toContain(IMG);
    expect(store.usesImageInHistory(IMG)).toBe(false);
  });
  it("the layout cannot be saved: the image is NOT deleted", async () => {
    const { srv, store, deps } = await open(LAYOUT);
    srv.s.script.push(() => srv.fail(500, "ERROR_INTERNAL"));
    const r = await deleteDesignImage(IMG, deps());
    expect(r).toEqual({ ok: false, stage: "save" });
    expect(srv.s.deleted).toEqual([]);
    expect(store.getSnapshot().dirty).toBe(true); // the autosave keeps trying
    store.stop();
  });
  it("the delete call fails after the save: the server layout is without the image, the history stays, the error is returned", async () => {
    const { srv, store, deps } = await open(LAYOUT);
    const fetchMock = globalThis.fetch as unknown as ReturnType<typeof vi.fn>;
    const real = fetchMock.getMockImplementation()! as unknown as (
      u: string,
      r?: RequestInit,
    ) => Promise<Response>;
    fetchMock.mockImplementation(async (url: string, req?: RequestInit) =>
      url === "/api/media/delete"
        ? Response.json(
            { status: "ERROR_STORAGE_ERROR", error_message: "x", request_id: "r" },
            { status: 502 },
          )
        : real(url, req),
    );
    store.apply(setBg("back", { color: "paper" }));
    const r = await deleteDesignImage(IMG, deps());
    expect(r).toMatchObject({ ok: false, stage: "delete" });
    expect((r as { error: unknown }).error).toBeInstanceOf(ApiError);
    expect(JSON.stringify(srv.s.layout)).not.toContain(IMG); // nothing is lost on the server
    expect(store.getSnapshot().canUndo).toBe(true);
  });
});
