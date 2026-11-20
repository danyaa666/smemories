import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FitText } from "./fit";

function fakeFonts(status: "loading" | "loaded") {
  const then = vi.fn();
  const fonts = {
    status,
    ready: { then },
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  };
  Object.defineProperty(document, "fonts", { value: fonts, configurable: true });
  return { then, fonts };
}

afterEach(() => Reflect.deleteProperty(document, "fonts"));

describe("FitText and web fonts (T-193)", () => {
  it("fits again when the fonts are ready while one is still loading", () => {
    const { then } = fakeFonts("loading");
    render(<FitText fitKey="a">hello</FitText>);
    expect(then).toHaveBeenCalledTimes(1);
  });

  it("does not wait for fonts that are all loaded: the first fit already used them", () => {
    const { then, fonts } = fakeFonts("loaded");
    render(<FitText fitKey="a">hello</FitText>);
    expect(then).not.toHaveBeenCalled();
    expect(fonts.addEventListener).toHaveBeenCalledWith("loadingdone", expect.any(Function)); // a font that loads later still re-fits it
  });
});
