import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../api/book";
import type { BookDesign, DesignBlock, DesignOptions } from "../api/bookDesign";
import i18n from "../i18n";
import { loaded } from "../components/imageQueue";
import { err, mockApi, ok, renderApp, user, type Call } from "../test/api";

// T-144: the customise screen: controls, live preview, save, reset, the leave guard and the owner check.

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";

const BLOCK: DesignBlock = {
  themes: ["lavender", "peach", "mint"],
  font_sets: ["playful", "classic"],
  sections: [
    { id: "cover", hideable: false, movable: false, fixed: "first" },
    { id: "profile", hideable: true, movable: true, fixed: null },
    { id: "notes", hideable: false, movable: true, fixed: null },
    { id: "back", hideable: true, movable: false, fixed: "last" },
  ],
  texts: [
    { id: "cover.belongs", max: 40 },
    { id: "cover.sticker", max: 40 },
    { id: "profile.heading", max: 40 },
    { id: "profile.subtitle", max: 50 },
    { id: "notes.heading", max: 40 },
    { id: "back.heading", max: 40 },
    { id: "back.subtitle", max: 60 },
    { id: "back.pact", max: 18 },
  ],
  images: [{ id: "cover.pattern", choices: ["dots", "plain"] }],
};

const yearbook = (over: object = {}) => ({
  id: BOOK,
  title: "Lớp 12A1",
  school_name: "THPT Chu Văn An",
  class_name: "12A1",
  graduation_year: 2026,
  motto: "",
  language: "vi",
  page_size: "A5",
  template_id: "memory-book",
  cover_media_id: null,
  created_at: 1760000000000,
  updated_at: 1760086400000,
  profile: {
    id: "p",
    is_owner: true,
    full_name: "Lan",
    nickname: "",
    birthday: null,
    quote: "",
    hobbies: "",
    future_plans: "",
    photo_media_id: null,
  },
  ...over,
});

const bookData = (over: Partial<BookData["yearbook"]> = {}): BookData => ({
  design: null,
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
    ...over,
  },
  profile: {
    full_name: "Nguyễn Thị Hồng",
    nickname: "Hồng",
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
});

const design = (
  options: DesignOptions | null = null,
  block: DesignBlock | null = BLOCK,
  template = "memory-book",
): BookDesign => ({
  template_id: template,
  options,
  updated_at: options ? 1793361600000 : null,
  design: block,
});

const MEMORY = {
  id: "memory-book",
  name: { en: "Memory Book", vi: "Lưu bút" },
  page_sizes: ["A5"],
  renderer: "html",
  preview: "/m.jpg",
};
const NAVY = {
  ...MEMORY,
  id: "navy-classic",
  name: { en: "Navy Classic", vi: "Navy" },
  preview: "/n.jpg",
};

type Hooks = Record<string, (c: Call) => Response>;
function serve(
  opts: { options?: DesignOptions | null; gone?: boolean; hooks?: Hooks; data?: BookData } = {},
) {
  return mockApi({
    "GET /user/get-me": () => ok(user()),
    [`GET /yearbook/get?id=${BOOK}`]: () => (opts.gone ? err(404, "not_found") : ok(yearbook())),
    [`GET /book-data/get?yearbook_id=${BOOK}`]: () => ok(opts.data ?? bookData()),
    [`GET /book-design/get?yearbook_id=${BOOK}`]: () => ok(design(opts.options ?? null)),
    "GET /template/get-list?page_size=A5": () => ok({ items: [MEMORY, NAVY], next_id: "" }),
    ...opts.hooks,
  });
}
const open = () => renderApp(`/yearbooks/${BOOK}/customise`);
const sheets = (label?: string) =>
  document.querySelectorAll(label ? `[data-page="${label}"]` : "section[data-page]");
const theme = () => [
  ...new Set(
    [...document.querySelectorAll<HTMLElement>(".bk-stage .mb-page")].map((e) => e.dataset.theme),
  ),
];
const loadedScreen = async () => {
  await screen.findByRole("heading", { name: "Customise the design" });
  await screen.findByRole("group", { name: "Colours" });
};

beforeEach(() => loaded.clear());
afterEach(() => vi.unstubAllGlobals());

describe("the customise screen", () => {
  it("loads the design once, shows the controls with the template's names and the real book as a preview", async () => {
    const calls = serve();
    open();
    await loadedScreen();
    const colours = screen.getByRole("group", { name: "Colours" });
    expect(
      within(colours)
        .getAllByRole("radio")
        .map((r) => (r as HTMLInputElement).value),
    ).toEqual(["lavender", "peach", "mint"]);
    expect(within(colours).getByRole("radio", { name: /Lavender/ })).toBeChecked();
    expect(within(colours).getByText("Selected")).toBeInTheDocument(); // the state is written, not only coloured
    expect(
      within(screen.getByRole("group", { name: "Fonts" })).getByRole("radio", { name: /Playful/ }),
    ).toBeChecked();
    expect(screen.getByLabelText("Cover: “this book belongs to”")).toHaveAttribute(
      "placeholder",
      "cuốn lưu bút này là của",
    ); // the book's own wording
    await waitFor(() => expect(sheets().length).toBe(4)); // cover, profile, ONE notes page (of three notes), back
    expect(calls.filter((c) => c.path.startsWith("/book-data/get")).length).toBe(1);
    expect(calls.filter((c) => c.path.startsWith("/book-design/get")).length).toBe(1);
    expect(screen.getByRole("button", { name: "Save design" })).toBeDisabled(); // nothing to save yet
  });

  it("applies every choice to the preview at once and saves exactly the validated document", async () => {
    let body: unknown;
    const u = userEvent.setup();
    serve({
      hooks: {
        "POST /book-design/update": (c) => {
          body = c.body;
          return ok(design((c.body as { options: DesignOptions }).options));
        },
      },
    });
    open();
    await loadedScreen();
    await u.click(screen.getByRole("radio", { name: /Peach/ }));
    expect(theme()).toEqual(["peach"]);
    await u.click(
      within(screen.getByRole("group", { name: "Fonts" })).getByRole("radio", { name: /Classic/ }),
    );
    await waitFor(() =>
      expect(document.querySelector('.bk-stage .mb-page[data-fonts="classic"]')).not.toBeNull(),
    );
    await u.click(screen.getByRole("switch", { name: "Print “All about me”" }));
    await u.click(screen.getByRole("button", { name: "Move “Notes from friends” up" }));
    await u.type(screen.getByLabelText("Cover: “this book belongs to”"), "Của Hồng");
    await u.click(screen.getByRole("radio", { name: /Plain paper/ }));
    expect(screen.getByText("You have changes that are not saved.")).toBeInTheDocument();
    await u.click(screen.getByRole("button", { name: "Save design" }));
    await screen.findByText("Design saved.");
    expect(body).toEqual({
      yearbook_id: BOOK,
      options: {
        v: 1,
        theme: "peach",
        fonts: "classic",
        sections: {
          order: ["cover", "notes", "profile", "back"],
          hidden: ["profile"],
        },
        texts: { "cover.belongs": "Của Hồng" },
        images: { "cover.pattern": "plain" },
      },
    });
    expect(screen.getByRole("button", { name: "Save design" })).toBeDisabled(); // saved: no longer a change
  });

  it("shows the pages of the focused control only, and the notes as one page", async () => {
    const u = userEvent.setup();
    serve();
    open();
    await loadedScreen();
    await u.click(screen.getByLabelText("Last page: reunion pact"));
    await waitFor(() =>
      expect([...sheets()].map((s) => s.getAttribute("data-page"))).toEqual(["back"]),
    );
    await u.click(screen.getByLabelText("Friends’ pages: heading"));
    await waitFor(() =>
      expect([...sheets()].map((s) => s.getAttribute("data-page"))).toEqual(["notes"]),
    );
  });

  it("counts characters, cuts at the manifest's limit and keeps the template's wording as the placeholder", async () => {
    const u = userEvent.setup();
    serve();
    open();
    await loadedScreen();
    const pact = screen.getByLabelText("Last page: reunion pact");
    expect(pact).toHaveAttribute("placeholder", "Hẹn ngày gặp lại");
    expect(screen.getAllByText("0/18").length).toBe(1);
    await u.click(pact);
    await u.paste("😄".repeat(30));
    expect(pact).toHaveValue("😄".repeat(18));
    expect(screen.getByText("18/18")).toBeInTheDocument();
  });

  it("shows the saved design after a reload", async () => {
    serve({
      options: {
        v: 1,
        theme: "mint",
        fonts: "classic",
        sections: { order: ["cover", "notes", "profile", "back"], hidden: ["back"] },
        texts: { "back.pact": "Hẹn" },
        images: { "cover.pattern": "plain" },
      },
    });
    open();
    await loadedScreen();
    expect(screen.getByRole("radio", { name: /Mint/ })).toBeChecked();
    expect(
      within(screen.getByRole("group", { name: "Fonts" })).getByRole("radio", { name: /Classic/ }),
    ).toBeChecked();
    expect(screen.getByRole("switch", { name: "Print “Let’s stay in touch”" })).not.toBeChecked();
    expect(screen.getByLabelText("Last page: reunion pact")).toHaveValue("Hẹn");
    expect(screen.getByRole("radio", { name: /Plain paper/ })).toBeChecked();
    const names = screen
      .getAllByRole("listitem")
      .filter((li) => li.hasAttribute("data-sec"))
      .map((li) => li.getAttribute("data-sec"));
    expect(names).toEqual(["cover", "notes", "profile", "back"]);
    expect(screen.getByRole("button", { name: "Save design" })).toBeDisabled();
  });

  it("names the labels in Vietnamese when the app is", async () => {
    await i18n.changeLanguage("vi");
    serve();
    open();
    await screen.findByRole("group", { name: "Màu sắc" });
    expect(screen.getByRole("radio", { name: /Đào/ })).toBeInTheDocument();
    expect(screen.getByLabelText("Trang cuối: lời hẹn gặp lại")).toBeInTheDocument();
  });
});

describe("reordering without a mouse", () => {
  it("keeps the focus on the moved section's control and announces the new place", async () => {
    const u = userEvent.setup();
    serve();
    open();
    await loadedScreen();
    const down = () => screen.getByRole("button", { name: "Move “All about me” down" });
    down().focus();
    await u.keyboard("{Enter}");
    // profile is now last among the movable places: its down button is disabled, so the focus moved to its up button
    expect(screen.getByRole("button", { name: "Move “All about me” up" })).toHaveFocus();
    expect(screen.getByText("“All about me” is now number 3 of 4.")).toBeInTheDocument();
    const order = screen
      .getAllByRole("listitem")
      .filter((li) => li.hasAttribute("data-sec"))
      .map((li) => li.getAttribute("data-sec"));
    expect(order).toEqual(["cover", "notes", "profile", "back"]);
    // the fixed sections have no move buttons and say so
    const cover = screen
      .getAllByRole("listitem")
      .find((li) => li.getAttribute("data-sec") === "cover");
    expect(within(cover as HTMLElement).queryByRole("button")).toBeNull();
    expect(within(cover as HTMLElement).getByText("Fixed: always first")).toBeInTheDocument();
    await u.keyboard("{Enter}"); // up again, from the keyboard, on the same button
    expect(screen.getByText("“All about me” is now number 2 of 4.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Move “All about me” down" })).toHaveFocus();
  });

  it("announces what a switch does", async () => {
    const u = userEvent.setup();
    serve();
    open();
    await loadedScreen();
    await u.click(screen.getByRole("switch", { name: "Print “Let’s stay in touch”" }));
    expect(screen.getByText("“Let’s stay in touch” will be left out.")).toBeInTheDocument();
    expect(screen.getByText("Left out")).toBeInTheDocument();
  });
});

describe("saving problems", () => {
  it("shows a 400 ERROR_INVALID_DESIGN message on the text it names", async () => {
    const u = userEvent.setup();
    serve({
      hooks: {
        "POST /book-design/update": () =>
          Response.json(
            {
              status: "ERROR_INVALID_DESIGN",
              error_message:
                "design options: texts.cover.sticker: must be at most 40 characters, one line, no control characters",
              request_id: "r",
            },
            { status: 400 },
          ),
      },
    });
    open();
    await loadedScreen();
    await u.type(screen.getByLabelText("Cover: sticker"), "x");
    await u.click(screen.getByRole("button", { name: "Save design" }));
    const alert = await screen.findByText(/texts\.cover\.sticker: must be at most 40 characters/);
    expect(screen.getByLabelText("Cover: sticker")).toHaveAccessibleDescription(
      /texts\.cover\.sticker/,
    );
    expect(alert).toHaveAttribute("role", "alert");
    expect(screen.getByRole("button", { name: "Save design" })).toBeEnabled(); // the edits are kept, the owner can fix and save again
    expect(screen.getByLabelText("Cover: sticker")).toHaveValue("x");
  });

  it("shows another failure above the buttons", async () => {
    const u = userEvent.setup();
    serve({
      hooks: {
        "POST /book-design/update": () => err(429, "rate_limited", { "Retry-After": "120" }),
      },
    });
    open();
    await loadedScreen();
    await u.click(screen.getByRole("radio", { name: /Mint/ }));
    await u.click(screen.getByRole("button", { name: "Save design" }));
    expect(await screen.findByText("Too many attempts. Try again in 2 min.")).toBeInTheDocument();
  });
});

describe("reset to template", () => {
  it("asks first, naming what is lost, then returns to the template's look", async () => {
    const u = userEvent.setup();
    let posts = 0;
    serve({
      options: { v: 1, theme: "mint" },
      hooks: {
        "POST /book-design/reset": () => {
          posts++;
          return ok({});
        },
      },
    });
    open();
    await loadedScreen();
    expect(theme()).toEqual(["mint"]);
    await u.click(screen.getByRole("button", { name: "Reset to template" }));
    const dialog = await screen.findByRole("dialog", { name: "Reset the design?" });
    expect(dialog).toHaveTextContent(/deletes your colours, fonts, section order/);
    expect(dialog).toHaveTextContent(/notes and photos stay/);
    await u.click(within(dialog).getByRole("button", { name: "Keep my design" }));
    expect(posts).toBe(0);
    expect(theme()).toEqual(["mint"]);
    await u.click(screen.getByRole("button", { name: "Reset to template" }));
    await u.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Reset the design" }),
    );
    await screen.findByText("The design is back to the template’s look.");
    expect(posts).toBe(1);
    expect(theme()).toEqual(["lavender"]);
    expect(screen.getByRole("radio", { name: /Lavender/ })).toBeChecked();
    expect(screen.getByRole("button", { name: "Save design" })).toBeDisabled();
  });
});

describe("leaving with unsaved changes", () => {
  it("asks before an in-app link is followed, and lets the owner stay or leave", async () => {
    const u = userEvent.setup();
    serve({ hooks: { [`GET /book-data/get?yearbook_id=${BOOK}`]: () => ok(bookData()) } });
    open();
    await loadedScreen();
    const where = () => screen.getByTestId("where").textContent;
    await u.click(screen.getByRole("radio", { name: /Peach/ }));
    await u.click(screen.getByRole("link", { name: "Back to the book" }));
    const dialog = await screen.findByRole("dialog", { name: "Leave without saving?" });
    expect(where()).toBe(`/yearbooks/${BOOK}/customise`);
    await u.click(within(dialog).getByRole("button", { name: "Stay here" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("radio", { name: /Peach/ })).toBeChecked(); // nothing was lost
    await u.click(screen.getAllByRole("link", { name: "Back to the book" })[0] as HTMLElement);
    await u.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Leave" }));
    await waitFor(() => expect(where()).toBe(`/yearbooks/${BOOK}/book`));
  });

  it("does not ask when nothing changed, and asks the browser before the tab is closed", async () => {
    const u = userEvent.setup();
    serve();
    open();
    await loadedScreen();
    const unload = () => {
      const e = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(e);
      return e.defaultPrevented;
    };
    expect(unload()).toBe(false);
    await u.click(screen.getByRole("radio", { name: /Mint/ }));
    expect(unload()).toBe(true);
    await u.click(screen.getByRole("radio", { name: /Lavender/ })); // back to the saved state
    expect(unload()).toBe(false);
    await u.click(screen.getAllByRole("link", { name: "Back to the book" })[0] as HTMLElement);
    await waitFor(() =>
      expect(screen.getByTestId("where").textContent).toBe(`/yearbooks/${BOOK}/book`),
    );
  });

  it("does not ask after a save", async () => {
    const u = userEvent.setup();
    serve({
      hooks: {
        "POST /book-design/update": (c) =>
          ok(design((c.body as { options: DesignOptions }).options)),
      },
    });
    open();
    await loadedScreen();
    await u.click(screen.getByRole("radio", { name: /Mint/ }));
    await u.click(screen.getByRole("button", { name: "Save design" }));
    await screen.findByText("Design saved.");
    const e = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(e);
    expect(e.defaultPrevented).toBe(false);
  });
});

describe("who may open it", () => {
  it("shows the not-found screen to someone else and makes no design or book request", async () => {
    const calls = serve({ gone: true });
    open();
    expect(await screen.findByText("This yearbook does not exist.")).toBeInTheDocument();
    expect(
      calls
        .map((c) => c.path)
        .filter((p) => p.startsWith("/book-design") || p.startsWith("/book-data")),
    ).toEqual([]);
    expect(screen.queryByRole("button", { name: "Save design" })).toBeNull();
  });
});

describe("a book whose template cannot be customised", () => {
  it("says so and offers the template picker", async () => {
    mockApi({
      "GET /user/get-me": () => ok(user()),
      [`GET /yearbook/get?id=${BOOK}`]: () => ok(yearbook({ template_id: null })),
      [`GET /book-data/get?yearbook_id=${BOOK}`]: () => ok(bookData({ template_id: null })),
      [`GET /book-design/get?yearbook_id=${BOOK}`]: () => ok(design(null, null, "")),
      "GET /template/get-list?page_size=A5": () => ok({ items: [MEMORY], next_id: "" }),
    });
    open();
    expect(await screen.findByText(/cannot be customised/)).toBeInTheDocument();
    expect(await screen.findByRole("radio", { name: /Memory Book/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save design" })).toBeNull();
  });
});

describe("changing the template", () => {
  it("says the design was reset and loads the new template's choices", async () => {
    const u = userEvent.setup();
    let template: string = "memory-book";
    const calls = mockApi({
      "GET /user/get-me": () => ok(user()),
      [`GET /yearbook/get?id=${BOOK}`]: () => ok(yearbook()),
      [`GET /book-data/get?yearbook_id=${BOOK}`]: () => ok(bookData()),
      [`GET /book-design/get?yearbook_id=${BOOK}`]: () =>
        template === "memory-book"
          ? ok(design({ v: 1, theme: "peach" }))
          : ok(
              design(
                null,
                {
                  themes: ["navy", "gold"],
                  font_sets: ["serif"],
                  sections: BLOCK.sections.slice(0, 1),
                  texts: [],
                  images: [],
                },
                template,
              ),
            ),
      "GET /template/get-list?page_size=A5": () => ok({ items: [MEMORY, NAVY], next_id: "" }),
      "POST /yearbook/update": () => {
        template = "navy-classic";
        return ok({ id: BOOK, template_id: "navy-classic" });
      },
    });
    open();
    await loadedScreen();
    expect(screen.queryByText(/the earlier design was reset/)).toBeNull();
    await u.click(await screen.findByRole("radio", { name: /Navy Classic/ }));
    expect(await screen.findByText(/the earlier design was reset/)).toBeInTheDocument();
    const colours = await screen.findByRole("group", { name: "Colours" });
    await waitFor(() =>
      expect(
        within(colours)
          .getAllByRole("radio")
          .map((r) => (r as HTMLInputElement).value),
      ).toEqual(["navy", "gold"]),
    );
    expect(calls.filter((c) => c.path.startsWith("/book-design/get")).length).toBe(2);
    expect(screen.getByRole("button", { name: "Save design" })).toBeDisabled();
  });
});

describe("browser Back (T-148)", () => {
  const openAfter = async () => {
    window.history.replaceState(null, "", "/yearbooks");
    window.history.pushState(null, "", `/yearbooks/${BOOK}/customise`);
    renderApp(`/yearbooks/${BOOK}/customise`, { browser: true });
    await loadedScreen();
  };
  const back = () => act(() => window.history.back());
  const url = () => window.location.pathname;
  const dirty = (u: ReturnType<typeof userEvent.setup>) =>
    u.click(screen.getByRole("radio", { name: /Peach/ }));

  it("asks first; Stay keeps the edits and the URL, and a second Back asks again", async () => {
    const u = userEvent.setup();
    serve();
    await openAfter();
    await dirty(u);
    back();
    const dialog = await screen.findByRole("dialog", { name: "Leave without saving?" });
    await waitFor(() => expect(window.history.length).toBeGreaterThan(0));
    expect(url()).toBe(`/yearbooks/${BOOK}/customise`);
    await u.click(within(dialog).getByRole("button", { name: "Stay here" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("radio", { name: /Peach/ })).toBeChecked();
    expect(url()).toBe(`/yearbooks/${BOOK}/customise`);
    back();
    expect(
      await screen.findByRole("dialog", { name: "Leave without saving?" }),
    ).toBeInTheDocument();
    expect(url()).toBe(`/yearbooks/${BOOK}/customise`);
  });

  it("Leave goes back to the screen before", async () => {
    const u = userEvent.setup();
    serve();
    await openAfter();
    await dirty(u);
    back();
    await u.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Leave" }));
    await waitFor(() => expect(url()).toBe("/yearbooks"));
  });

  it("leaves at once without edits, and after the edits are saved", async () => {
    const u = userEvent.setup();
    serve({
      hooks: {
        "POST /book-design/update": (c) =>
          ok(design((c.body as { options: DesignOptions }).options)),
      },
    });
    await openAfter();
    await dirty(u);
    await u.click(screen.getByRole("button", { name: "Save design" }));
    await screen.findByText("Design saved.");
    await waitFor(() => expect(url()).toBe(`/yearbooks/${BOOK}/customise`));
    back();
    await waitFor(() => expect(url()).toBe("/yearbooks"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("an in-app link leaves one entry, so Back from there returns to the screen", async () => {
    const u = userEvent.setup();
    serve();
    await openAfter();
    await dirty(u);
    await u.click(screen.getAllByRole("link", { name: "Back to the book" })[0] as HTMLElement);
    await u.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Leave" }));
    await waitFor(() => expect(url()).toBe(`/yearbooks/${BOOK}/book`));
    back();
    await waitFor(() => expect(url()).toBe(`/yearbooks/${BOOK}/customise`));
  });
});

describe("a template change with unsaved edits (T-148)", () => {
  const tplApi = () =>
    serve({
      hooks: {
        "POST /yearbook/update": () => ok({ id: BOOK, template_id: "navy-classic" }),
      },
    });

  it("asks first and names what is lost; cancel keeps the template and the edits", async () => {
    const u = userEvent.setup();
    const calls = tplApi();
    open();
    await loadedScreen();
    await u.click(screen.getByRole("radio", { name: /Peach/ }));
    await u.click(await screen.findByRole("radio", { name: /Navy Classic/ }));
    const dialog = await screen.findByRole("dialog", { name: "Change the template?" });
    expect(dialog).toHaveTextContent(/not saved/);
    await u.click(within(dialog).getByRole("button", { name: "Keep editing" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(calls.some((c) => c.path === "/yearbook/update")).toBe(false);
    expect(screen.getByRole("radio", { name: /Memory Book/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /Peach/ })).toBeChecked();
  });

  it("changes the template after a yes, and does not ask without edits", async () => {
    const u = userEvent.setup();
    const calls = tplApi();
    open();
    await loadedScreen();
    await u.click(screen.getByRole("radio", { name: /Peach/ }));
    await u.click(screen.getByRole("radio", { name: /Navy Classic/ }));
    await u.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Change the template",
      }),
    );
    await waitFor(() => expect(calls.filter((c) => c.path === "/yearbook/update")).toHaveLength(1));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("goes straight through when nothing is unsaved", async () => {
    const u = userEvent.setup();
    const calls = tplApi();
    open();
    await loadedScreen();
    await u.click(await screen.findByRole("radio", { name: /Navy Classic/ }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(calls.filter((c) => c.path === "/yearbook/update")).toHaveLength(1));
  });
});

describe("retries (T-148)", () => {
  const BOOK_GET = `GET /yearbook/get?id=${BOOK}`;
  it("shows not-found at once for a 404 and a 403, without retrying", async () => {
    for (const status of [404, 403]) {
      loaded.clear();
      const calls = mockApi({
        "GET /user/get-me": () => ok(user()),
        [BOOK_GET]: () => err(status, status === 404 ? "not_found" : "forbidden"),
      });
      const view = renderApp(`/yearbooks/${BOOK}/customise`, { retry: 3 });
      await waitFor(
        () => expect(calls.filter((c) => c.path.startsWith("/yearbook/get"))).toHaveLength(1),
        { timeout: 500 },
      );
      if (status === 404)
        expect(
          await screen.findByText("This yearbook does not exist.", {}, { timeout: 500 }),
        ).toBeInTheDocument();
      await new Promise((r) => setTimeout(r, 1300)); // the first retry would fire after 1 s
      expect(calls.filter((c) => c.path.startsWith("/yearbook/get"))).toHaveLength(1);
      view.unmount();
      vi.unstubAllGlobals();
    }
  });

  it("still retries a 5xx", async () => {
    let n = 0;
    mockApi({
      "GET /user/get-me": () => ok(user()),
      [BOOK_GET]: () => (n++ === 0 ? err(500, "internal_error") : ok(yearbook())),
      [`GET /book-data/get?yearbook_id=${BOOK}`]: () => ok(bookData()),
      [`GET /book-design/get?yearbook_id=${BOOK}`]: () => ok(design()),
      "GET /template/get-list?page_size=A5": () => ok({ items: [MEMORY], next_id: "" }),
    });
    renderApp(`/yearbooks/${BOOK}/customise`, { retry: 3 });
    expect(
      await screen.findByRole("group", { name: "Colours" }, { timeout: 4000 }),
    ).toBeInTheDocument();
    expect(n).toBe(2);
  }, 10000);
});

describe("links to the screen", () => {
  it("the print screen links to it next to the template picker", async () => {
    serve();
    renderApp(`/yearbooks/${BOOK}/book`);
    const link = await screen.findByRole("link", { name: "Customise the design" });
    expect(link).toHaveAttribute("href", `/yearbooks/${BOOK}/customise`);
  });
});
