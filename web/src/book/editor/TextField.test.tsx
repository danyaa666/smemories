import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useMemo, useRef, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { addItem, emptyDoc, runeCount, type LayoutDoc, type Op } from "./document";
import { TextField, TYPING_BURST_MS } from "./TextField";

// T-161: the text field. The value is never rewritten while a composition runs, the cap is applied at compositionend and while typing outside one
// (with the caret kept), paste is plain text, and a burst of typing is one gesture of the store. jsdom has no IME: the events are fired by hand
// in the order the browser fires them (the real path through CDP `Input.imeSetComposition` is tools/e2e/editor-text.spec.mjs).

const docOf = (text: string): LayoutDoc =>
  addItem("cover", { id: "aaaaaa", t: "text", text, x: 100, w: 400, h: 120, cy: 500 })(emptyDoc())
    .doc;
const textOf = (d: LayoutDoc) => d.pages.cover?.items?.[0]?.text ?? "";

const spies = { preview: vi.fn(), commit: vi.fn() };
function Harness({ initial, onRefused }: { initial: string; onRefused?: (r?: string) => void }) {
  const [doc, setDoc] = useState(() => docOf(initial));
  const cur = useRef(doc);
  const store = useMemo(
    () => ({
      preview: (op: Op) => {
        spies.preview();
        const r = op(cur.current);
        if (r.refused) return r.refused;
        cur.current = r.doc;
        setDoc(r.doc);
        return undefined;
      },
      commit: () => spies.commit(),
    }),
    [],
  );
  return (
    <TextField
      kind="cover"
      id="aaaaaa"
      value={textOf(doc)}
      store={store}
      label="T"
      onRefused={onRefused}
    />
  );
}
const field = () => screen.getByRole("textbox", { name: "T" }) as HTMLTextAreaElement;
const nativeSet = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
/** The browser changed the value and left the caret at `caret`: fires the input event React listens to. */
const typed = (el: HTMLTextAreaElement, value: string, caret = value.length) => {
  nativeSet.call(el, value);
  el.setSelectionRange(caret, caret);
  fireEvent.input(el);
};
const flush = () => act(async () => void (await Promise.resolve()));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  spies.preview.mockClear();
  spies.commit.mockClear();
});

describe("composition", () => {
  it("never rewrites the value, or sends it to the document, while a composition runs", () => {
    render(<Harness initial="" />);
    const el = field();
    fireEvent.compositionStart(el, { data: "" });
    for (const v of ["V", "Vi", "Viê", "Việ", "Việt"]) {
      typed(el, v);
      fireEvent.compositionUpdate(el, { data: v });
      expect(el.value).toBe(v);
    }
    expect(spies.preview).not.toHaveBeenCalled();
    fireEvent.compositionEnd(el, { data: "Việt" });
    expect(el.value).toBe("Việt");
    expect(spies.preview).toHaveBeenCalledTimes(1);
  });

  it("does not apply the cap in the middle of a composition: 499 runes + 3 composed = 502, cut to 500 at compositionend", async () => {
    render(<Harness initial={"a".repeat(499)} />);
    const el = field();
    fireEvent.compositionStart(el);
    typed(el, "a".repeat(499) + "đ");
    typed(el, "a".repeat(499) + "đư");
    typed(el, "a".repeat(499) + "đườ");
    expect(runeCount(el.value)).toBe(502);
    fireEvent.compositionEnd(el);
    await flush();
    expect(runeCount(el.value)).toBe(500);
    expect(el.value).toBe("a".repeat(499) + "đ");
  });

  it("when the browser fires compositionend before the last input (Safari), the value is still cut", () => {
    render(<Harness initial="" />);
    const el = field();
    fireEvent.compositionStart(el);
    fireEvent.compositionEnd(el);
    typed(el, "😀".repeat(600));
    expect(runeCount(el.value)).toBe(500);
  });
});

describe("the cap outside a composition", () => {
  it("cuts a value over 500 code points at once and puts the caret back where the user was typing", async () => {
    render(<Harness initial={"a".repeat(500)} />);
    const el = field();
    typed(el, "a".repeat(100) + "b" + "a".repeat(400), 101);
    await flush();
    expect(el.value).toBe("a".repeat(100) + "b" + "a".repeat(399));
    expect(runeCount(el.value)).toBe(500);
    expect(el.selectionStart).toBe(101);
    expect(el.selectionEnd).toBe(101);
  });

  it("does not use maxlength", () => {
    render(<Harness initial="" />);
    expect(field().hasAttribute("maxlength")).toBe(false);
  });

  it("counts a flag emoji as two", () => {
    render(<Harness initial="" />);
    typed(field(), "🇻🇳".repeat(300));
    expect(runeCount(field().value)).toBe(500);
  });
});

const paste = (el: HTMLElement, data: Record<string, string>) => {
  const e = new Event("paste", { bubbles: true, cancelable: true });
  Object.assign(e, { clipboardData: { getData: (t: string) => data[t] ?? "" } });
  act(() => void el.dispatchEvent(e));
  return e;
};

describe("paste", () => {
  it("takes text/plain only: the HTML on the clipboard inserts no markup", () => {
    render(<Harness initial="" />);
    const el = field();
    const e = paste(el, {
      "text/plain": "bold text",
      "text/html": '<b style="font-size:99px">bold</b> <a href="x">link</a>',
    });
    expect(e.defaultPrevented).toBe(true);
    expect(el.value).toBe("bold text");
  });

  it("an HTML-only clipboard inserts nothing", () => {
    render(<Harness initial="x" />);
    paste(field(), { "text/html": "<b>bold</b>" });
    expect(field().value).toBe("x");
  });

  it("turns CRLF and CR into LF and inserts at the selection", () => {
    render(<Harness initial="ab" />);
    const el = field();
    el.setSelectionRange(1, 1);
    paste(el, { "text/plain": "1\r\n2\r3" });
    expect(el.value).toBe("a1\n2\n3b");
  });

  it("600 pasted emoji give exactly 500 code points", () => {
    render(<Harness initial="" />);
    paste(field(), { "text/plain": "😀".repeat(600) });
    expect([...field().value].length).toBe(500);
  });

  it("a pasted tab and zero-width space do not reach the document (the server would refuse them)", () => {
    const refused = vi.fn();
    render(<Harness initial="" onRefused={refused} />);
    paste(field(), { "text/plain": "a\tb​c" });
    expect(field().value).toBe("a bc");
    expect(refused).toHaveBeenLastCalledWith(undefined);
  });

  it("refuses a drop of rich text and inserts its plain text", () => {
    render(<Harness initial="" />);
    const el = field();
    const e = new Event("drop", { bubbles: true, cancelable: true });
    Object.assign(e, {
      dataTransfer: {
        types: ["text/html", "text/plain"],
        files: [],
        getData: (t: string) => (t === "text/plain" ? "dropped" : "<b>dropped</b>"),
      },
    });
    act(() => void el.dispatchEvent(e));
    expect(e.defaultPrevented).toBe(true);
    expect(el.value).toBe("dropped");
  });
});

describe("history", () => {
  it("typing is one gesture: nothing is committed per key, one commit after a pause", () => {
    vi.useFakeTimers();
    render(<Harness initial="" />);
    const el = field();
    typed(el, "a");
    typed(el, "ab");
    typed(el, "abc");
    expect(spies.preview).toHaveBeenCalledTimes(3);
    expect(spies.commit).not.toHaveBeenCalled();
    act(() => void vi.advanceTimersByTime(TYPING_BURST_MS));
    expect(spies.commit).toHaveBeenCalledTimes(1);
  });

  it("is committed on blur, and Escape commits and tells the screen", () => {
    const onEscape = vi.fn();
    const store = { preview: () => undefined, commit: vi.fn() };
    render(
      <TextField kind="cover" id="aaaaaa" value="" store={store} label="T" onEscape={onEscape} />,
    );
    const el = field();
    typed(el, "x");
    fireEvent.blur(el);
    expect(store.commit).toHaveBeenCalledTimes(1);
    typed(el, "xy");
    fireEvent.keyDown(el, { key: "Escape" });
    expect(store.commit).toHaveBeenCalledTimes(2);
    expect(onEscape).toHaveBeenCalledTimes(1);
  });

  it("commits what is open when the field goes away", () => {
    const { unmount } = render(<Harness initial="" />);
    typed(field(), "a");
    unmount();
    expect(spies.commit).toHaveBeenCalledTimes(1);
  });

  it("shows a text that changed in the document (the other field, an undo)", () => {
    const store = { preview: () => undefined, commit: vi.fn() };
    const { rerender } = render(
      <TextField kind="cover" id="a" value="one" store={store} label="T" />,
    );
    rerender(<TextField kind="cover" id="a" value="two" store={store} label="T" />);
    expect(field().value).toBe("two");
  });
});

describe("the field", () => {
  it("is a text field with the given accessible name and no spell-check squiggles", () => {
    render(<Harness initial="x" />);
    expect(field().tagName).toBe("TEXTAREA");
    expect(field().getAttribute("spellcheck")).toBe("false");
  });
});
