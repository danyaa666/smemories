// QA (T-158, AC4): text the server refuses (internal/textx CleanMultiline: Cc except newline, Cf except ZWJ, Zl, Zp, U+FFFD) must be refused by the
// client mirror too, and the length is counted after NFC and trim, as the server does. Seen live: the editor ended in `invalid` on a pasted U+200B.
import { describe, expect, it } from "vitest";
import { addItem, emptyDoc, setText, type LayoutItem } from "./document";

const item = (text: string): LayoutItem => ({
  id: "aaaaaa",
  t: "text",
  text,
  x: 100,
  w: 400,
  h: 120,
  cy: 500,
});

describe("client mirror of the server's text rule", () => {
  it.each([
    ["zero-width space U+200B", "a​b"],
    ["left-to-right mark U+200E", "a‎b"],
    ["byte order mark U+FEFF", "﻿ab"],
    ["line separator U+2028", "a b"],
    ["paragraph separator U+2029", "a b"],
    ["replacement character U+FFFD", "a�b"],
  ])("refuses %s", (_name, text) => {
    expect(addItem("cover", item(text))(emptyDoc()).refused).toBe("bad_text");
    const doc = addItem("cover", item("ok"))(emptyDoc()).doc;
    expect(setText("cover", "aaaaaa", { text })(doc).refused).toBe("bad_text");
  });

  it("keeps the joiner U+200D (emoji sequences) and the newline", () => {
    expect(addItem("cover", item("👨‍👩\nx"))(emptyDoc()).refused).toBeUndefined();
  });

  it("counts code points after NFC and trim, like the server", () => {
    const decomposed = "ế".normalize("NFD").repeat(300); // 600 code points, 300 after NFC: the server stores it
    expect(addItem("cover", item(decomposed))(emptyDoc()).refused).toBeUndefined();
    expect(addItem("cover", item("a".repeat(500) + "   "))(emptyDoc()).refused).toBeUndefined();
  });
});
