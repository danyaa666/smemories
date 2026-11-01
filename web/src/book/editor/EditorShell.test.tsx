import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import { getTemplate } from "../templates";
import type { LayoutDoc } from "./document";
import { EditorShell } from "./EditorShell";
import { mockServer } from "./mockServer";
import { DEBOUNCE_MS } from "./store";

// T-159: the editor shell: page strip, stage, selection overlay, property panel, toolbar, banners. jsdom has no layout, so a template element
// is drawn at the template's natural place (the measured place is the browser's job, see tools/e2e/editor-shell.spec.mjs).

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const LAYOUT: LayoutDoc = {
  v: 1,
  pages: {
    cover: {
      items: [
        { id: "textAA", t: "text", text: "Xin chào lớp 12A1", x: 100, w: 300, h: 100, cy: 700 },
        {
          id: "starBB",
          t: "asset",
          asset: "sticker.gold-star",
          x: 600,
          w: 150,
          h: 150,
          cy: 300,
          r: 10,
        },
      ],
    },
  },
};

const data = (over: Partial<BookData> = {}): BookData => ({
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
  notes: [1, 2, 3].map((i) => ({
    id: `01J9Z3K6V8Q4M7N2P5R8N0000${i}`,
    answers: { name: `Bạn ${i}`, message: `Chúc mừng số ${i}` },
    signature: null,
    photo_ids: [],
  })),
  note_count: 3,
  approved_total: 3,
  truncated: false,
  ...over,
});

const mount = async (layout: LayoutDoc | null = LAYOUT, over: Partial<BookData> = {}) => {
  const srv = mockServer({ version: 2, layout });
  render(
    <MemoryRouter>
      <EditorShell bookId={BOOK} data={data(over)} safeMm={5} />
    </MemoryRouter>,
  );
  await screen.findByRole("toolbar");
  return srv;
};
const hit = (name: string | RegExp) => screen.getByRole("button", { name });
const field = (name: string) => screen.getByLabelText(name) as HTMLInputElement;
const sheetKind = () => document.querySelector(".ed-pagebox .bk-sheet")?.getAttribute("data-page");
const moved = (id: string) =>
  document.querySelector(`.ed-pagebox [data-layout-id="${id}"]`)?.classList.contains("mb-ov");
const live = () => document.querySelector(".visually-hidden[role=status]")?.textContent;
const setNum = async (name: string, v: string) => {
  const input = field(name);
  fireEvent.change(input, { target: { value: v } });
  fireEvent.keyDown(input, { key: "Enter" });
};

beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("page strip", () => {
  it("has one thumbnail per page in book order, marks the current one and moves with the arrow keys", async () => {
    await mount();
    const strip = screen.getByRole("navigation", { name: "Pages" });
    const names = within(strip)
      .getAllByRole("button")
      .map((b) => b.getAttribute("aria-label"));
    expect(names).toEqual([
      "Page 1 of 6: Cover",
      "Page 2 of 6: Profile",
      "Page 3 of 6: Notes page 1",
      "Page 4 of 6: Notes page 2",
      "Page 5 of 6: Notes page 3",
      "Page 6 of 6: Back cover",
    ]);
    expect(within(strip).getByRole("button", { name: /Cover/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(sheetKind()).toBe("cover");
    expect(strip.querySelectorAll(".bk-sheet").length).toBe(6); // the real book components, one sheet each
    within(strip).getByRole("button", { name: /Cover/ }).focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(sheetKind()).toBe("profile");
    expect(within(strip).getByRole("button", { name: /Profile/ })).toHaveFocus();
    await userEvent.keyboard("{End}");
    expect(sheetKind()).toBe("back");
    await userEvent.keyboard("{ArrowUp}{ArrowUp}");
    expect(document.querySelector(".ed-pagebox .bk-sheet")?.getAttribute("data-page")).toBe(
      "notes",
    );
    expect(live()).toContain("Page 4 of 6");
  });

  it("clears the selection when the page changes", async () => {
    await mount();
    await userEvent.click(hit("Cover title"));
    expect(hit("Cover title")).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(screen.getByRole("button", { name: /Profile/ }));
    expect(screen.getByRole("heading", { name: "Page: Profile" })).toBeInTheDocument();
  });
});

describe("selection", () => {
  it("selects an element and an item by click, announces it and clears on an empty click or Escape", async () => {
    await mount();
    expect(screen.getByRole("heading", { name: "Page: Cover" })).toBeInTheDocument();
    await userEvent.click(hit("Cover title"));
    expect(screen.getByRole("heading", { name: "Cover title" })).toBeInTheDocument();
    expect(live()).toBe("Selected: Cover title");
    await userEvent.click(hit(/Text: Xin chào/));
    expect(screen.getByRole("heading", { name: "Text: Xin chào lớp 12A1" })).toBeInTheDocument();
    expect(screen.getByText("Xin chào lớp 12A1", { selector: ".ed-preview" })).toBeInTheDocument();
    fireEvent.click(document.querySelector(".ed-scroll")!);
    expect(live()).toBe("Selection cleared");
    expect(screen.getByRole("heading", { name: "Page: Cover" })).toBeInTheDocument();
    await userEvent.click(hit("Cover photo"));
    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("heading", { name: "Page: Cover" })).toBeInTheDocument();
  });

  it("Tab cycles the page's elements and then its items in draw order, selecting as it goes", async () => {
    await mount();
    hit("Cover title").focus();
    const order: string[] = [];
    for (let i = 0; i < 5; i++) {
      order.push(document.activeElement?.getAttribute("aria-label") ?? "?");
      await userEvent.tab();
    }
    expect(order).toEqual([
      "Cover title",
      "Cover photo",
      "Cover sticker",
      "Text: Xin chào lớp 12A1",
      "Sticker",
    ]);
    expect(hit("Sticker")).toHaveAttribute("aria-pressed", "true"); // focus selected it
  });

  it("draws the handles of the selection outside the page's clip, as focusable shapes", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    const handles = screen.getAllByRole("button", { name: /^(Resize from|Rotate)/ });
    expect(handles.map((h) => h.getAttribute("aria-label"))).toEqual([
      "Resize from top left",
      "Resize from top",
      "Resize from top right",
      "Resize from right",
      "Resize from bottom right",
      "Resize from bottom",
      "Resize from bottom left",
      "Resize from left",
      "Rotate",
    ]);
    for (const h of handles) {
      expect(h.closest(".bk-sheet")).toBeNull(); // not inside overflow: hidden (ADR 0004 risk 8)
      h.focus();
      expect(h).toHaveFocus();
    }
    expect(screen.getAllByRole("button", { name: /^Resize from/ })).toHaveLength(8);
    fireEvent.click(document.querySelector(".ed-scroll")!);
    expect(screen.queryByRole("button", { name: "Rotate" })).toBeNull();
  });

  it("has a labelled stage and a safe margin that can be toggled", async () => {
    await mount();
    expect(screen.getByRole("region", { name: "Page being edited: Cover" })).toBeInTheDocument();
    expect(screen.getByTestId("safe-margin")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Safe margin" }));
    expect(screen.queryByTestId("safe-margin")).toBeNull();
  });

  it("zooms with the buttons and Ctrl + wheel, and goes back to fit", async () => {
    await mount();
    const zoom = () => document.querySelector<HTMLElement>(".ed-pagebox")!.dataset.zoom;
    expect(screen.getByRole("button", { name: "Fit to window" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await userEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    expect(zoom()).toBe("75");
    await userEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    expect(zoom()).toBe("100");
    expect(screen.getByLabelText("Zoom")).toHaveTextContent("100 %");
    fireEvent.wheel(document.querySelector(".ed-scroll")!, { deltaY: -100, ctrlKey: true });
    expect(zoom()).toBe("125");
    fireEvent.wheel(document.querySelector(".ed-scroll")!, { deltaY: 100 }); // no key: the page scrolls
    expect(zoom()).toBe("125");
    for (let i = 0; i < 5; i++)
      await userEvent.click(screen.getByRole("button", { name: "Zoom out" }));
    expect(zoom()).toBe("50");
    expect(screen.getByRole("button", { name: "Zoom out" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Fit to window" }));
    expect(screen.getByRole("button", { name: "Fit to window" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
});

describe("property panel", () => {
  it("writes the whole box of an element, one history step per committed input, and undoes it from the keyboard", async () => {
    await mount();
    await userEvent.click(hit("Cover title"));
    expect(field("Width")).toHaveValue(82.4); // 672 of 816 px, as % of the page
    expect(moved("cover.title")).toBe(false);
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    await setNum("Width", "50");
    expect(moved("cover.title")).toBe(true);
    expect(field("Width")).toHaveValue(50);
    // the other numbers did not jump back to something else
    expect(field("Left")).toHaveValue(11.8);
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
    await setNum("Rotation (degrees)", "5");
    fireEvent.keyDown(document, { key: "z", ctrlKey: true });
    expect(field("Rotation (degrees)")).toHaveValue(0);
    expect(field("Width")).toHaveValue(50); // one step undone, the width step is still there
    fireEvent.keyDown(document, { key: "z", metaKey: true });
    expect(moved("cover.title")).toBe(false);
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    fireEvent.keyDown(document, { key: "z", ctrlKey: true, shiftKey: true });
    expect(moved("cover.title")).toBe(true);
    fireEvent.keyDown(document, { key: "z", metaKey: true }); // undo again
    fireEvent.keyDown(document, { key: "y", ctrlKey: true }); // Ctrl+Y redoes
    expect(moved("cover.title")).toBe(true);
    expect(field("Rotation (degrees)")).toHaveValue(0);
    fireEvent.keyDown(document, { key: "y", ctrlKey: true });
    expect(field("Rotation (degrees)")).toHaveValue(5);
    expect(screen.getByRole("button", { name: "Redo" })).toBeDisabled();
  });

  it("changes an item's place, shows a refusal and keeps the typed number out of the store", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    expect(field("Left")).toHaveValue(60); // x 600 thousandths of the width
    await setNum("Left", "10");
    expect(field("Left")).toHaveValue(10);
    const star = () =>
      document.querySelector<HTMLElement>(".ed-pagebox [data-layout-item=starBB]")!;
    expect(parseFloat(star().style.left)).toBeCloseTo(81.6, 0);
    await setNum("Width", "500"); // far outside the limits
    expect(await screen.findByRole("alert")).toHaveTextContent("Between 1 and 200");
    expect(field("Width")).toHaveValue(15);
    await setNum("Width", "20");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("explains an empty or out-of-range entry, puts the old value back and stores nothing (R4)", async () => {
    await mount();
    await userEvent.click(hit("Cover title"));
    for (const bad of ["250", "-5", "0.04", ""]) {
      await setNum("Width", bad);
      expect(screen.getByRole("alert")).toHaveTextContent("Between 1 and 200");
      expect(field("Width")).toHaveValue(82.4);
    }
    expect(field("Width")).toHaveAttribute("aria-invalid", "true");
    expect(moved("cover.title")).toBe(false);
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    await setNum("Width", "50"); // a good entry clears the message
    expect(screen.queryByRole("alert")).toBeNull();
    await i18n.changeLanguage("vi");
    await userEvent.click(hit("Tiêu đề bìa"));
    await setNum("Rộng", "999");
    expect(screen.getByRole("alert")).toHaveTextContent("Nhập số từ 1 đến 200");
  });

  it("an edit writes only the field the student changed; the other stored values stay bit-identical (R3)", async () => {
    const stored = { x: 117, w: 601, h: 133, cy: 187, r: 0 };
    const srv = await mount({
      v: 1,
      pages: { cover: { overrides: { "cover.title": stored } } },
    });
    await userEvent.click(hit("Cover title"));
    const saved = async () => {
      await userEvent.click(screen.getByRole("link", { name: "Preview as PDF" }));
      await waitFor(() =>
        expect(srv.s.layout?.pages.cover?.overrides?.["cover.title"]).toBeDefined(),
      );
      return srv.s.layout!.pages.cover!.overrides!["cover.title"];
    };
    const tab = { close: vi.fn(), location: { href: "" }, opener: {} };
    vi.stubGlobal(
      "open",
      vi.fn(() => tab),
    );
    await setNum("Width", "50");
    expect(await saved()).toEqual({ ...stored, w: 500 });
    await setNum("Left", "20");
    await setNum("Top", "30"); // Top commits cy from the stored height only
    await waitFor(() => expect(field("Top")).toHaveValue(30));
    await userEvent.click(screen.getByRole("link", { name: "Preview as PDF" }));
    await waitFor(() => expect(srv.s.saves.length).toBeGreaterThan(1));
    const ov = srv.s.layout!.pages.cover!.overrides!["cover.title"]!;
    expect(ov).toMatchObject({ x: 200, w: 500, h: 133, r: 0 });
    await setNum("Height", "20"); // keeps the shown Top, moves the centre
    expect(field("Top")).toHaveValue(30);
    vi.unstubAllGlobals();
  });

  it("reorders, hides, resets and deletes", async () => {
    await mount();
    await userEvent.click(hit(/Text: Xin chào/));
    const order = () =>
      [...document.querySelectorAll(".ed-pagebox [data-layout-item]")].map((e) =>
        e.getAttribute("data-layout-item"),
      );
    expect(order()).toEqual(["textAA", "starBB"]);
    expect(screen.getByRole("button", { name: "Send to back" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Bring to front" }));
    expect(order()).toEqual(["starBB", "textAA"]);
    expect(live()).toBe("Moved to position 2 of 2");
    await userEvent.click(screen.getByRole("button", { name: "Send backward" }));
    expect(order()).toEqual(["textAA", "starBB"]);
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(order()).toEqual(["starBB"]);
    expect(screen.getByRole("heading", { name: "Page: Cover" })).toBeInTheDocument();
    expect(live()).toBe("Deleted");

    await userEvent.click(hit("Cover photo"));
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull(); // an element is hidden, not deleted
    expect(screen.queryByRole("button", { name: "Bring to front" })).toBeNull();
    expect(screen.getByRole("button", { name: "Reset this element" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Hide" }));
    expect(document.querySelector(".ed-pagebox [data-layout-id='cover.photo']")).toBeNull();
    expect(hit("Cover photo (hidden)")).toBeInTheDocument(); // still selectable: it can come back
    await userEvent.click(screen.getByRole("button", { name: "Show" }));
    expect(document.querySelector(".ed-pagebox [data-layout-id='cover.photo']")).not.toBeNull();
    await setNum("Left", "30");
    expect(moved("cover.photo")).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Reset this element" }));
    expect(moved("cover.photo")).toBe(false);
  });

  it("clears a page background from the page properties", async () => {
    await mount({ v: 1, pages: { cover: { bg: { color: "paper" } } } });
    await userEvent.click(screen.getByRole("button", { name: "Clear background" }));
    expect(screen.queryByRole("button", { name: "Clear background" })).toBeNull();
  });
});

describe("toolbar and banners", () => {
  it("undoes and redoes with the buttons and says what it is saving", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).click(hit("Sticker"));
    await setNum("Left", "10");
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(field("Left")).toHaveValue(60);
    fireEvent.click(screen.getByRole("button", { name: "Redo" }));
    expect(field("Left")).toHaveValue(10);
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS + 10));
    expect(srv.s.saves).toHaveLength(1);
    expect(srv.s.saves[0]!.base_version).toBe(2);
    expect(screen.getByText("Saved")).toBeInTheDocument();
  });

  it("asks before resetting all layout, then goes back to the template", async () => {
    const srv = await mount();
    await userEvent.click(screen.getByRole("button", { name: "Reset all layout" }));
    const dlg = screen.getByRole("dialog");
    await userEvent.click(within(dlg).getByRole("button", { name: "Keep my layout" }));
    expect(srv.s.resets).toBe(0);
    expect(hit(/Text: Xin chào/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Reset all layout" }));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Reset all layout" }),
    );
    await waitFor(() => expect(srv.s.resets).toBe(1));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /Text: Xin chào/ })).toBeNull(),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens the PDF preview in a new tab and saves first", async () => {
    await mount();
    const a = screen.getByRole("link", { name: "Preview as PDF" });
    expect(a).toHaveAttribute("href", `/yearbooks/${BOOK}/book`);
    expect(a).toHaveAttribute("target", "_blank");
  });

  it("sends the new tab to the book only after the save in flight and the one queued behind it are done (R2)", async () => {
    const srv = await mount();
    const tab = { close: vi.fn(), location: { href: "" }, opener: {} };
    const open = vi.fn(() => tab);
    vi.stubGlobal("open", open);
    let release!: () => void;
    const held = new Promise<void>((r) => (release = r));
    srv.s.script.push(async () => {
      await held;
      srv.s.version++;
      return srv.body({ version: srv.s.version, updated_at: 1 });
    });
    await userEvent.click(hit("Cover title"));
    await setNum("Width", "40");
    await userEvent.click(screen.getByRole("link", { name: "Preview as PDF" })); // starts the first save
    await waitFor(() => expect(srv.s.saves).toHaveLength(1));
    await setNum("Width", "45"); // an edit while the first save is in flight: queued
    await userEvent.click(screen.getByRole("link", { name: "Preview as PDF" }));
    expect(open).toHaveBeenCalledWith("about:blank", "_blank"); // opened by the click, still blank
    expect(tab.location.href).toBe("");
    release();
    await waitFor(() => expect(tab.location.href).toBe(`/yearbooks/${BOOK}/book`));
    expect(srv.s.saves).toHaveLength(2);
    expect(srv.s.layout?.pages.cover?.overrides?.["cover.title"]?.w).toBe(450); // the tab opens on the last edit
    vi.unstubAllGlobals();
  });

  it("closes the tab and says so when the latest edit could not be saved (R2)", async () => {
    const srv = await mount();
    const tab = { close: vi.fn(), location: { href: "" }, opener: {} };
    vi.stubGlobal(
      "open",
      vi.fn(() => tab),
    );
    srv.s.script.push(() => srv.fail(400, "ERROR_INVALID_ARGUMENT", { problems: [] }));
    await userEvent.click(hit("Cover title"));
    await setNum("Width", "40");
    await userEvent.click(screen.getByRole("link", { name: "Preview as PDF" }));
    await waitFor(() => expect(tab.close).toHaveBeenCalled());
    expect(tab.location.href).toBe("");
    expect(await screen.findByText(/preview was not opened/)).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it("shows the conflict banner, blocks edits until a choice and both choices work", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    fireEvent.click(hit("Sticker"));
    await setNum("Left", "10");
    srv.s.otherTab({ v: 1, pages: { cover: { items: [LAYOUT.pages.cover!.items![0]!] } } });
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS + 10));
    expect(
      screen.getByText(/changed in another tab or device. Editing is paused/),
    ).toBeInTheDocument();
    expect(field("Left")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Reset all layout" })).toBeDisabled();
    fireEvent.keyDown(document, { key: "z", ctrlKey: true }); // the shortcut is blocked too
    expect(field("Left")).toHaveValue(10);
    fireEvent.click(screen.getByRole("button", { name: "Use the saved version" }));
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(screen.queryByText(/Editing is paused/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Sticker" })).toBeNull(); // their document has no sticker
  });

  it("keeps my version on purpose", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    fireEvent.click(hit("Sticker"));
    await setNum("Left", "10");
    srv.s.otherTab(null);
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS + 10));
    fireEvent.click(screen.getByRole("button", { name: "Keep my version" }));
    await act(() => vi.advanceTimersByTimeAsync(10));
    expect(srv.s.layout?.pages.cover?.items).toHaveLength(2);
    expect(screen.queryByText(/Editing is paused/)).toBeNull();
  });

  it("shows what the server refused in the invalid state", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    srv.s.script.push(() =>
      srv.fail(400, "ERROR_INVALID_LAYOUT", {
        problems: [{ path: "pages.cover.items[1].w", code: "out_of_range" }],
      }),
    );
    fireEvent.click(hit("Sticker"));
    await setNum("Left", "10");
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS + 10));
    expect(screen.getByRole("alert")).toHaveTextContent("pages.cover.items[1].w: out_of_range");
  });

  it("replaces the editor by a 'no longer exists' screen when the book is gone", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    srv.s.script.push(() => srv.fail(404, "ERROR_NOT_FOUND"));
    fireEvent.click(hit("Sticker"));
    await setNum("Left", "10");
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS + 10));
    expect(screen.getByRole("heading", { name: "This book no longer exists" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Pages" })).toBeNull();
  });
});

describe("words", () => {
  it("every editable element of every template has a name in both languages, and the panel speaks Vietnamese", async () => {
    const ids = Object.keys(getTemplate("memory-book").layout!.natural);
    expect(ids.length).toBeGreaterThan(0);
    for (const lng of ["en", "vi"]) {
      for (const id of ids)
        expect(i18n.exists(`editor.element.${id}`, { lng }), `${lng} ${id}`).toBe(true);
    }
    await i18n.changeLanguage("vi");
    await mount();
    expect(screen.getByRole("button", { name: "Hoàn tác" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Tiêu đề bìa" }));
    expect(screen.getByLabelText("Rộng")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Trang đang chỉnh: Bìa" })).toBeInTheDocument();
  });
});
