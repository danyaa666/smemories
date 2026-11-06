import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import en from "../../locales/en.json";
import vi_ from "../../locales/vi.json";
import type { LayoutDoc } from "./document";
import { EditorShell } from "./EditorShell";
import { mockServer, type MockTemplate } from "./mockServer";

// T-175: the Templates panel in the editor: nothing is fetched before it opens, the list per page kind, the keyboard walk, read-only preview, apply
// (settle first, one request, history cleared), delete, and every error. The real browser path is tools/e2e/editor-templates.spec.mjs.

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const textItem = (id: string, text: string) => ({
  id,
  t: "text" as const,
  text,
  x: 100,
  w: 300,
  h: 100,
  cy: 700,
});
const CURRENT: LayoutDoc = {
  v: 1,
  pages: { cover: { items: [textItem("textAA", "Bản hiện tại")] } },
};
const mt = (
  id: string,
  kind: string,
  text: string,
  over: Partial<MockTemplate> = {},
): MockTemplate => ({
  id,
  source: "shipped",
  label_en: `Label ${id}`,
  label_vi: `Nhãn ${id}`,
  kind,
  item_count: 1,
  page: { items: [textItem(`t${id}`.padEnd(8, "0"), text)] },
  ...over,
});
const TEMPLATES = (): MockTemplate[] => [
  mt("clean", "cover", "Mẫu sạch", { label_en: "Clean title", label_vi: "Tiêu đề gọn" }),
  mt("bold", "cover", "Mẫu đậm", { label_en: "Bold" }),
  mt("01J9Z3K6V8Q4M7N2P5R8OWN001", "cover", "Của tôi", {
    source: "own",
    label_en: "My cover",
    label_vi: "My cover",
    created_at: 5,
  }),
  mt("calm", "notes", "Lưu bút êm", { label_en: "Calm notes" }),
  mt("tidy", "back", "Bìa sau gọn", { label_en: "Tidy back" }),
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
  notes: [1, 2].map((i) => ({
    id: `01J9Z3K6V8Q4M7N2P5R8N0000${i}`,
    answers: { name: `Bạn ${i}`, message: `Chúc mừng số ${i}` },
    signature: null,
    photo_ids: [],
  })),
  note_count: 2,
  approved_total: 2,
  truncated: false,
});

const mount = async (opts: { layout?: LayoutDoc | null; templates?: MockTemplate[] } = {}) => {
  const srv = mockServer({
    version: 5,
    layout: opts.layout === undefined ? CURRENT : opts.layout,
    templates: opts.templates ?? TEMPLATES(),
  });
  render(
    <MemoryRouter>
      <EditorShell bookId={BOOK} data={data()} safeMm={5} />
    </MemoryRouter>,
  );
  await screen.findByRole("toolbar");
  return srv;
};
const button = () => screen.getByRole("button", { name: /^Templates$/ });
const open = async () => {
  await userEvent.click(button());
  return screen.findAllByRole("radio");
};
const stageText = () => document.querySelector(".ed-pagebox")?.textContent ?? "";
const live = () => document.querySelector(".visually-hidden[role=status]")?.textContent;
const cardName = (n: string) => screen.getByRole("radio", { name: new RegExp(`^Template ${n},`) });
const applyBtn = (n: string) =>
  within(cardName(n).closest(".ed-tpl-card") as HTMLElement).getByRole("button", {
    name: "Apply",
  });
const confirmBtn = () =>
  within(screen.getByRole("dialog")).getByRole("button", { name: "Apply template" });
const addTextButton = () =>
  within(screen.getByRole("toolbar", { name: "Editor toolbar" })).getByRole("button", {
    name: "Add text",
  });
const next = () => userEvent.click(screen.getByRole("button", { name: "Next page" }));

beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("opening and the list", () => {
  it("fetches nothing before the panel opens, then lists once for the cover in two groups with accessible names", async () => {
    const srv = await mount();
    await new Promise((r) => setTimeout(r, 50));
    expect(srv.s.tcalls).toEqual([]);
    const radios = await open();
    expect(srv.s.tcalls).toEqual(["list:cover"]);
    expect(button()).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByRole("heading", { level: 2, name: "Templates for the cover" }),
    ).toBeVisible();
    expect(radios.map((r) => r.getAttribute("aria-label"))).toEqual([
      "Template Clean title, 1 item, shipped",
      "Template Bold, 1 item, shipped",
      "Template My cover, 1 item, my template",
    ]);
    expect(screen.getByRole("group", { name: "Shipped" })).toBeVisible();
    const own = screen.getByRole("group", { name: "My templates" });
    expect(within(own).getByText(/Images not included/)).toBeVisible();
    expect(screen.getAllByRole("button", { name: "Delete" })).toHaveLength(1); // never on a shipped card
    expect(within(own).getByRole("button", { name: "Delete" })).toBeVisible();
  });

  it("names a shipped template in the language of the app", async () => {
    await i18n.changeLanguage("vi");
    await mount();
    await userEvent.click(screen.getByRole("button", { name: /^Mẫu trang$/ }));
    expect(await screen.findByRole("radio", { name: /^Mẫu Tiêu đề gọn,/ })).toBeVisible();
    expect(screen.getByRole("heading", { level: 2, name: "Mẫu cho bìa" })).toBeVisible();
  });

  it("shows the empty text for each group, a Refresh that calls the list again, and no polling", async () => {
    const srv = await mount({ templates: [] });
    await userEvent.click(button());
    await waitFor(() =>
      expect(screen.getAllByText("No templates for this page yet.")).toHaveLength(2),
    );
    srv.s.templates.push(...TEMPLATES());
    await userEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(await screen.findAllByRole("radio")).toHaveLength(3);
    expect(srv.s.tcalls).toEqual(["list:cover", "list:cover"]);
  });

  it("shows an error with a retry for a failed list, and the offline text for a network failure", async () => {
    const srv = await mount();
    const f = vi.mocked(fetch);
    const real = f.getMockImplementation()!;
    f.mockImplementationOnce(() => Promise.reject(new TypeError("down")));
    await userEvent.click(button());
    expect(await screen.findByText(/You are offline/)).toBeInTheDocument();
    f.mockImplementation(real);
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findAllByRole("radio")).toHaveLength(3);
    expect(srv.s.tcalls).toEqual(["list:cover"]);
  });

  it("draws at most 12 thumbnails at once", async () => {
    const many = Array.from({ length: 15 }, (_, i) => mt(`m${i}`, "cover", `Mẫu ${i}`));
    await mount({ templates: many });
    await open();
    expect(document.querySelectorAll(".ed-tpl-thumb .bk-sheet")).toHaveLength(12);
    expect(document.querySelectorAll(".ed-tpl-thumb")).toHaveLength(15);
  });

  it("is hidden when the book's template has no layout block", async () => {
    // a template without a `layout` spec never mounts the editor's button: the shell reads it from the book's template id
    const d = data();
    d.yearbook.template_id = "no-such-template";
    mockServer({ version: 5, layout: null });
    render(
      <MemoryRouter>
        <EditorShell bookId={BOOK} data={d} safeMm={5} />
      </MemoryRouter>,
    );
    await screen.findByRole("toolbar");
    expect(screen.queryByRole("button", { name: /^Templates$/ })).toBeNull();
  });
});

describe("page kind", () => {
  it("follows the page being edited, keeps two kinds in memory and says notes templates apply to every notes page", async () => {
    const srv = await mount();
    await open();
    expect(screen.queryByText(/applies to every notes page/)).toBeNull();
    await next(); // profile: no templates are shipped for it in the fixture
    await screen.findByRole("heading", { level: 2, name: "Templates for the profile page" });
    expect(srv.s.tcalls).toEqual(["list:cover", "list:profile"]);
    await next(); // notes page 1
    expect(
      await screen.findByRole("heading", { level: 2, name: "Templates for the notes pages" }),
    ).toBeVisible();
    expect(screen.getByText("A notes template applies to every notes page.")).toBeVisible();
    expect(await screen.findByRole("radio", { name: /^Template Calm notes,/ })).toBeVisible();
    await next(); // notes page 2: the same kind, no new call
    expect(screen.getByRole("radio", { name: /^Template Calm notes,/ })).toBeVisible();
    expect(srv.s.tcalls).toEqual(["list:cover", "list:profile", "list:notes"]);
    await userEvent.click(screen.getByRole("button", { name: "Previous page" }));
    await userEvent.click(screen.getByRole("button", { name: "Previous page" }));
    await userEvent.click(screen.getByRole("button", { name: "Previous page" }));
    expect(await screen.findByRole("radio", { name: /^Template Clean title,/ })).toBeVisible();
    expect(srv.s.tcalls.filter((c) => c === "list:cover")).toHaveLength(2); // the cover list was dropped when a third kind came
  });

  it("leaves a preview when the student steps to a page of another kind, not when the kind stays", async () => {
    await mount();
    await open();
    await next();
    await next(); // notes 1
    await userEvent.click(await screen.findByRole("radio", { name: /^Template Calm notes,/ }));
    await screen.findByText(/Previewing the template 'Calm notes'/);
    await next(); // notes 2: same kind
    expect(screen.getByText(/Previewing the template 'Calm notes'/)).toBeVisible();
    expect(stageText()).toContain("Lưu bút êm");
    await next(); // back cover
    expect(screen.queryByText(/Previewing the template/)).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Previous page" }));
    expect(screen.queryByText(/Previewing the template/)).toBeNull(); // does not come back by itself
    expect(stageText()).not.toContain("Lưu bút êm");
  });
});

describe("one side panel at a time", () => {
  it("opening Templates closes History and the reverse", async () => {
    await mount();
    await userEvent.click(screen.getByRole("button", { name: /History/ }));
    await screen.findByRole("heading", { name: "Version history" });
    await open();
    expect(screen.queryByRole("heading", { name: "Version history" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: /History/ }));
    await screen.findByRole("heading", { name: "Version history" });
    expect(screen.queryByRole("heading", { name: "Templates for the cover" })).toBeNull();
    expect(button()).toHaveAttribute("aria-expanded", "false");
  });
});

describe("keyboard", () => {
  it("walks the cards with the arrows, previews with Enter, Escape leaves the preview, then closes and gives focus back", async () => {
    await mount();
    const radios = await open();
    await userEvent.tab(); // from the panel to Refresh
    await userEvent.tab(); // then "Save this page as a template" (T-176)
    await userEvent.tab(); // and to the first card
    expect(radios[0]).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}");
    expect(radios[1]).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    expect(radios[2]).toHaveFocus(); // stops at the last card
    await userEvent.keyboard("{Home}");
    expect(radios[0]).toHaveFocus();
    await userEvent.keyboard("{End}{ArrowUp}{Enter}");
    await screen.findByText(/Previewing the template 'Bold'/);
    expect(radios[1]).toHaveAttribute("aria-checked", "true");
    await waitFor(() => expect(stageText()).toContain("Mẫu đậm"));
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByText(/Previewing the template/)).toBeNull();
    expect(stageText()).toContain("Bản hiện tại");
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("radiogroup", { name: "Templates" })).toBeNull();
    await waitFor(() => expect(button()).toHaveFocus());
  });

  it("applies with the keys: a card's Apply, the confirm, and focus lands on the Templates button", async () => {
    const srv = await mount();
    await open();
    applyBtn("Clean title").focus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus();
    await userEvent.tab();
    expect(confirmBtn()).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(srv.s.applies).toHaveLength(1));
    await waitFor(() => expect(button()).toHaveFocus());
  });
});

describe("preview", () => {
  it("is read-only, a status region, says how many items are left out, and never saves", async () => {
    const gone = mt("gone", "cover", "x", {
      page: {
        items: [
          {
            id: "goneAA",
            t: "asset",
            asset: "sticker.removed-long-ago",
            x: 1,
            w: 100,
            h: 100,
            cy: 1,
          },
        ],
      },
    });
    const srv = await mount({ templates: [...TEMPLATES(), gone] });
    await open();
    await userEvent.click(cardName("Label gone"));
    const bar = await screen.findByText(
      /Previewing the template 'Label gone' on the cover. Your page is unchanged./,
    );
    expect(bar).toHaveTextContent("1 item will be left out");
    expect(bar.closest("[role=status]")).not.toBeNull();
    expect(addTextButton()).toBeDisabled();
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Back to current" }));
    expect(stageText()).toContain("Bản hiện tại");
    await userEvent.click(cardName("Clean title"));
    await waitFor(() => expect(stageText()).toContain("Mẫu sạch"));
    expect(live()).toBe("Previewing a template. Your page is unchanged.");
    await new Promise((r) => setTimeout(r, 50));
    expect(srv.s.saves).toHaveLength(0);
    expect(srv.s.applies).toHaveLength(0); // a preview alone sends no write
  });

  it("closing the panel while previewing returns to the current page", async () => {
    await mount();
    await open();
    await userEvent.click(cardName("Clean title"));
    await screen.findByText(/Previewing the template/);
    await userEvent.click(button());
    expect(screen.queryByText(/Previewing the template/)).toBeNull();
    expect(stageText()).toContain("Bản hiện tại");
  });
});

describe("apply", () => {
  it("asks once, sends one request even on a double click, adopts the answer, clears undo, closes, announces and offers Undo in History", async () => {
    const srv = await mount();
    await open();
    await userEvent.click(cardName("Clean title"));
    await userEvent.click(await screen.findByRole("button", { name: "Apply to the cover" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("This replaces the layout of the cover");
    expect(dialog).toHaveTextContent("Your current version is kept in the history");
    expect(dialog).toHaveTextContent("Undo and redo start again");
    expect(srv.s.applies).toHaveLength(0);
    await userEvent.dblClick(confirmBtn());
    await waitFor(() => expect(live()).toContain("Template applied to the cover."));
    expect(srv.s.applies).toEqual([
      {
        yearbook_id: BOOK,
        base_version: 5,
        kind: "cover",
        source: "shipped",
        template_id: "clean",
      },
    ]);
    expect(stageText()).toContain("Mẫu sạch");
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Redo" })).toBeDisabled();
    expect(screen.queryByRole("heading", { name: "Templates for the cover" })).toBeNull();
    await waitFor(() => expect(button()).toHaveFocus());
    const toast = document.querySelector<HTMLElement>(".banner")!;
    expect(toast).toHaveTextContent("Template applied to the cover.");
    await userEvent.click(within(toast).getByRole("button", { name: "Undo in History" }));
    const rows = await screen.findAllByRole("radio");
    expect(rows[0]).toHaveAccessibleName(/Before template, 1 item$/);
    expect(document.querySelector(".banner")).toBeNull();
  });

  it("applies from a card as well, and the dropped count is announced", async () => {
    const srv = await mount();
    srv.s.dropped = 2;
    await open();
    await userEvent.click(applyBtn("Bold"));
    await userEvent.click(confirmBtn());
    await waitFor(() =>
      expect(live()).toBe(
        "Template applied to the cover. 2 items could not be applied because their image or sticker no longer exists.",
      ),
    );
    expect(srv.s.applies[0]).toMatchObject({ template_id: "bold", source: "shipped" });
  });

  it("sends an own template with its source", async () => {
    const srv = await mount();
    await open();
    await userEvent.click(applyBtn("My cover"));
    await userEvent.click(confirmBtn());
    await waitFor(() => expect(srv.s.applies).toHaveLength(1));
    expect(srv.s.applies[0]).toMatchObject({
      source: "own",
      template_id: "01J9Z3K6V8Q4M7N2P5R8OWN001",
    });
  });

  it("cancelling the confirm sends nothing", async () => {
    const srv = await mount();
    await open();
    await userEvent.click(applyBtn("Bold"));
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(srv.s.applies).toHaveLength(0);
    expect(screen.getByRole("heading", { name: "Templates for the cover" })).toBeVisible();
  });

  it("skips the confirm when the page has nothing to lose", async () => {
    const srv = await mount({
      layout: { v: 1, pages: { profile: { items: [textItem("textBB", "x")] } } },
    });
    await open();
    await userEvent.click(applyBtn("Bold"));
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(srv.s.applies).toHaveLength(1));
    await waitFor(() => expect(live()).toContain("Template applied to the cover."));
  });

  it("saves a pending edit first and asks with the version that save produced", async () => {
    const srv = await mount();
    await userEvent.click(addTextButton());
    expect(srv.s.saves).toHaveLength(0); // still in the debounce
    await open();
    await userEvent.click(applyBtn("Clean title"));
    await userEvent.click(confirmBtn());
    await waitFor(() => expect(srv.s.applies).toHaveLength(1));
    expect(srv.s.saves).toHaveLength(1);
    expect(srv.s.saves[0]!.base_version).toBe(5);
    expect(srv.s.applies[0]!.base_version).toBe(6);
  });

  it("sends nothing and says so when the newest edit cannot be saved", async () => {
    const srv = await mount();
    await userEvent.click(addTextButton());
    srv.s.script.push(() => Response.json({ status: "ERROR_INTERNAL" }, { status: 500 }));
    await open();
    await userEvent.click(applyBtn("Clean title"));
    await userEvent.click(confirmBtn());
    expect(await screen.findByText(/latest changes could not be saved yet/)).toBeVisible();
    expect(srv.s.applies).toHaveLength(0);
  });
});

describe("apply errors (each with its own text; the editor is left as it was)", () => {
  const fail = (
    srv: Awaited<ReturnType<typeof mount>>,
    status: number,
    code: string,
    data?: object,
    headers?: Record<string, string>,
  ) => srv.s.applyScript.push(() => srv.fail(status, code, data, headers));
  const attempt = async () => {
    await open();
    await userEvent.click(applyBtn("Clean title"));
    await userEvent.click(confirmBtn());
  };
  const untouched = (srv: Awaited<ReturnType<typeof mount>>) => {
    expect(stageText()).toContain("Bản hiện tại");
    expect(srv.s.saves).toHaveLength(0);
    expect(screen.getByRole("heading", { name: "Templates for the cover" })).toBeVisible();
  };

  it("another tab saved: the editor's conflict flow takes over, nothing is overwritten", async () => {
    const srv = await mount();
    fail(srv, 409, "ERROR_VERSION_CONFLICT", { version: 9, layout: { v: 1, pages: {} } });
    await attempt();
    expect(
      await screen.findByText(/changed in another tab or device, so no template was applied/),
    ).toBeVisible();
    expect(screen.getByText(/Editing is paused until you choose/)).toBeVisible();
    expect(addTextButton()).toBeDisabled();
    expect(srv.s.saves).toHaveLength(0);
  });

  it("the template no longer fits: the preview stays, Back to current and Refresh are offered", async () => {
    const srv = await mount();
    fail(srv, 400, "ERROR_INVALID_LAYOUT", { problems: [] });
    await open();
    await userEvent.click(cardName("Clean title"));
    await userEvent.click(await screen.findByRole("button", { name: "Apply to the cover" }));
    await userEvent.click(confirmBtn());
    expect(await screen.findByText(/no longer fits the book's design/)).toBeVisible();
    expect(screen.getByRole("button", { name: "Back to current" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Refresh" })).toBeEnabled();
    expect(stageText()).toContain("Mẫu sạch"); // still the preview
    await userEvent.click(screen.getByRole("button", { name: "Back to current" }));
    untouched(srv);
  });

  it.each([
    [409, "ERROR_TEMPLATE_MISMATCH", /made for another design of the book/],
    [404, "ERROR_NOT_FOUND", /This template no longer exists/],
  ])(
    "%i %s: its own text, the list is read again, the preview ends",
    async (status, code, text) => {
      const srv = await mount();
      fail(srv, status, code);
      await attempt();
      expect(await screen.findByText(text)).toBeVisible();
      await waitFor(() => expect(srv.s.tcalls.filter((c) => c === "list:cover")).toHaveLength(2));
      await screen.findAllByRole("radio");
      untouched(srv);
    },
  );

  it("rate limited: the Retry-After seconds", async () => {
    const srv = await mount();
    fail(srv, 429, "ERROR_RATE_LIMITED", undefined, { "Retry-After": "12" });
    await attempt();
    expect(await screen.findByText("Too many requests. Try again in 12 s.")).toBeVisible();
    untouched(srv);
  });

  it("offline", async () => {
    const srv = await mount();
    await open();
    await userEvent.click(applyBtn("Clean title"));
    const f = vi.mocked(fetch);
    const real = f.getMockImplementation()!;
    f.mockImplementationOnce(() => Promise.reject(new TypeError("down")));
    await userEvent.click(confirmBtn());
    expect(await screen.findByText(/You are offline/)).toBeVisible();
    f.mockImplementation(real);
    untouched(srv);
    // and a retry works once the network is back
    await userEvent.click(applyBtn("Clean title"));
    await userEvent.click(confirmBtn());
    await waitFor(() => expect(srv.s.applies).toHaveLength(1 + 0));
  });

  it("a failed apply keeps the undo history", async () => {
    const srv = await mount();
    await userEvent.click(addTextButton());
    await act(async () => {});
    fail(srv, 500, "ERROR_INTERNAL");
    await attempt();
    expect(await screen.findByText("Something went wrong. Nothing was changed.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
  });
});

describe("every error has its own text in vi and en", () => {
  it("has the same eight keys, all different, in both languages", () => {
    const keys = ["conflict", "invalid", "mismatch", "gone", "busy", "offline", "unsaved", "other"];
    for (const l of [en, vi_]) {
      expect(Object.keys(l.editor.templates.err)).toEqual(keys);
      expect(new Set(Object.values(l.editor.templates.err)).size).toBe(keys.length);
    }
  });
});

describe("delete an own template", () => {
  it("confirms, deletes, removes the card and announces; a shipped card has no Delete", async () => {
    const srv = await mount();
    await open();
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent(
      "Delete the template 'My cover'? Books that already used it are not changed.",
    );
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByRole("radio", { name: /My cover/ })).toBeNull());
    expect(srv.s.tcalls).toEqual(["list:cover", "delete:01J9Z3K6V8Q4M7N2P5R8OWN001"]);
    expect(live()).toBe("Template 'My cover' deleted.");
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
    expect(screen.getAllByText("No templates for this page yet.")).toHaveLength(1);
  });

  it("a 404 (already gone) refreshes the list", async () => {
    const srv = await mount();
    await open();
    srv.s.templates = srv.s.templates.filter((x) => x.source !== "own");
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }),
    );
    await waitFor(() => expect(screen.queryByRole("radio", { name: /My cover/ })).toBeNull());
    expect(srv.s.tcalls.filter((c) => c === "list:cover")).toHaveLength(2);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("ends a preview of the deleted template", async () => {
    await mount();
    await open();
    await userEvent.click(cardName("My cover"));
    await waitFor(() => expect(stageText()).toContain("Của tôi"));
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }),
    );
    await waitFor(() => expect(screen.queryByText(/Previewing the template/)).toBeNull());
    expect(stageText()).toContain("Bản hiện tại");
  });

  it("shows a failed delete in the dialog and keeps the card", async () => {
    const srv = await mount();
    await open();
    srv.s.deleteScript.push(() => srv.fail(500, "ERROR_INTERNAL"));
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }),
    );
    expect(await within(screen.getByRole("dialog")).findByRole("alert")).toHaveTextContent(
      "Something went wrong",
    );
    expect(screen.getByRole("radio", { name: /My cover/ })).toBeInTheDocument();
  });
});
