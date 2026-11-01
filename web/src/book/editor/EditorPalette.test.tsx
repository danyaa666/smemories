import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../../api/book";
import { ApiError } from "../../api/client";
import i18n from "../../i18n";
import { assets } from "../assets";
import type { LayoutDoc, LayoutItem } from "./document";
import { EditorShell } from "./EditorShell";
import { DesignImages } from "./palette/DesignImages";
import { Uploads } from "./palette/uploads";
import type { DesignImages as Images } from "./palette/useDesignImages";
import { mockServer, type MockMedia } from "./mockServer";
import { DEBOUNCE_MS } from "./store";

// T-162: the palette in the editor shell: tabs, the searchable keyboard grid, adding, tint, duplicate, the image controls, the background tab,
// the delete-in-use dialog and the refused-items button. jsdom has no layout; the e2e spec editor-palette.spec.mjs runs the same in a browser.
const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const M1 = "01J9Z3K6V8Q4M7N2P5R8IMG001";
const M2 = "01J9Z3K6V8Q4M7N2P5R8IMG002";
const IMGS: MockMedia[] = [
  { id: M1, width: 4000, height: 3000 },
  { id: M2, width: 600, height: 900 },
];
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
const item = (id: string, over: Partial<LayoutItem>): LayoutItem => ({
  id,
  t: "asset",
  asset: "sticker.gold-star",
  x: 100,
  w: 200,
  h: 200,
  cy: 400,
  ...over,
});

const mount = async (layout: LayoutDoc | null = null, media = IMGS) => {
  const srv = mockServer({ version: 2, layout, media });
  render(
    <MemoryRouter>
      <EditorShell bookId={BOOK} data={data()} safeMm={5} />
    </MemoryRouter>,
  );
  await screen.findByRole("toolbar");
  return srv;
};
const tab = (name: string) => screen.getByRole("tab", { name });
const tile = (name: string | RegExp) => screen.getByRole("button", { name });
const live = () => document.querySelector(".visually-hidden[role=status]")?.textContent;
const saved = async (srv: ReturnType<typeof mockServer>) => {
  await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS + 50));
  return srv.s.layout;
};

beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("palette tabs and grid", () => {
  it("has the five tabs; the arrow keys move between them", async () => {
    await mount();
    expect(screen.getAllByRole("tab").map((t) => t.textContent)).toEqual([
      "Stickers",
      "Shapes",
      "Frames",
      "My images",
      "Background",
    ]);
    tab("Stickers").focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(tab("Shapes")).toHaveFocus();
    expect(tab("Shapes")).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(tab("Background")).toHaveFocus();
  });

  it("every tile is named by its label, in the language of the screen", async () => {
    await mount();
    expect(tile("Cool sun")).toBeInTheDocument();
    await act(() => i18n.changeLanguage("vi"));
    expect(tile("Mặt trời đeo kính")).toBeInTheDocument();
  });

  it('searches without diacritics ("mat troi" finds "Mặt trời đeo kính") and says when nothing matches', async () => {
    await mount();
    await act(() => i18n.changeLanguage("vi"));
    await userEvent.type(screen.getByRole("searchbox"), "mat troi");
    expect(tile("Mặt trời đeo kính")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ngôi sao vàng" })).toBeNull();
    await userEvent.clear(screen.getByRole("searchbox"));
    await userEvent.type(screen.getByRole("searchbox"), "zzzzzz");
    expect(screen.getByText(/Không tìm thấy/)).toBeInTheDocument();
  });

  it("one tab stop; arrows, Home and End move between the tiles; Enter adds the sticker, selects it and puts the focus on it", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    const tiles = within(screen.getByRole("group", { name: "Stickers" })).getAllByRole("button");
    expect(tiles.filter((b) => b.tabIndex === 0)).toHaveLength(1);
    tiles[0]!.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(tiles[1]).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}");
    expect(tiles[4]).toHaveFocus();
    await userEvent.keyboard("{End}");
    expect(tiles.at(-1)).toHaveFocus();
    await userEvent.keyboard("{Home}");
    expect(tiles[0]).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await act(() => vi.advanceTimersByTimeAsync(10));
    const added = document.querySelector<HTMLElement>(".ed-box.is-selected")!;
    expect(added).toBeTruthy();
    expect(added.querySelector(".ed-hit")).toHaveFocus(); // the keys of T-160 act on it now
    expect(live()).toMatch(/^Added: /);
    const stored = (await saved(srv))!.pages.cover!.items![0]!;
    expect(stored).toMatchObject({ t: "asset", w: 250, x: 375, cy: 500 });
    expect(stored.asset).toBe(assets.filter((a) => a.kind === "sticker")[0]!.id);
  });
});

describe("adding and limits", () => {
  it("a click adds on top of the others; the 41st item on a page is refused with a message", async () => {
    const items = Array.from({ length: 40 }, (_, i) => item(`it${String(i).padStart(4, "0")}`, {}));
    await mount({ v: 1, pages: { cover: { items } } });
    await userEvent.click(tile("Cool sun"));
    expect(screen.getByRole("alert")).toHaveTextContent("already has 40 items");
    expect(document.querySelectorAll(".ed-box[data-target^='it']")).toHaveLength(40);
  });
  it("the 151st in the book is refused with its own message", async () => {
    const page = (p: string, n: number) =>
      Array.from({ length: n }, (_, i) => item(`${p}${String(i).padStart(4, "0")}`, {}));
    await mount({
      v: 1,
      pages: {
        cover: { items: page("c", 39) },
        profile: { items: page("p", 40) },
        back: { items: page("b", 40) },
        notes: { items: page("n", 31) },
      },
    });
    await userEvent.click(tile("Cool sun"));
    expect(screen.getByRole("alert")).toHaveTextContent("already has 150 added items");
  });
});

describe("asset, image and duplicate controls", () => {
  it("tints a tintable asset with the colour tokens, and goes back to the original colours", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount({
      v: 1,
      pages: { cover: { items: [item("star01", { asset: "shape.heart" })] } },
    });
    await userEvent.click(screen.getByRole("button", { name: "Sticker" }));
    const group = screen.getByRole("radiogroup", { name: "Colour of the picture" });
    expect(within(group).getAllByRole("radio").length).toBeGreaterThan(2);
    expect(screen.getByRole("button", { name: "Original colours" })).toBeDisabled();
    await userEvent.click(within(group).getByRole("radio", { name: "Accent" }));
    expect((await saved(srv))!.pages.cover!.items![0]!.tint).toBe("accent");
    await userEvent.click(screen.getByRole("button", { name: "Original colours" }));
    expect("tint" in (await saved(srv))!.pages.cover!.items![0]!).toBe(false);
  });
  it("a picture that paints its own colours has no tint control", async () => {
    await mount({
      v: 1,
      pages: { cover: { items: [item("star01", { asset: "sticker.gold-star" })] } },
    });
    await userEvent.click(screen.getByRole("button", { name: "Sticker" }));
    const tintable = assets.find((a) => a.id === "sticker.gold-star")!.tintable;
    expect(!!screen.queryByRole("radiogroup", { name: "Colour of the picture" })).toBe(tintable);
  });

  it("Duplicate and Ctrl+D copy the selected item 3 % right and down, as one step each", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount({ v: 1, pages: { cover: { items: [item("star01", {})] } } });
    await userEvent.click(screen.getByRole("button", { name: "Sticker" }));
    await userEvent.click(screen.getByRole("button", { name: "Duplicate" }));
    let items = (await saved(srv))!.pages.cover!.items!;
    expect(items).toHaveLength(2);
    expect(items[1]).toMatchObject({ x: 130, cy: 421, w: 200 });
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "d", ctrlKey: true });
    items = (await saved(srv))!.pages.cover!.items!;
    expect(items).toHaveLength(3);
    expect(items[2]!.x).toBe(160);
    await userEvent.click(screen.getByRole("button", { name: "Undo" })); // one step each
    expect((await saved(srv))!.pages.cover!.items).toHaveLength(2);
  });

  it("an image item has fit, a corner radius slider with its value and a picker for another image", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount({
      v: 1,
      pages: {
        cover: {
          items: [
            {
              id: "img001",
              t: "image",
              media: M1,
              fit: "contain",
              x: 100,
              w: 400,
              h: 300,
              cy: 300,
            },
          ],
        },
      },
    });
    await userEvent.click(screen.getByRole("button", { name: "Picture" }));
    await userEvent.selectOptions(screen.getByLabelText("Fit"), "cover");
    const slider = screen.getByRole("slider", { name: "Corner radius" });
    fireEvent.change(slider, { target: { value: "120" } });
    fireEvent.keyUp(slider);
    expect(screen.getByRole("group", { name: /Image/ }).querySelector("output")).toHaveTextContent(
      "120",
    );
    let it = (await saved(srv))!.pages.cover!.items![0]!;
    expect(it).toMatchObject({ fit: "cover", radius: 120, w: 400, h: 300, x: 100 });
    await userEvent.click(screen.getByRole("button", { name: "Replace image" }));
    await userEvent.click(await screen.findByRole("button", { name: "Use image 2 of 2" }));
    it = (await saved(srv))!.pages.cover!.items![0]!;
    expect(it).toMatchObject({ media: M2, w: 400, h: 300, x: 100 }); // the box is kept
    expect(live()).toBe("Image replaced");
  });
});

describe("My images tab", () => {
  it("lists the design images as thumbnails (lazy, thumb variant) and adds one at its aspect ratio, contain, selected", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    await userEvent.click(tab("My images"));
    const add = await screen.findByRole("button", { name: "Add image 1 of 2" });
    const img = add.querySelector("img")!;
    expect(img.getAttribute("src")).toBe(`/api/media/get-content?id=${M1}&variant=thumb`);
    expect(img.getAttribute("loading")).toBe("lazy");
    await userEvent.click(add);
    expect(live()).toBe("Added your image");
    expect((await saved(srv))!.pages.cover!.items![0]).toMatchObject({
      t: "image",
      media: M1,
      fit: "contain",
      w: 500,
      h: 375,
      x: 250,
    });
    expect(screen.getByRole("heading", { name: "Picture" })).toBeInTheDocument(); // selected
  });
  it("says so when there are no images yet", async () => {
    await mount(null, []);
    await userEvent.click(tab("My images"));
    expect(await screen.findByText(/No images yet/)).toBeInTheDocument();
  });
});

describe("Background tab", () => {
  it("says which page it changes; a colour, none, an asset and an image are one step each; all page types in one step", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    await userEvent.click(tab("Background"));
    expect(screen.getByText(/background of the cover\./)).toBeInTheDocument();
    await userEvent.click(
      within(
        within(screen.getByRole("complementary", { name: "Add to the page" })).getByRole(
          "radiogroup",
          { name: "Background colour" },
        ),
      ).getByRole("radio", {
        name: "Accent",
      }),
    );
    expect((await saved(srv))!.pages.cover!.bg).toEqual({ color: "accent" });
    await userEvent.click(screen.getByRole("button", { name: "Cool sun" }));
    expect((await saved(srv))!.pages.cover!.bg).toEqual({ asset: "sticker.cool-sun" });
    await userEvent.click(
      await screen.findByRole("button", { name: "Use image 1 of 2 as background" }),
    );
    expect((await saved(srv))!.pages.cover!.bg).toEqual({ media: M1 });
    await userEvent.click(screen.getByRole("button", { name: "Use on all page types" }));
    const pages = (await saved(srv))!.pages;
    expect(["cover", "profile", "notes", "back"].map((k) => pages[k]?.bg)).toEqual(
      Array(4).fill({ media: M1 }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Undo" })); // the grouped change is one step
    expect((await saved(srv))!.pages.profile?.bg).toBeUndefined();
    expect((await saved(srv))!.pages.cover!.bg).toEqual({ media: M1 });
    await userEvent.click(screen.getByRole("button", { name: "Remove background" }));
    expect((await saved(srv))!.pages.cover!.bg).toBeUndefined();
  });
  it("the nothing-selected panel has the colour swatches and a way to the tab", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    const panel = screen.getByRole("complementary", { name: "Properties" });
    await userEvent.click(within(panel).getByRole("radio", { name: "Paper" }));
    expect((await saved(srv))!.pages.cover!.bg).toEqual({ color: "paper" });
    await userEvent.click(within(panel).getByRole("button", { name: /More backgrounds/ }));
    expect(tab("Background")).toHaveAttribute("aria-selected", "true");
  });
});

describe("deleting a design image", () => {
  const used: LayoutDoc = {
    v: 1,
    pages: {
      cover: {
        bg: { media: M1 },
        items: [
          { id: "img001", t: "image", media: M1, x: 100, w: 400, h: 300, cy: 300 },
          item("keep01", {}),
        ],
      },
      profile: {
        items: [{ id: "img002", t: "image", media: M1, x: 100, w: 400, h: 300, cy: 300 }],
      },
    },
  };
  const askDelete = async (n: string) => {
    await userEvent.click(tab("My images"));
    await userEvent.click(await screen.findByRole("button", { name: n }));
  };

  it("in use: the dialog says where and that history is cleared; confirm saves first, then deletes, clears the history and the list", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount(used);
    await userEvent.click(screen.getByRole("button", { name: "Sticker" })); // so there is some history after the first edit
    await userEvent.click(screen.getByRole("button", { name: "Duplicate" }));
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
    await askDelete("Delete image 1 of 2");
    const dlg = screen.getByRole("dialog");
    expect(dlg).toHaveTextContent("used on 2 page types (the cover, the profile page)");
    expect(dlg).toHaveTextContent("Undo and redo are cleared");
    await userEvent.click(within(dlg).getByRole("button", { name: "Delete image" }));
    await waitFor(() => expect(srv.s.deleted).toEqual([M1]));
    expect(srv.s.calls.slice(-2)).toEqual(["save", `delete:${M1}`]);
    expect(JSON.stringify(srv.s.layout)).not.toContain(M1);
    expect(srv.s.layout!.pages.cover!.items!.map((i) => i.id)).toContain("keep01");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Redo" })).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Delete image 2 of 2" })).toBeNull(); // only one image is left in the list
    expect(live()).toBe("Image deleted");
  });

  it("not in use: a plain question and the history stays", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount(used);
    await userEvent.click(screen.getByRole("button", { name: "Sticker" }));
    await userEvent.click(screen.getByRole("button", { name: "Duplicate" }));
    await askDelete("Delete image 2 of 2");
    const dlg = screen.getByRole("dialog");
    expect(dlg).toHaveTextContent("deleted for good");
    expect(dlg).not.toHaveTextContent("Undo and redo");
    await userEvent.click(within(dlg).getByRole("button", { name: "Delete image" }));
    await waitFor(() => expect(srv.s.deleted).toEqual([M2]));
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
  });

  it("when the delete call fails the dialog says so and the image stays in the list", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount(used);
    const real = (
      globalThis.fetch as unknown as ReturnType<typeof vi.fn>
    ).getMockImplementation()! as unknown as (u: string, r?: RequestInit) => Promise<Response>;
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockImplementation(
      async (u: string, r?: RequestInit) =>
        u === "/api/media/delete"
          ? Response.json(
              { status: "ERROR_STORAGE_ERROR", error_message: "x", request_id: "r" },
              { status: 502 },
            )
          : real(u, r),
    );
    await askDelete("Delete image 1 of 2");
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete image" }),
    );
    expect(
      await screen.findByText(/saved without the image, but it could not be deleted/),
    ).toBeInTheDocument();
    expect(JSON.stringify(srv.s.layout)).not.toContain(M1);
    await userEvent.click(screen.getByRole("button", { name: "Keep it" }));
    expect(screen.getByRole("button", { name: "Delete image 1 of 2" })).toBeInTheDocument();
  });
});

describe("a layout the server refuses", () => {
  it("shows the problem list and a button that removes the offending items", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount({
      v: 1,
      pages: {
        cover: {
          items: [
            item("keep01", {}),
            {
              id: "bad001",
              t: "image",
              media: "01J9Z3K6V8Q4M7N2P5R8GONE00",
              x: 100,
              w: 400,
              h: 300,
              cy: 300,
            },
          ],
        },
      },
    });
    srv.s.script.push(() =>
      srv.fail(400, "ERROR_INVALID_LAYOUT", {
        problems: [{ path: "pages.cover.items[1].media", code: "unknown_media" }],
      }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Sticker" }));
    await userEvent.click(screen.getByRole("button", { name: "Duplicate" }));
    await act(() => vi.advanceTimersByTimeAsync(DEBOUNCE_MS + 50));
    expect(await screen.findByText(/server refused the layout/)).toBeInTheDocument();
    expect(screen.getByText(/pages.cover.items\[1\].media/)).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Remove the items the server refused" }),
    );
    const stored = (await saved(srv))!;
    expect(stored.pages.cover!.items!.map((i) => i.id)).toEqual(["keep01", expect.any(String)]);
    expect(JSON.stringify(stored)).not.toContain("bad001");
  });

  it("an image the server no longer has is drawn as a placeholder that can be selected", async () => {
    const real = (id: string) => ({
      id,
      t: "image" as const,
      media: M1,
      x: 100,
      w: 400,
      h: 300,
      cy: 300,
    });
    await mount({ v: 1, pages: { cover: { items: [real("img001")] } } });
    const stage = document.querySelector(".ed-pagebox")!;
    const photo = stage.querySelector<HTMLImageElement>("[data-layout-item='img001'] img")!;
    fireEvent.error(photo); // the image loader retries once after a second before it gives up
    await new Promise((r) => setTimeout(r, 1100));
    fireEvent.error(stage.querySelector("[data-layout-item='img001'] img")!);
    expect(
      await within(stage as HTMLElement).findByRole("img", { name: "Missing image" }),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Picture" }));
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(stage.querySelector("[data-layout-item='img001']")).toBeNull();
  });
});

describe("the upload area", () => {
  const images = (): Images => ({
    items: [],
    status: "ready",
    more: false,
    loadingMore: false,
    loadMore: () => {},
    reload: () => {},
    prepend: () => {},
    drop: () => {},
  });
  const send = async (err: unknown, have = 3, name = "a.png", type = "image/png") => {
    const up = new Uploads({ yearbookId: "Y", upload: () => Promise.reject(err) });
    up.bind({ have: () => have, onDone: () => {} });
    render(
      <MemoryRouter>
        <DesignImages
          images={images()}
          uploads={up}
          onPick={() => {}}
          pickLabel={() => "x"}
          readOnly={false}
        />
      </MemoryRouter>,
    );
    await userEvent.upload(screen.getByTestId("design-file"), new File(["x"], name, { type }));
    return screen.findByRole("alert");
  };
  const e = (status: number, code: string, retry?: number) =>
    new ApiError(status, code, "m", undefined, retry);
  it.each([
    ["too large", e(413, "payload_too_large"), 3, /larger than 10 MB/],
    ["unsupported", e(415, "unsupported_media_type"), 3, /Only JPEG, PNG and WebP/],
    ["corrupt", e(400, "invalid_image"), 3, /could not be read/],
    ["cap with the count", e(400, "quota_exceeded"), 30, /already has 30 of 30/],
    ["quota", e(400, "quota_exceeded"), 4, /500 MB is reached/],
    ["rate limit with the wait", e(429, "rate_limited", 5), 3, /Try again in 5 s/],
    ["busy with the wait", e(503, "busy", 8), 3, /busy.*8 s/],
    ["not verified, with the link", e(403, "email_not_verified"), 3, /Verify your email address/],
    ["offline", e(0, "network_error"), 3, /No connection/],
  ])("%s", async (_n, err, have, text) => {
    expect(await send(err, have)).toHaveTextContent(text);
  });
  it("links to the existing verify flow when the email is not verified", async () => {
    await send(e(403, "email_not_verified"));
    expect(screen.getByRole("link", { name: "Verify your email" })).toHaveAttribute(
      "href",
      "/verify-email",
    );
  });
  it("a GIF is refused on the client, before anything is sent", async () => {
    const upload = vi.fn();
    const up = new Uploads({ yearbookId: "Y", upload });
    render(
      <MemoryRouter>
        <DesignImages
          images={images()}
          uploads={up}
          onPick={() => {}}
          pickLabel={() => "x"}
          readOnly={false}
        />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByTestId("design-file"), {
      target: { files: [new File(["x"], "a.gif", { type: "image/gif" })] },
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("Only JPEG, PNG and WebP");
    expect(upload).not.toHaveBeenCalled();
  });
});
