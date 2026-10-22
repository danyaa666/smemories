import { act, fireEvent, render, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { usePhotoPreload } from "../book/ready";
import { LIMIT, queueStats } from "./imageQueue";
import { QueuedImg } from "./QueuedImg";

const withSrc = (c: HTMLElement) =>
  [...c.querySelectorAll("img")].filter((i) => i.getAttribute("src"));

afterEach(() => {
  vi.useRealTimers();
});

// The browser is simulated: an image is "in flight" from the moment it has a src until a load event is fired on it.
describe("gallery image queue (T-116)", () => {
  it("never has more than LIMIT of 100 photos in flight, and loads all of them", () => {
    const { container } = render(
      <>
        {Array.from({ length: 100 }, (_, i) => (
          <QueuedImg key={i} src={`/api/media/get-content?id=${i}&variant=thumb`} alt="" />
        ))}
      </>,
    );
    const done = new Set<Element>();
    let peak = 0;
    for (let guard = 0; guard < 200 && done.size < 100; guard++) {
      const flying = withSrc(container).filter((i) => !done.has(i));
      peak = Math.max(peak, flying.length);
      expect(flying.length).toBeLessThanOrEqual(LIMIT);
      expect(queueStats().active).toBeLessThanOrEqual(LIMIT);
      flying.forEach((i) => {
        done.add(i);
        fireEvent.load(i);
      });
    }
    expect(peak).toBe(LIMIT);
    expect(done.size).toBe(100);
    expect(queueStats()).toEqual({ active: 0, waiting: 0 });
  });

  it("retries a failed image once, then reports the error", () => {
    vi.useFakeTimers();
    const onError = vi.fn();
    const { container } = render(<QueuedImg src="/a" alt="" onError={onError} />);
    const img = () => container.querySelector("img")!;
    fireEvent.error(img());
    expect(onError).not.toHaveBeenCalled();
    act(() => void vi.advanceTimersByTime(1000));
    expect(img().getAttribute("src")).toBe("/a");
    fireEvent.error(img());
    expect(onError).toHaveBeenCalledTimes(1);
    expect(queueStats().active).toBe(0);
  });

  it("gives the slot back when an image leaves the page", () => {
    const { unmount } = render(<QueuedImg src="/b" alt="" />);
    expect(queueStats().active).toBe(1);
    unmount();
    expect(queueStats().active).toBe(0);
  });
});

describe("print preload (T-116)", () => {
  it("shares the queue and settles only when every photo has loaded or failed", () => {
    const made: { onload?: () => void; onerror?: () => void }[] = [];
    vi.stubGlobal(
      "Image",
      class {
        onload?: () => void;
        onerror?: () => void;
        constructor() {
          made.push(this);
        }
        set src(_: string) {}
      },
    );
    const ids = Array.from({ length: 100 }, (_, i) => `p${i}`);
    const { result } = renderHook(() => usePhotoPreload(ids));
    let rounds = 0;
    let flown = 0;
    while (!result.current.settled && rounds++ < 50) {
      const batch = made.splice(0);
      expect(batch.length).toBeLessThanOrEqual(LIMIT);
      expect(queueStats().active).toBeLessThanOrEqual(LIMIT);
      expect(result.current.settled).toBe(false);
      act(() => batch.forEach((im) => (flown++ === 20 ? im.onerror : im.onload)?.()));
    }
    expect(result.current).toMatchObject({ total: 100, loaded: 99, failed: 1, settled: true });
    vi.unstubAllGlobals();
  });
});
