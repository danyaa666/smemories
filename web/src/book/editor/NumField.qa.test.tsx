import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import i18n from "../../i18n";
import { NumField } from "./NumField";

// T-188 QA: the spinbutton contract (valuenow/min/max), refused input moves nothing, arrows clamp.
afterEach(async () => {
  cleanup();
  await act(() => i18n.changeLanguage("en"));
});

const mount = (onCommit = vi.fn(() => true), value = 12.4) => {
  render(
    <NumField
      label="Width"
      value={value}
      step={0.1}
      min={1}
      max={200}
      disabled={false}
      onCommit={onCommit}
    />,
  );
  return { onCommit, input: screen.getByRole("spinbutton", { name: "Width" }) as HTMLInputElement };
};

describe("NumField spinbutton contract", () => {
  it("exposes valuenow, valuemin and valuemax", () => {
    const { input } = mount();
    expect(input).toHaveAttribute("aria-valuenow", "12.4");
    expect(input).toHaveAttribute("aria-valuemin", "1");
    expect(input).toHaveAttribute("aria-valuemax", "200");
  });
  it.each(["abc", "1,2,3", "", "  ", "0", "201", "Infinity"])(
    "refuses %j: nothing is committed and the old value is back",
    (text) => {
      const { onCommit, input } = mount();
      fireEvent.change(input, { target: { value: text } });
      fireEvent.keyDown(input, { key: "Enter" });
      expect(onCommit).not.toHaveBeenCalled();
      expect(input).toHaveValue("12.4");
    },
  );
  it("ArrowDown and ArrowUp clamp at the limits", () => {
    const { input } = mount(
      vi.fn(() => true),
      1,
    );
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(input).toHaveValue("1");
    fireEvent.change(input, { target: { value: "200" } });
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(input).toHaveValue("200");
  });
  it("in Vietnamese the stepped value is written with a comma and read back", async () => {
    await act(() => i18n.changeLanguage("vi"));
    const { onCommit, input } = mount();
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(input).toHaveValue("12,5");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onCommit).toHaveBeenCalledWith(12.5);
  });
});
