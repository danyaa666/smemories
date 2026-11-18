import type { TFunction } from "i18next";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pageBox } from "../../layout/geometry";
import type { Target } from "../targets";
import { SAY_MS, useGestures, type GestureOpts } from "./useGestures";

// T-182 (AC3): what a run of arrow keys says to a screen reader is bounded: the first step at once, the rest collapsed into one message with the final place.
const t = ((k: string, o?: Record<string, unknown>) =>
  `${k} ${JSON.stringify(o ?? {})}`) as unknown as TFunction;
const target: Target = {
  id: "cover.title",
  kind: "cover",
  rect: { left: 100, top: 100, width: 300, height: 100 },
  r: 0,
  hidden: false,
};

function setup() {
  const announce = vi.fn();
  const store = {
    begin: vi.fn(),
    preview: vi.fn(),
    commit: vi.fn(),
    cancel: vi.fn(),
    apply: vi.fn(),
    select: vi.fn(),
  };
  const opts: GestureOpts = {
    targets: [target],
    pb: pageBox("A5"),
    safe: 0,
    selection: [target.id],
    readOnly: false,
    pageEl: { current: null },
    store,
    announce,
    t,
  };
  const { result, unmount } = renderHook(() => useGestures(opts));
  const press = (key: string, extra = {}) =>
    result.current.api.key(
      {
        key,
        shiftKey: false,
        altKey: false,
        ctrlKey: false,
        metaKey: false,
        preventDefault: () => {},
        ...extra,
      } as never,
      target,
    );
  return { announce, store, press, api: result.current.api, unmount };
}

describe("announcements of key steps", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("20 arrow presses in a row make two messages: the first at once, the last with the final place", () => {
    const { announce, press, store } = setup();
    for (let i = 0; i < 20; i++) {
      act(() => press("ArrowRight"));
      vi.advanceTimersByTime(10);
    }
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce.mock.calls[0]![0]).toContain('\\"n\\":1'); // "Moved right 1." of the first press
    vi.advanceTimersByTime(SAY_MS);
    expect(announce).toHaveBeenCalledTimes(2);
    const x = (n: number) => Number(/"x":"([\d.]+)"/.exec(announce.mock.calls[n]![0])![1]);
    expect(x(1)).toBeCloseTo(x(0) + (19 / pageBox("A5").w) * 100, 0); // the 20th press' place: 19 px further than the first
    vi.advanceTimersByTime(5000);
    expect(announce).toHaveBeenCalledTimes(2); // nothing else, no repeat
    expect(store.commit).toHaveBeenCalledTimes(1); // and one history step
  });

  it("a press after a pause says its place at once", () => {
    const { announce, press } = setup();
    act(() => press("ArrowDown"));
    vi.advanceTimersByTime(SAY_MS + 1);
    act(() => press("ArrowDown"));
    expect(announce).toHaveBeenCalledTimes(2);
  });

  it("an undo right after the keys drops the message that was waiting", () => {
    const { announce, press } = setup();
    act(() => press("ArrowRight"));
    act(() => press("ArrowRight"));
    act(() => press("z", { ctrlKey: true }));
    vi.advanceTimersByTime(5000);
    expect(announce).toHaveBeenCalledTimes(1);
  });

  it("leaving the item drops the message that was still waiting (the new selection says its own)", () => {
    const { announce, press, api } = setup();
    act(() => press("ArrowRight"));
    act(() => press("ArrowRight"));
    api.blur();
    vi.advanceTimersByTime(5000);
    expect(announce).toHaveBeenCalledTimes(1);
  });
});
