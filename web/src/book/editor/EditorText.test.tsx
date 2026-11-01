import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import type { LayoutDoc, LayoutItem } from "./document";
import { EditorShell } from "./EditorShell";
import { mockServer } from "./mockServer";
import { DEBOUNCE_MS } from "./store";

// T-161: text boxes in the editor shell: add, type in place, the property panel's text controls, the 40-item refusal, blank text removal, the clip warning.
// jsdom has no layout and no IME: the real keystrokes, the pixels and the composition through CDP are tools/e2e/editor-text.spec.mjs.

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const text = (id: string, over: Partial<LayoutItem> = {}): LayoutItem => ({
  id,
  t: "text",
  text: "Xin chào lớp 12A1",
  x: 100,
  w: 300,
  h: 100,
  cy: 700,
  ...over,
});
const LAYOUT: LayoutDoc = { v: 1, pages: { cover: { items: [text("textAA")] } } };
const book = (): BookData => ({
  design: null,
  layout: null,
  yearbook: {
    title: "Lớp 12A1",
    school_name: "THPT Chu Văn An",
    class_name: "12A1",
    graduation_year: 2026,
    motto: "",
    language: "vi",
    page_size: "A5",
    template_id: "memory-book",
    cover_media_id: null,
  },
  profile: {
    full_name: "Hồng",
    nickname: "",
    quote: "",
    hobbies: "",
    future_plans: "",
    photo_media_id: null,
  },
  notes: [],
  note_count: 0,
  approved_total: 0,
  truncated: false,
});
const mount = async (layout: LayoutDoc | null = LAYOUT) => {
  const srv = mockServer({ version: 2, layout });
  render(
    <MemoryRouter>
      <EditorShell bookId={BOOK} data={book()} safeMm={5} />
    </MemoryRouter>,
  );
  await screen.findByRole("toolbar");
  return srv;
};
const hit = (name: string | RegExp) => screen.getByRole("button", { name });
const items = () => [...document.querySelectorAll<HTMLElement>(".ed-pagebox [data-layout-item]")];
const printed = (id: string) =>
  document.querySelector<HTMLElement>(`.ed-pagebox [data-layout-item="${id}"] .bk-fit`)!;
const onPage = () =>
  screen.getByRole("textbox", { name: "Text of the box" }) as HTMLTextAreaElement;
const inPanel = () =>
  screen.getByRole("textbox", { name: "Text of the box (edit here)" }) as HTMLTextAreaElement;
const nativeSet = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
const type = (el: HTMLTextAreaElement, v: string) => {
  nativeSet.call(el, v);
  fireEvent.input(el);
};
const addButton = () =>
  within(screen.getByRole("toolbar")).getByRole("button", { name: "Add text" });

beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("add text", () => {
  it("adds a text box in the manifest's default style, centred, selected and open for typing, with the text selected", async () => {
    const srv = await mount(null);
    await userEvent.click(addButton());
    const box = items();
    expect(box).toHaveLength(1);
    const el = onPage();
    expect(el).toHaveFocus();
    expect(el.value).toBe("Your text");
    expect(el.selectionStart).toBe(0);
    expect(el.selectionEnd).toBe(9);
    expect(printed(box[0]!.dataset.layoutItem!)).toHaveTextContent("Your text");
    // the panel shows the style it was added with: body font, the middle of 8..96, the first colour token, left, not bold
    expect(screen.getByLabelText("Font")).toHaveValue("body");
    expect(screen.getByLabelText("Size")).toHaveValue(52);
    expect(screen.getByRole("radio", { name: "Ink" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("button", { name: "Left" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("checkbox", { name: "Bold" })).not.toBeChecked();
    await waitFor(() => expect(srv.s.saves.length).toBe(1), { timeout: DEBOUNCE_MS + 2000 });
    const saved = srv.s.saves[0]!.layout.pages.cover!.items![0]!;
    expect(saved).toMatchObject({
      t: "text",
      text: "Your text",
      x: 200,
      w: 600,
      cy: 500,
      size: 52,
    });
  });

  it("the empty-selection panel has the same button", async () => {
    await mount(null);
    const panel = screen.getByRole("complementary", { name: "Properties" });
    await userEvent.click(within(panel).getByRole("button", { name: "Add text" }));
    expect(items()).toHaveLength(1);
  });

  it("refuses the 41st box on a page with a clear message and adds nothing", async () => {
    const full: LayoutDoc = {
      v: 1,
      pages: {
        cover: {
          items: Array.from({ length: 40 }, (_, i) => text(`item${String(i).padStart(2, "0")}`)),
        },
      },
    };
    await mount(full);
    await userEvent.click(addButton());
    expect(screen.getByRole("alert")).toHaveTextContent("This page already has 40 items");
    expect(items()).toHaveLength(40);
    expect(screen.queryByRole("textbox", { name: "Text of the box" })).toBeNull();
  });

  it("refuses the 151st box in the book", async () => {
    const page = (n: number, p: string) =>
      Array.from({ length: n }, (_, i) => text(`${p}${String(i).padStart(3, "0")}`));
    await mount({
      v: 1,
      pages: {
        profile: { items: page(40, "pro") },
        notes: { items: page(40, "not") },
        back: { items: page(40, "bac") },
        cover: { items: page(30, "cov") },
      },
    });
    await userEvent.click(addButton());
    expect(screen.getByRole("alert")).toHaveTextContent("150");
    expect(items()).toHaveLength(30);
  });

  it("in Vietnamese, with the new text in Vietnamese", async () => {
    await i18n.changeLanguage("vi");
    await mount(null);
    await userEvent.click(
      within(screen.getByRole("toolbar")).getByRole("button", { name: "Thêm chữ" }),
    );
    expect(screen.getByRole("textbox", { name: "Chữ trong khung" })).toHaveValue("Chữ của bạn");
  });
});

describe("edit in place", () => {
  it("opens with Enter on a selected text, types, shows the typing on the page, and Escape keeps it and gives the focus back", async () => {
    await mount();
    hit(/Text: Xin chào/).focus();
    await userEvent.keyboard("{Enter}");
    expect(onPage()).toHaveFocus();
    type(onPage(), "Việt Nam");
    expect(printed("textAA")).toHaveTextContent("Việt Nam");
    expect(inPanel()).toHaveValue("Việt Nam"); // the panel field follows
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("textbox", { name: "Text of the box" })).toBeNull();
    await waitFor(() => expect(hit(/Text: Việt Nam/)).toHaveFocus());
    expect(printed("textAA")).toHaveTextContent("Việt Nam");
  });

  it("opens with a double click, and with a click on an item that was already selected, but not with the first click", async () => {
    await mount();
    await userEvent.click(hit(/Text: Xin chào/));
    expect(screen.queryByRole("textbox", { name: "Text of the box" })).toBeNull();
    await userEvent.click(hit(/Text: Xin chào/));
    expect(onPage()).toBeInTheDocument();
    cleanup();
    await mount();
    await userEvent.dblClick(hit(/Text: Xin chào/));
    expect(onPage()).toBeInTheDocument();
  });

  it("the printed text stays a div with React text: typed markup is text, never HTML", async () => {
    await mount();
    hit(/Text: Xin chào/).focus();
    await userEvent.keyboard("{Enter}");
    type(onPage(), "<img src=x onerror=alert(1)><b>bold</b>");
    expect(printed("textAA").tagName).toBe("DIV");
    expect(printed("textAA").querySelector("img, b")).toBeNull();
    expect(printed("textAA")).toHaveTextContent("<img src=x onerror=alert(1)><b>bold</b>");
    expect(printed("textAA")).toHaveStyle({ whiteSpace: "pre-wrap" });
  });

  it("one burst of typing is one undo step, and Ctrl/Cmd+Z inside the field is not the editor's undo", async () => {
    await mount();
    hit(/Text: Xin chào/).focus();
    await userEvent.keyboard("{Enter}");
    type(onPage(), "a");
    type(onPage(), "ab");
    type(onPage(), "abc");
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled(); // not committed yet
    fireEvent.keyDown(onPage(), { key: "z", ctrlKey: true });
    fireEvent.keyDown(onPage(), { key: "z", metaKey: true });
    expect(printed("textAA")).toHaveTextContent("abc");
    await userEvent.keyboard("{Escape}"); // commits the burst
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(printed("textAA")).toHaveTextContent("Xin chào lớp 12A1"); // the whole burst at once
    await userEvent.click(screen.getByRole("button", { name: "Redo" }));
    expect(printed("textAA")).toHaveTextContent("abc");
  });

  it("clicking outside closes edit mode and keeps the text", async () => {
    await mount();
    hit(/Text: Xin chào/).focus();
    await userEvent.keyboard("{Enter}");
    type(onPage(), "kept");
    await userEvent.click(document.querySelector(".ed-scroll")!);
    expect(screen.queryByRole("textbox", { name: "Text of the box" })).toBeNull();
    expect(printed("textAA")).toHaveTextContent("kept");
  });

  it("a click into the text field does not clear the selection", async () => {
    await mount();
    hit(/Text: Xin chào/).focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.click(onPage());
    expect(onPage()).toBeInTheDocument();
    expect(hit(/Text: Xin chào/)).toHaveAttribute("aria-pressed", "true");
  });

  it("closing with only white space removes the box, one undo step brings it back", async () => {
    await mount();
    hit(/Text: Xin chào/).focus();
    await userEvent.keyboard("{Enter}");
    type(onPage(), "  \n ");
    expect(items()).toHaveLength(1); // an empty box is allowed while editing
    await userEvent.keyboard("{Escape}");
    expect(items()).toHaveLength(0);
    expect(screen.getByRole("heading", { name: /Page: Cover/ })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(items()).toHaveLength(1);
    expect(printed("textAA")).toHaveTextContent(/^\s*$/);
  });

  it("cuts a paste of 600 emoji to 500 code points on the page and in the document", async () => {
    const srv = await mount();
    hit(/Text: Xin chào/).focus();
    await userEvent.keyboard("{Enter}");
    onPage().select();
    const e = new Event("paste", { bubbles: true, cancelable: true });
    Object.assign(e, {
      clipboardData: {
        getData: (t: string) => (t === "text/plain" ? "😀".repeat(600) : "<b>x</b>"),
      },
    });
    onPage().dispatchEvent(e);
    expect([...onPage().value].length).toBe(500);
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(srv.s.saves.length).toBe(1), { timeout: DEBOUNCE_MS + 2000 });
    expect([...srv.s.saves[0]!.layout.pages.cover!.items![0]!.text!].length).toBe(500);
  });
});

describe("text controls in the property panel", () => {
  const pick = async () => userEvent.click(hit(/Text: Xin chào/));

  it("the panel field is a normal multi-line field with the same cap, and edits a text that is behind another item", async () => {
    await mount();
    await pick();
    expect(inPanel()).toHaveValue("Xin chào lớp 12A1");
    expect(screen.getByText("17 of 500 characters")).toBeInTheDocument();
    type(inPanel(), "😀".repeat(600));
    expect([...inPanel().value].length).toBe(500);
    expect(printed("textAA")).toHaveTextContent("😀");
    expect(screen.getByText("500 of 500 characters")).toBeInTheDocument();
  });

  it("font, colour, alignment and bold are one history step each and reach the printed text", async () => {
    await mount();
    await pick();
    await userEvent.selectOptions(screen.getByLabelText("Font"), "display");
    expect(printed("textAA").style.fontFamily).toBe("var(--mb-disp)");
    await userEvent.click(screen.getByRole("radio", { name: "Accent" }));
    expect(printed("textAA").style.color).toBe("var(--mb-a1)");
    await userEvent.click(screen.getByRole("button", { name: "Centre" }));
    expect(printed("textAA").style.textAlign).toBe("center");
    await userEvent.click(screen.getByRole("checkbox", { name: "Bold" }));
    expect(printed("textAA").style.fontWeight).toBe("700");
    const undo = screen.getByRole("button", { name: "Undo" });
    for (const [prop, was] of [
      ["fontWeight", "400"],
      ["textAlign", "left"],
      ["color", "var(--mb-ink)"],
      ["fontFamily", "var(--mb-body)"],
    ] as const) {
      await userEvent.click(undo);
      expect(printed("textAA").style[prop]).toBe(was);
    }
  });

  it("the font list shows display names, the colours have names for screen readers", async () => {
    await mount();
    await pick();
    const font = screen.getByLabelText("Font");
    expect(
      within(font)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Heading font", "Body font"]);
    const names = within(screen.getByRole("radiogroup", { name: "Colour" }))
      .getAllByRole("radio")
      .map((r) => r.getAttribute("aria-label"));
    expect(names).toEqual(["Ink", "Soft ink", "Paper", "White", "Accent"]);
  });

  it("the size is a number and a slider within the manifest range; a value outside is refused with the range", async () => {
    await mount();
    await pick();
    const size = screen.getByLabelText("Size") as HTMLInputElement;
    const slider = screen.getByRole("slider", { name: "Size slider" });
    expect(slider).toHaveAttribute("min", "8");
    expect(slider).toHaveAttribute("max", "96");
    for (const bad of ["7", "97", "-3", ""]) {
      fireEvent.change(size, { target: { value: bad } });
      fireEvent.keyDown(size, { key: "Enter" });
      expect(screen.getByRole("alert")).toHaveTextContent("Between 8 and 96");
      expect(size).toHaveValue(24);
    }
    fireEvent.change(size, { target: { value: "40" } });
    fireEvent.keyDown(size, { key: "Enter" });
    expect(printed("textAA").style.fontSize).toBe("40px");
    fireEvent.change(slider, { target: { value: "60" } });
    fireEvent.keyUp(slider);
    expect(printed("textAA").style.fontSize).toBe("60px");
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(printed("textAA").style.fontSize).toBe("40px"); // the slider drag was one step
  });

  it("the swatches take the book's theme", async () => {
    await mount();
    await pick();
    expect(screen.getByRole("radiogroup", { name: "Colour" })).toHaveAttribute(
      "data-theme",
      "lavender",
    );
  });
});

describe("a text that is cut", () => {
  it("shows the warning beside the stage and in the panel while the fit rule gives up, not for other items", async () => {
    const h = vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(5000);
    try {
      await mount();
      expect(screen.queryByTestId("text-cut-panel")).toBeNull();
      await userEvent.click(hit(/Text: Xin chào/));
      expect(screen.getAllByText(/Text is cut/)).toHaveLength(2);
      expect(printed("textAA")).toHaveAttribute("data-fit", "overflow");
      await userEvent.click(hit("Cover title"));
      expect(screen.queryByText(/Text is cut/)).toBeNull();
    } finally {
      h.mockRestore();
    }
  });

  it("is quiet for a text that fits", async () => {
    await mount();
    await userEvent.click(hit(/Text: Xin chào/));
    expect(screen.queryByText(/Text is cut/)).toBeNull();
  });
});
