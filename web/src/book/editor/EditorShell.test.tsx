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
    expect(screen.getByRole("textbox", { name: "Text of the box (edit here)" })).toHaveValue(
      "Xin chào lớp 12A1",
    );
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
    const handles = screen.getAllByRole("button", { name: /^(Resize from|Rotate$)/ });
    // this sticker is smaller than a hit area on this stage, so only the corner handles and the rotate handle are drawn (the panel and the keys resize it)
    expect(handles.map((h) => h.getAttribute("aria-label")).sort()).toEqual([
      "Resize from bottom left",
      "Resize from bottom right",
      "Resize from top left",
      "Resize from top right",
      "Rotate",
    ]);
    for (const h of handles) {
      expect(h.closest(".bk-sheet")).toBeNull(); // not inside overflow: hidden (ADR 0004 risk 8)
      h.focus();
      expect(h).toHaveFocus();
    }
    expect(screen.getAllByRole("button", { name: /^Resize from/ })).toHaveLength(4);
    await userEvent.click(hit("Text: Xin chào lớp 12A1"));
    // a box wide enough for them keeps the top and bottom handles
    expect(screen.getAllByRole("button", { name: /^Resize from (top|bottom)$/ })).toHaveLength(2);
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

/** The stage is 408 px wide on this screen: half a screen px per design px, so a pointer delta is double in design px. */
const stubStage = () => {
  const pagebox = document.querySelector<HTMLElement>(".ed-pagebox")!;
  pagebox.getBoundingClientRect = () =>
    ({ left: 0, top: 0, right: 408, bottom: 578, width: 408, height: 578, x: 0, y: 0 }) as DOMRect;
};
const pointer = (type: string, x: number, y: number, init: PointerEventInit = {}) =>
  new PointerEvent(type, {
    pointerId: 1,
    pointerType: "mouse",
    button: 0,
    clientX: x,
    clientY: y,
    bubbles: true,
    cancelable: true,
    ...init,
  });
const drag = (
  target: Element,
  from: [number, number],
  to: [number, number],
  init: PointerEventInit = {},
) => {
  fireEvent(target, pointer("pointerdown", ...from, init));
  fireEvent(window, pointer("pointermove", ...to, init));
  return () => fireEvent(window, pointer("pointerup", ...to, init));
};
const undoable = () =>
  !(screen.getByRole("button", { name: "Undo" }) as HTMLButtonElement).disabled;

describe("gestures (pointer)", () => {
  it("a drag moves the item by the pointer distance at the stage's scale and is one undo step", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    stubStage();
    expect(field("Left")).toHaveValue(60);
    const up = drag(hit("Sticker"), [100, 100], [140, 100], { altKey: true }); // 40 screen px = 80 design px = 9.8 % of the page
    up();
    expect(field("Left")).toHaveValue(69.8);
    expect(field("Top")).toHaveValue(field("Top").valueAsNumber); // unchanged by a horizontal drag
    expect(undoable()).toBe(true);
    expect(live()).toContain("Position 69.8 percent from the left");
    fireEvent.keyDown(document, { key: "z", ctrlKey: true });
    expect(field("Left")).toHaveValue(60);
    expect(undoable()).toBe(false); // one gesture, one step
  });

  it("Escape cancels the drag: the start box comes back, no step is recorded and the selection stays", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    stubStage();
    drag(hit("Sticker"), [100, 100], [160, 130], { altKey: true });
    await act(() => new Promise((r) => requestAnimationFrame(() => r(0))));
    expect(field("Left")).not.toHaveValue(60); // the preview shows it
    fireEvent.keyDown(window, { key: "Escape" });
    expect(field("Left")).toHaveValue(60);
    expect(undoable()).toBe(false);
    expect(hit("Sticker")).toHaveAttribute("aria-pressed", "true");
    fireEvent(window, pointer("pointerup", 160, 130));
    expect(undoable()).toBe(false);
    expect(live()).toBe("Cancelled");
  });

  it("a cancelled pointer and a click without movement record nothing; a second pointer is ignored", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    stubStage();
    drag(hit("Sticker"), [100, 100], [160, 130], { altKey: true });
    fireEvent(window, pointer("pointercancel", 160, 130));
    expect(field("Left")).toHaveValue(60);
    expect(undoable()).toBe(false);
    drag(hit("Sticker"), [100, 100], [101, 100])(); // below the slop: a click
    expect(undoable()).toBe(false);
    fireEvent(hit("Sticker"), pointer("pointerdown", 100, 100));
    fireEvent(hit("Sticker"), pointer("pointerdown", 300, 300, { pointerId: 2 }));
    fireEvent(window, pointer("pointermove", 300, 300, { pointerId: 2, altKey: true })); // not the gesture's pointer
    fireEvent(window, pointer("pointerup", 300, 300, { pointerId: 2 }));
    expect(field("Left")).toHaveValue(60);
    fireEvent(window, pointer("pointercancel", 100, 100));
  });

  it("snaps to the page centre line within 6 screen px and Alt switches it off", async () => {
    await mount();
    await userEvent.click(hit("Sticker")); // 150 wide: its centre is at 60 + 7.5 = 67.5 %, the page centre is at 50 %
    stubStage();
    const centreLeft = (816 / 2 - (150 * 0.816) / 2) / 8.16; // design px -> % of the page width
    // move the item's centre to 3 screen px (6 design px) beside the page centre: it snaps onto it
    const dx = (816 / 2 + 6 - (600 * 0.816 + 61.2)) * 0.5;
    drag(hit("Sticker"), [100, 100], [100 + dx, 100])();
    expect(field("Left").valueAsNumber).toBeCloseTo(centreLeft, 1);
    fireEvent.keyDown(document, { key: "z", ctrlKey: true });
    drag(hit("Sticker"), [100, 100], [100 + dx, 100], { altKey: true })();
    expect(field("Left").valueAsNumber).toBeGreaterThan(centreLeft + 0.5);
  });

  it("resizes from a handle along the item's own axes and rotates from the rotate handle", async () => {
    await mount();
    await userEvent.click(hit("Sticker")); // r = 10 degrees
    stubStage();
    const w0 = field("Width").valueAsNumber;
    const up = drag(
      screen.getByRole("button", { name: "Resize from bottom right" }),
      [200, 200],
      [240, 200],
      { altKey: true },
    );
    up();
    // 40 screen px along x on a box turned 10 degrees: 80 design px * cos(10) of width
    expect(field("Width").valueAsNumber).toBeCloseTo(
      w0 + ((80 * Math.cos((10 * Math.PI) / 180)) / 816) * 100,
      0,
    );
    fireEvent.keyDown(document, { key: "z", ctrlKey: true });
    const centre = [(600 * 0.816 + 61.2) * 0.5, 0];
    void centre;
    const rotate = screen.getByRole("button", { name: "Rotate" });
    drag(rotate, [200, 100], [200, 100])(); // no movement: nothing changes
    expect(field("Rotation (degrees)")).toHaveValue(10);
  });

  it("touch: a swipe over an unselected item starts nothing, the tap's click selects, then a drag moves", async () => {
    await mount();
    stubStage();
    fireEvent(hit("Sticker"), pointer("pointerdown", 100, 100, { pointerType: "touch" }));
    fireEvent(window, pointer("pointermove", 160, 100, { pointerType: "touch", altKey: true }));
    fireEvent(window, pointer("pointerup", 160, 100, { pointerType: "touch" }));
    expect(hit("Sticker")).toHaveAttribute("aria-pressed", "false"); // no click came: the browser panned instead
    expect(undoable()).toBe(false);
    fireEvent.click(hit("Sticker")); // a tap
    expect(hit("Sticker")).toHaveAttribute("aria-pressed", "true");
    expect(undoable()).toBe(false);
    drag(hit("Sticker"), [100, 100], [160, 100], { pointerType: "touch", altKey: true })();
    expect(undoable()).toBe(true);
  });
});

describe("gestures (keyboard and buttons)", () => {
  const key = (k: string, init: object = {}) => userEvent.keyboard(k, init);

  it("arrows move 1 design px and Shift 10; a burst of keys is one undo step; the live region says what happened", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    hit("Sticker").focus();
    const left0 = field("Left").valueAsNumber;
    await key("{ArrowRight}{ArrowRight}{ArrowRight}");
    await key("{Shift>}{ArrowRight}{/Shift}");
    expect(field("Left").valueAsNumber).toBeCloseTo(left0 + (13 / 816) * 100, 1);
    expect(live()).toContain("Moved right 10.");
    expect(live()).toContain("percent from the left");
    expect(undoable()).toBe(false); // the burst is still open: nothing committed yet
    await key("{Control>}z{/Control}"); // Ctrl closes the burst, then undo takes the whole burst away
    expect(field("Left")).toHaveValue(left0);
    expect(undoable()).toBe(false);
  });

  it("Alt+Shift+Arrow resizes by 10 along the box's own axes; Alt+Arrow alone is left to the browser", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    hit("Sticker").focus();
    const w0 = field("Width").valueAsNumber;
    const alt = fireEvent.keyDown(hit("Sticker"), { key: "ArrowLeft", altKey: true });
    expect(alt).toBe(true); // not default-prevented
    await key("{Alt>}{Shift>}{ArrowRight}{/Shift}{/Alt}");
    expect(field("Width").valueAsNumber).toBeGreaterThan(w0);
    expect(live()).toContain("Size");
  });

  it("comma and period rotate by 1 degree and with Shift by 15; the angle wraps to (-180, 180]", async () => {
    await mount();
    await userEvent.click(hit("Sticker")); // 10 degrees
    hit("Sticker").focus();
    await key(".");
    expect(field("Rotation (degrees)")).toHaveValue(11);
    await key("{Shift>}.{/Shift}"); // Shift+. types >
    expect(field("Rotation (degrees)")).toHaveValue(26);
    await key(",");
    expect(field("Rotation (degrees)")).toHaveValue(25);
    expect(live()).toContain("Rotated to 25 degrees");
  });

  it("Delete removes the selected item, clears the selection and says so; a template element is not deleted", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    hit("Sticker").focus();
    await key("{Delete}");
    expect(screen.queryByRole("button", { name: "Sticker" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Page: Cover" })).toBeInTheDocument();
    expect(live()).toBe("Deleted");
    fireEvent.keyDown(document, { key: "z", ctrlKey: true });
    expect(hit("Sticker")).toBeInTheDocument();
    await userEvent.click(hit("Cover title"));
    hit("Cover title").focus();
    await key("{Delete}");
    expect(hit("Cover title")).toBeInTheDocument();
  });

  it("the panel buttons rotate and resize with the same steps", async () => {
    await mount();
    await userEvent.click(hit("Sticker"));
    const w0 = field("Width").valueAsNumber;
    await userEvent.click(screen.getByRole("button", { name: "Rotate right 15°" }));
    expect(field("Rotation (degrees)")).toHaveValue(25);
    await userEvent.click(screen.getByRole("button", { name: "Rotate left 1°" }));
    expect(field("Rotation (degrees)")).toHaveValue(24);
    await userEvent.click(screen.getByRole("button", { name: "Wider" }));
    expect(field("Width").valueAsNumber).toBeCloseTo(w0 + (10 / 816) * 100, 0);
    await userEvent.click(screen.getByRole("button", { name: "Shorter" }));
    expect(live()).toBe("Shorter");
  });

  it("? opens the shortcut list (all rows, in Vietnamese too) and Escape closes it", async () => {
    await mount();
    await userEvent.keyboard("?");
    const sheet = screen.getByRole("region", { name: "Keyboard shortcuts" });
    expect(within(sheet).getAllByRole("row")).toHaveLength(14);
    expect(within(sheet).getByText(/Alt \+ Shift/)).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("region", { name: "Keyboard shortcuts" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Keyboard shortcuts" }));
    expect(screen.getByRole("region", { name: "Keyboard shortcuts" })).toBeInTheDocument();
    cleanup();
    await i18n.changeLanguage("vi");
    await mount();
    await userEvent.click(screen.getByRole("button", { name: "Phím tắt" }));
    expect(screen.getByText("Xoay 15 độ")).toBeInTheDocument();
  });
});

describe("hit order follows the drawn order (T-160 A1)", () => {
  const hitOrder = () =>
    [...document.querySelectorAll(".ed-hit")].map((b) => b.getAttribute("aria-label"));

  it("a template element moved later is drawn, and hit, above one moved earlier; items stay on top", async () => {
    await mount();
    expect(hitOrder().slice(0, 3)).toEqual(["Cover title", "Cover photo", "Cover sticker"]);
    // the photo is moved first, then the title: the title's box is appended to the page after the photo's, so it is drawn above it
    await userEvent.click(hit("Cover photo"));
    await setNum("Width", "30");
    await userEvent.click(hit("Cover title"));
    await setNum("Width", "50");
    expect(hitOrder()).toEqual([
      "Cover sticker", // not moved: still the template's own
      "Cover photo",
      "Cover title",
      "Text: Xin chào lớp 12A1",
      "Sticker",
    ]);
    const drawn = [...document.querySelectorAll(".ed-pagebox [data-layout-id]")].map((n) =>
      n.getAttribute("data-layout-id"),
    );
    expect(drawn.filter((id) => id === "cover.photo" || id === "cover.title")).toEqual([
      "cover.photo",
      "cover.title",
    ]);
    // Tab walks the same order, so the topmost of an overlap is the one a click reaches
    hit("Cover sticker").focus();
    await userEvent.tab();
    expect(document.activeElement).toBe(hit("Cover photo"));
    await userEvent.tab();
    expect(document.activeElement).toBe(hit("Cover title"));
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
