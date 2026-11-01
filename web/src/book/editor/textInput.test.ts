import { describe, expect, it } from "vitest";
import { MAX_TEXT, runeCount } from "./document";
import { capText, fixText, plainText } from "./textInput";

// T-161: the 500 limit is in code points (not UTF-16 units), counted after NFC and trim like the server; the paste normaliser keeps only what the server accepts.

describe("runeCount", () => {
  it.each([
    ["ASCII", "hello", 5],
    ["Vietnamese precomposed", "Việt Nam", 8],
    ["Vietnamese decomposed counts as its composed form", "Việt Nam".normalize("NFD"), 8],
    ["an emoji is one code point (two UTF-16 units)", "😀", 1],
    ["a flag is two regional indicators", "🇻🇳", 2],
    ["a family emoji joined with ZWJ is five", "👨‍👩‍👧", 5],
    ["edge white space is not counted", "  ab \n", 2],
  ])("%s", (_name, text, n) => {
    expect(runeCount(text)).toBe(n);
  });
  it("is not String.length", () => {
    expect("😀".repeat(600).length).toBe(1200);
    expect(runeCount("😀".repeat(600))).toBe(600);
  });
});

describe("capText", () => {
  it("leaves a text within the cap alone, even with 983 UTF-16 units", () => {
    const t = "😀".repeat(MAX_TEXT);
    expect(capText(t)).toBe(t);
    expect(t.length).toBe(1000);
  });
  it("cuts 600 emoji to exactly 500 code points and never splits a pair", () => {
    const out = capText("😀".repeat(600));
    expect([...out].length).toBe(500);
    expect(out.length).toBe(1000);
    expect(out).toBe("😀".repeat(500));
  });
  it("cuts ASCII at 500", () => {
    expect(capText("a".repeat(501))).toBe("a".repeat(500));
  });
  it("counts decomposed Vietnamese by its composed form: 800 decomposed letters are 400 and stay", () => {
    const t = "ế".normalize("NFD").repeat(400);
    expect([...t].length).toBe(1200);
    expect(capText(t)).toBe(t);
  });
  it("cuts decomposed letters only when the composed form is over 500", () => {
    const t = "ế".normalize("NFD").repeat(700); // 700 composed
    const out = capText(t);
    expect(runeCount(out)).toBe(500);
    expect(t.startsWith(out)).toBe(true);
  });
});

describe("plainText (paste)", () => {
  it("turns CRLF and CR into LF", () => {
    expect(plainText("a\r\nb\rc\nd")).toBe("a\nb\nc\nd");
  });
  it("turns a tab into a space and drops what the server refuses", () => {
    expect(plainText("a\tb​c‎d e\u0000f�g﻿h")).toBe("a bcdefgh");
  });
  it("keeps the joiner of an emoji sequence, the newline, and Vietnamese", () => {
    expect(plainText("👨‍👩\nViệt")).toBe("👨‍👩\nViệt");
  });
  it("drops a lone surrogate", () => {
    expect(plainText("a\ud800b")).toBe("ab");
  });
});

describe("fixText", () => {
  it("is plain and capped", () => {
    const out = fixText("😀\r\n".repeat(400));
    expect(out.includes("\r")).toBe(false);
    expect(runeCount(out)).toBeLessThanOrEqual(500);
  });
});
