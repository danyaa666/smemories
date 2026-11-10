import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import type { LayoutDoc } from "./document";
import { EditorBody } from "./EditorBody";
import { mockServer } from "./mockServer";
import { PhoneStage } from "./PhoneStage";
import { Stage } from "./Stage";
import { DEBOUNCE_MS } from "./store";

// T-181: the phone editor (below 768 px): the same component and store with the phone chrome. jsdom has no layout, no touch and no on-screen keyboard: the
// real touch run (375x812, 414x896, landscape) is tools/e2e/editor-phone.spec.mjs and the keyboards are owner manual checks (docs/editor-phone.md).

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const LAYOUT: LayoutDoc = {
  v: 1,
  pages: {
    cover: {
      items: [
        { id: "textAA", t: "text", text: "Xin chào lớp 12A1", x: 100, w: 300, h: 100, cy: 700 },
        { id: "starBB", t: "asset", asset: "sticker.gold-star", x: 600, w: 150, h: 150, cy: 300 },
      ],
    },
  },
};
const data = (): BookData => ({
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

const Phone = ({ phone }: { phone: boolean }) => (
  <MemoryRouter>
    <EditorBody
      bookId={BOOK}
      data={data()}
      safeMm={5}
      phone={phone}
      Stage={phone ? PhoneStage : Stage}
    />
  </MemoryRouter>
);
const mount = async (layout: LayoutDoc | null = LAYOUT, phone = true) => {
  const srv = mockServer({ version: 2, layout });
  const r = render(<Phone phone={phone} />);
  await screen.findByRole("toolbar");
  return { srv, ...r };
};
const hit = (name: string | RegExp) => screen.getByRole("button", { name });
const sheet = () => screen.queryByRole("region", { name: "Properties" });
const live = () => document.querySelector(".visually-hidden[role=status]")?.textContent;

beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("the phone shell", () => {
  it("draws the page read-only: no handles, no strip, no desktop toolbar, a pager and an undo/redo bar", async () => {
    await mount();
    expect(document.querySelector(".ed-phone")).not.toBeNull();
    expect(document.querySelector(".ed-handle")).toBeNull();
    expect(document.querySelector(".ed-rotate")).toBeNull();
    expect(screen.queryByRole("navigation", { name: "Pages" })).toBeNull(); // no page strip
    expect(screen.queryByRole("button", { name: "Zoom in" })).toBeNull();
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Redo" })).toBeDisabled();
    expect(screen.getByTestId("page-position")).toHaveTextContent("Page 1 of 3: Cover");
    expect(screen.getByRole("button", { name: "Done" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Print or save as PDF" })).toBeInTheDocument();
    expect(sheet()).toBeNull();
  });

  it("a tap selects (and opens the sheet) without changing the document; a tap beside the items clears", async () => {
    const { srv } = await mount();
    await userEvent.click(hit("Cover title"));
    expect(hit("Cover title")).toHaveAttribute("aria-pressed", "true");
    expect(sheet()).not.toBeNull();
    expect(within(sheet()!).getByRole("heading", { name: "Cover title" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled(); // a view-only tap is no history step
    await new Promise((r) => setTimeout(r, DEBOUNCE_MS + 100));
    expect(srv.s.saves.length).toBe(0);
    fireEvent.click(document.querySelector(".ed-scroll")!);
    expect(hit("Cover title")).toHaveAttribute("aria-pressed", "false");
    expect(sheet()).toBeNull();
  });

  it("starts no gesture: a pointer press on an item or a drag moves nothing", async () => {
    const { srv } = await mount();
    const el = hit("Cover title");
    fireEvent.pointerDown(el, { pointerId: 1, clientX: 10, clientY: 10, pointerType: "touch" });
    fireEvent.pointerMove(el, { pointerId: 1, clientX: 90, clientY: 90, pointerType: "touch" });
    fireEvent.pointerUp(el, { pointerId: 1, clientX: 90, clientY: 90, pointerType: "touch" });
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(srv.s.saves.length).toBe(0);
    expect(document.querySelector(".ed-guide")).toBeNull();
  });

  it("nudges with the four arrows and resizes with the size buttons; each tap is one history step that is saved", async () => {
    const { srv } = await mount();
    await userEvent.click(hit(/Text: Xin chào/));
    const box = () => srv.s.saves.at(-1)!.layout.pages.cover!.items![0]!;
    await userEvent.click(within(sheet()!).getByRole("button", { name: "Move right" }));
    await userEvent.click(within(sheet()!).getByRole("button", { name: "Move far down" }));
    expect(live()).toBe("Moved far down");
    await userEvent.click(within(sheet()!).getByRole("button", { name: "Wider" }));
    await waitFor(() => expect(srv.s.saves.length).toBe(1), { timeout: DEBOUNCE_MS + 2000 });
    expect(box().x).toBeGreaterThan(100); // 5 design px right
    expect(box().w).toBeGreaterThan(300); // 10 design px wider
    expect(box().cy).toBeGreaterThan(700);
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    await waitFor(() => expect(srv.s.saves.length).toBe(2), { timeout: DEBOUNCE_MS + 2000 });
    expect(box()).toMatchObject({ x: 100, w: 300, cy: 700 });
    expect(screen.getByRole("button", { name: "Redo" })).toBeEnabled();
  });

  it("edits the text in the sheet's textarea, composition included: nothing is written until the composition ends", async () => {
    const { srv } = await mount();
    await userEvent.click(hit(/Text: Xin chào/));
    const ta = within(sheet()!).getByLabelText(
      "Text of the box (edit here)",
    ) as HTMLTextAreaElement;
    expect(ta.value).toBe("Xin chào lớp 12A1");
    expect(getComputedStyle(ta).display).not.toBe("none");
    fireEvent.compositionStart(ta, { data: "" });
    for (const v of ["Việ", "Việt"]) {
      fireEvent.change(ta, { target: { value: v } });
      fireEvent.compositionUpdate(ta, { data: v });
    }
    await new Promise((r) => setTimeout(r, DEBOUNCE_MS + 100));
    expect(srv.s.saves.length).toBe(0);
    fireEvent.compositionEnd(ta, { data: "Việt" });
    await waitFor(() => expect(srv.s.saves.length).toBe(1), { timeout: DEBOUNCE_MS + 2000 });
    expect(srv.s.saves[0]!.layout.pages.cover!.items![0]!.text).toBe("Việt");
    expect(within(sheet()!).getByText(/500/)).toBeInTheDocument(); // the same 500 code point cap as on the desktop
  });

  it("asks once before a delete and the delete is one undoable step", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    await userEvent.click(within(sheet()!).getByRole("button", { name: "Delete" }));
    const d = screen.getByRole("dialog", { name: "Delete this item?" });
    await userEvent.click(within(d).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByRole("button", { name: "Sticker" })).not.toBeNull();
    await userEvent.click(within(sheet()!).getByRole("button", { name: "Delete" }));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }),
    );
    expect(screen.queryByRole("button", { name: "Sticker" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(screen.queryByRole("button", { name: "Sticker" })).not.toBeNull();
  });

  it("lists the page's items in the Layers sheet and selects from it", async () => {
    await mount();
    await userEvent.click(screen.getByRole("button", { name: "Layers" }));
    const layers = screen.getByRole("region", { name: "Layers" });
    const names = within(layers)
      .getAllByRole("button")
      .map((b) => b.textContent)
      .filter((n) => n !== "Close");
    expect(names).toContain("Sticker"); // topmost first: the sticker was added last
    expect(names.indexOf("Sticker")).toBeLessThan(names.indexOf("Text: Xin chào lớp 12A1"));
    await userEvent.click(within(layers).getByRole("button", { name: "Sticker" }));
    expect(hit("Sticker")).toHaveAttribute("aria-pressed", "true");
    expect(sheet()).not.toBeNull();
  });

  it("adds a text box and a sticker from the Add sheet, selected and ready in the property sheet", async () => {
    const { srv } = await mount();
    await userEvent.click(screen.getByRole("button", { name: "Add" }));
    const add = screen.getByRole("region", { name: "Add" });
    await userEvent.click(within(add).getByRole("button", { name: "Add text" }));
    expect(sheet()).not.toBeNull();
    expect(within(sheet()!).getByLabelText("Text of the box (edit here)")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Add" }));
    const stickers = within(screen.getByRole("region", { name: "Add" })).getAllByRole("button");
    const first = stickers.find((b) =>
      /sticker|star|heart|doodle/i.test(b.getAttribute("aria-label") ?? ""),
    );
    expect(first).toBeDefined();
    await userEvent.click(first!);
    await waitFor(() => expect(srv.s.saves.length).toBeGreaterThan(0), {
      timeout: DEBOUNCE_MS + 2000,
    });
    expect(srv.s.saves.at(-1)!.layout.pages.cover!.items!.length).toBe(4);
  });

  it("pages with the pager and jumps from the list sheet; the selection is cleared on the new page", async () => {
    await mount();
    await userEvent.click(hit("Cover title"));
    await userEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(screen.getByTestId("page-position")).toHaveTextContent("Page 2 of 3: Profile");
    expect(document.querySelector(".ed-pagebox .bk-sheet")?.getAttribute("data-page")).toBe(
      "profile",
    );
    expect(document.querySelector(".ed-box.is-selected")).toBeNull();
    await userEvent.click(screen.getByTestId("page-position"));
    const list = screen.getByRole("region", { name: "Pages" });
    await userEvent.click(within(list).getByRole("button", { name: "Page 3 of 3: Back cover" }));
    expect(screen.getByTestId("page-position")).toHaveTextContent("Page 3 of 3: Back cover");
    expect(screen.queryByRole("region", { name: "Pages" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Previous page" }));
    expect(screen.getByTestId("page-position")).toHaveTextContent("Page 2 of 3: Profile");
  });

  it("opens History and Templates as full-height dialogs: focus goes in, Escape closes, focus returns to the button", async () => {
    await mount();
    for (const name of ["History", "Templates"]) {
      const opener = screen.getByRole("button", { name });
      await userEvent.click(opener);
      const d = screen.getByRole("dialog", { name: /History|Cover|template/i });
      expect(d).toHaveAttribute("aria-modal", "true");
      await waitFor(() => expect(d.contains(document.activeElement)).toBe(true));
      await userEvent.keyboard("{Escape}");
      expect(screen.queryByRole("dialog")).toBeNull();
      await waitFor(() => expect(opener).toHaveFocus());
    }
  });

  it("keeps Tab inside the dialog", async () => {
    await mount();
    await userEvent.click(screen.getByRole("button", { name: "History" }));
    const d = screen.getByRole("dialog");
    const items = [...d.querySelectorAll<HTMLElement>("button:not([disabled])")];
    items[items.length - 1]!.focus();
    await userEvent.tab();
    expect(d.contains(document.activeElement)).toBe(true);
    await userEvent.tab({ shift: true });
    await userEvent.tab({ shift: true });
    expect(d.contains(document.activeElement)).toBe(true);
  });

  it("the sheet is a labelled region that follows the visual viewport (the keyboard never covers it)", async () => {
    const listeners: Record<string, () => void> = {};
    const vv = {
      height: 400,
      offsetTop: 0,
      addEventListener: (k: string, f: () => void) => void (listeners[k] = f),
      removeEventListener: () => {},
    };
    vi.stubGlobal("visualViewport", vv);
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
    await mount();
    await userEvent.click(hit("Cover title"));
    const bottom = document.querySelector<HTMLElement>(".ed-bottom")!;
    expect(sheet()).toHaveAccessibleName("Properties");
    expect(bottom.style.getPropertyValue("--kb")).toBe("400px"); // 800 - 400: the keyboard's height
    vv.height = 800;
    listeners["resize"]!();
    await waitFor(() => expect(bottom.style.getPropertyValue("--kb")).toBe("0px"));
    vi.unstubAllGlobals();
  });

  it("every phone button has an accessible name", async () => {
    await mount();
    await userEvent.click(hit("Cover title"));
    for (const b of document.querySelectorAll(".ed-phone button, .ed-phone a")) {
      expect(
        (b.getAttribute("aria-label") ?? b.textContent ?? "").trim(),
        b.outerHTML.slice(0, 80),
      ).not.toBe("");
    }
  });
});

describe("crossing 768 px", () => {
  it("keeps the selection, the open sheet and the typed text when the shell swaps, both ways", async () => {
    const { srv, rerender } = await mount();
    await userEvent.click(hit(/Text: Xin chào/));
    const ta = within(sheet()!).getByLabelText(
      "Text of the box (edit here)",
    ) as HTMLTextAreaElement;
    await userEvent.clear(ta);
    await userEvent.type(ta, "Chào");
    await waitFor(() => expect(srv.s.saves.length).toBeGreaterThan(0), {
      timeout: DEBOUNCE_MS + 2000,
    });
    await userEvent.click(screen.getByRole("button", { name: "Layers" })); // another sheet is open when the width changes
    rerender(<Phone phone={false} />);
    await screen.findByRole("navigation", { name: "Pages" }); // the desktop shell
    expect(document.querySelector(".ed-phone")).toBeNull();
    expect(document.querySelector(".ed-handle")).not.toBeNull(); // the selected item has its handles again
    const onPage = () =>
      within(document.querySelector<HTMLElement>(".ed-overlay")!).getByRole("button", {
        name: /Text: Chào/,
      });
    expect(onPage()).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByLabelText("Text of the box (edit here)")).toHaveValue("Chào");
    rerender(<Phone phone />);
    await screen.findByRole("region", { name: "Layers" }); // the open sheet is still open
    expect(onPage()).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled(); // the history is the same store's
    expect(srv.s.saves.length).toBeGreaterThan(0);
  });
});

describe("what the stylesheet promises on touch", () => {
  const css = readFileSync(resolve(process.cwd(), "src/styles/editor.css"), "utf8");
  it("the 44 px rule of the coarse-pointer block covers the Templates dialogs (QA of T-176)", () => {
    const block = css.slice(css.indexOf("@media (pointer: coarse)"));
    expect(block.slice(0, block.indexOf("\n}\n"))).toMatch(
      /\.ed-panel \.dialog button[\s\S]*min-height: 44px/,
    );
  });
  it("phone buttons are at least 44 px and the page never captures a swipe", () => {
    expect(css).toMatch(/\.ed-phone button,[\s\S]*?min-height: 44px/);
    expect(css).toMatch(/\.ed-phone \.ed-hit,[\s\S]*?touch-action: manipulation/);
    expect(css).not.toMatch(/\.ed-phone[^{]*\{[^}]*touch-action: none/);
  });
});
