import { describe, expect, it, vi } from "vitest";
import i18n, { detectLanguage, setLanguage } from "./i18n";

function browserLanguage(lang: string) {
  vi.spyOn(window.navigator, "language", "get").mockReturnValue(lang);
}

describe("detectLanguage", () => {
  it.each([
    ["vi-VN", "vi"],
    ["vi", "vi"],
    ["en-US", "en"],
    ["fr-FR", "en"],
  ])("maps browser language %s to %s", (browser, expected) => {
    browserLanguage(browser);
    expect(detectLanguage()).toBe(expected);
  });

  it("prefers a stored choice over the browser language", () => {
    browserLanguage("vi-VN");
    window.localStorage.setItem("smemories.lang", "en");
    expect(detectLanguage()).toBe("en");
  });

  it("ignores an invalid stored value", () => {
    browserLanguage("vi-VN");
    window.localStorage.setItem("smemories.lang", "xx");
    expect(detectLanguage()).toBe("vi");
  });

  it("falls back to the browser language when storage throws", () => {
    browserLanguage("vi-VN");
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(detectLanguage()).toBe("vi");
  });
});

describe("setLanguage", () => {
  it("persists the choice, switches the language and updates <html lang>", async () => {
    await setLanguage("vi");
    expect(window.localStorage.getItem("smemories.lang")).toBe("vi");
    expect(i18n.t("language.label")).toBe("Ngôn ngữ");
    expect(document.documentElement.lang).toBe("vi");
  });

  it("still switches the language when storage throws", async () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    await setLanguage("vi");
    expect(document.documentElement.lang).toBe("vi");
  });
});
