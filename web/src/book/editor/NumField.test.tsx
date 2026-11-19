import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import i18n from "../../i18n";
import { pageBox } from "../layout/geometry";
import { describe as say } from "./gestures/useGestures";
import { NumField, shown, typed } from "./NumField";

// T-188 F-10 and F-17: numbers are written and read the way the language writes them.
afterEach(async () => {
  cleanup();
  await act(() => i18n.changeLanguage("en"));
});

describe("shown and typed", () => {
  it("writes one decimal at most with the separator of the language", () => {
    expect(shown("en", 12.44)).toBe("12.4");
    expect(shown("vi", 12.44)).toBe("12,4");
    expect(shown("vi", 60)).toBe("60");
    expect(shown("vi", 1234.5)).toBe("1234,5");
  });
  it("reads either separator and refuses anything else", () => {
    expect(typed("12,5")).toBe(12.5);
    expect(typed(" 12.5 ")).toBe(12.5);
    expect(typed("1,2,3")).toBeNaN();
    expect(typed("abc")).toBeNaN();
  });
});

describe("NumField in Vietnamese", () => {
  const mount = (onCommit = vi.fn(() => true)) => {
    render(
      <NumField
        label="Left"
        value={12.4}
        step={0.1}
        min={0}
        max={100}
        disabled={false}
        onCommit={onCommit}
      />,
    );
    return {
      onCommit,
      input: screen.getByRole("spinbutton", { name: "Left" }) as HTMLInputElement,
    };
  };
  it("shows a decimal comma and accepts a comma or a point", async () => {
    await act(() => i18n.changeLanguage("vi"));
    const { onCommit, input } = mount();
    expect(input).toHaveValue("12,4");
    fireEvent.change(input, { target: { value: "20,5" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onCommit).toHaveBeenLastCalledWith(20.5);
    fireEvent.change(input, { target: { value: "30.5" } });
    fireEvent.blur(input);
    expect(onCommit).toHaveBeenLastCalledWith(30.5);
  });
  it("says the range with the comma and puts the old value back", async () => {
    await act(() => i18n.changeLanguage("vi"));
    const { onCommit, input } = mount();
    fireEvent.change(input, { target: { value: "200" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onCommit).not.toHaveBeenCalled();
    expect(input).toHaveValue("12,4");
    expect(screen.getByRole("alert")).toHaveTextContent("từ 0 đến 100");
  });
  it("steps with the arrow keys, clamped, and commits on Enter", () => {
    const { onCommit, input } = mount();
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(input).toHaveValue("12.5");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onCommit).toHaveBeenCalledWith(12.5);
  });
});

describe("announcements and the print hint in Vietnamese", () => {
  const pb = pageBox("A5");
  const rect = { left: pb.w * 0.1234, top: pb.h * 0.5, width: pb.w * 0.5, height: pb.h * 0.25 };
  it("say the places with a decimal comma", () => {
    const t = i18n.getFixedT("vi");
    expect(say(t, "move", rect, 0, pb)).toContain(
      "cách mép trái 12,3 phần trăm, cách mép trên 50 phần trăm",
    );
    expect(say(i18n.getFixedT("en"), "resize", rect, 0, pb)).toContain(
      "Size 50 percent of the page width, 25 percent",
    );
  });
  it("the print hint comes from the locale file", () => {
    const size = { size: "Letter", w: 215.9, h: 279.4 };
    expect(i18n.getFixedT("vi")("editor.printHint", size)).toContain("Letter (215,9 × 279,4 mm)");
    expect(i18n.getFixedT("en")("editor.printHint", size)).toContain("Letter (215.9 × 279.4 mm)");
  });
});
