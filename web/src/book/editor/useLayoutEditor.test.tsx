import { act, cleanup, render, screen } from "@testing-library/react";
import { useEffect } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "../../i18n";
import { ops, type LayoutDoc } from "./document";
import { mockServer } from "./mockServer";
import { SaveStatus } from "./SaveStatus";
import { DEBOUNCE_MS } from "./store";
import { useLayoutEditor } from "./useLayoutEditor";

const ID = "01J9Z3K6V8Q4M7N2P5R8T0W1XY";
const doc: LayoutDoc = {
  v: 1,
  pages: {
    cover: { items: [{ id: "aaaaaa", t: "text", text: "hi", x: 100, w: 400, h: 120, cy: 500 }] },
  },
};

type Editor = ReturnType<typeof useLayoutEditor>;
const latest: { current?: Editor } = {};
const ed = () => latest.current!;
function Probe() {
  const e = useLayoutEditor(ID);
  useEffect(() => {
    latest.current = e;
  });
  return (
    <>
      <SaveStatus state={e.saveState} dirty={e.dirty} />
      <output data-testid="v">{e.version}</output>
    </>
  );
}
const status = () => document.querySelector("[data-save-state]");
const mount = async () => {
  const r = render(
    <MemoryRouter>
      <Probe />
    </MemoryRouter>,
  );
  await act(() => vi.advanceTimersByTimeAsync(0));
  return r;
};

beforeEach(async () => {
  vi.useFakeTimers();
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("useLayoutEditor", () => {
  it("loads, edits, autosaves and shows the state", async () => {
    const srv = mockServer({ version: 2, layout: doc });
    await mount();
    expect(ed()).toMatchObject({ loaded: "ready", version: 2, dirty: false, canUndo: false });
    act(() => void ed().apply(ops.setText("cover", "aaaaaa", { text: "Xin chào" })));
    expect(ed()).toMatchObject({ dirty: true, canUndo: true });
    expect(status()).toHaveTextContent("Saving...");
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS));
    expect(srv.s.saves).toHaveLength(1);
    expect(srv.s.saves[0]).toMatchObject({ base_version: 2 });
    expect(status()).toHaveTextContent("Saved");
    expect(screen.getByTestId("v")).toHaveTextContent("3");
    act(() => ed().undo());
    expect(ed().canRedo).toBe(true);
    act(() => ed().redo());
    expect(ed().doc.pages.cover?.items?.[0]).toMatchObject({ text: "Xin chào" });
  });

  it("warns on beforeunload while dirty and not otherwise", async () => {
    mockServer({ version: 1, layout: doc });
    await mount();
    const unload = () => {
      const e = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(e);
      return e.defaultPrevented;
    };
    expect(unload()).toBe(false);
    act(() => void ed().apply(ops.setText("cover", "aaaaaa", { text: "x" })));
    expect(unload()).toBe(true);
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS));
    expect(unload()).toBe(false);
  });

  it("a hidden page flushes at once", async () => {
    const srv = mockServer({ version: 1, layout: doc });
    await mount();
    act(() => void ed().apply(ops.setText("cover", "aaaaaa", { text: "x" })));
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(srv.s.saves).toHaveLength(1);
    vi.restoreAllMocks();
  });

  it("a conflict from another tab shows in the state and both choices work from the hook", async () => {
    const srv = mockServer({ version: 1, layout: doc });
    await mount();
    srv.s.otherTab(null);
    act(() => void ed().apply(ops.setText("cover", "aaaaaa", { text: "mine" })));
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS));
    expect(status()).toHaveTextContent("changed in another tab");
    await act(() => ed().keepMine());
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(srv.s.layout?.pages.cover?.items?.[0]).toMatchObject({ text: "mine" });
    expect(ed().saveState).toBe("saved");
  });

  it("an unmount with unsaved edits saves them and leaves no timer", async () => {
    const srv = mockServer({ version: 1, layout: doc });
    const r = await mount();
    act(() => void ed().apply(ops.setText("cover", "aaaaaa", { text: "x" })));
    r.unmount();
    await vi.advanceTimersByTimeAsync(0);
    expect(srv.s.saves).toHaveLength(1);
    expect(vi.getTimerCount()).toBe(0);
  });
});
