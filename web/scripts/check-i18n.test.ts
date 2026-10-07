import { describe, expect, it } from "vitest";
import en from "../src/locales/en.json";
import viLocale from "../src/locales/vi.json";
import { compareLocales } from "./check-i18n.mjs";

describe("compareLocales", () => {
  it("passes on the real locale files", () => {
    expect(compareLocales(en, viLocale)).toEqual([]);
  });

  it("names a nested key that exists in only one locale", () => {
    const problems = compareLocales({ a: { b: "x", c: "y" } }, { a: { b: "x" } });
    expect(problems).toEqual(["a.c: missing in vi.json"]);
  });

  it("names a key missing from the first locale", () => {
    expect(compareLocales({ a: "x" }, { a: "x", extra: "y" })).toEqual([
      "extra: missing in en.json",
    ]);
  });

  it("flags empty and whitespace-only values", () => {
    const problems = compareLocales({ a: "x", b: "y" }, { a: "", b: "  " });
    expect(problems).toEqual([
      "a: empty or non-string value in vi.json",
      "b: empty or non-string value in vi.json",
    ]);
  });
});
