import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import type { LayoutDoc } from "./document";
import { EditorShell } from "./EditorShell";
import { mockServer, type MockTemplate } from "./mockServer";

// T-176: "Save this page as a template" in the Templates panel: the button and its reasons, the name dialog and its rules, settle first, one request,
// the list read again with the new card focused, a save that changes nothing in the editor, and every error. The real browser path is
// tools/e2e/editor-save-template.spec.mjs; the model (name rules, error mapping, state machine) is in templates.test.ts.

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const MEDIA = "01J9Z3K6V8Q4M7N2P5R8M00001";
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
const own = (id: string, name: string, created_at: number): MockTemplate => ({
  id,
  source: "own",
  label_en: name,
  label_vi: name,
  kind: "cover",
  item_count: 1,
  created_at,
  page: { items: [textItem(`t${created_at}`.padEnd(8, "0"), name)] },
});
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

const mount = async (opts: { layout?: LayoutDoc | null; templates?: MockTemplate[] } = {}) => {
  const srv = mockServer({
    version: 5,
    layout: opts.layout === undefined ? CURRENT : opts.layout,
    templates: opts.templates ?? [own("01J9Z3K6V8Q4M7N2P5R8OWN001", "My cover", 5)],
  });
  render(
    <MemoryRouter>
      <EditorShell bookId={BOOK} data={data()} safeMm={5} />
    </MemoryRouter>,
  );
  await screen.findByRole("toolbar");
  return srv;
};
const templatesButton = () => screen.getByRole("button", { name: /^Templates$/ });
const saveButton = () => screen.getByRole("button", { name: "Save this page as a template" });
const open = async () => {
  await userEvent.click(templatesButton());
  await screen.findAllByRole("radio");
};
const dialog = () => screen.getByRole("dialog", { name: "Save this page as a template" });
const nameField = () => within(dialog()).getByLabelText("Template name") as HTMLInputElement;
const setName = (v: string) => fireEvent.change(nameField(), { target: { value: v } });
const confirm = () => within(dialog()).getByRole("button", { name: "Save" });
const live = () => document.querySelector(".visually-hidden[role=status]")?.textContent;
const toast = () => document.querySelector<HTMLElement>(".banner")?.textContent;
const note = () => document.getElementById("ed-tpl-save-note")?.textContent;

beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the button", () => {
  it("is in the panel next to Refresh, reachable by Tab, and opens nothing before it is pressed", async () => {
    const srv = await mount();
    await open();
    expect(saveButton().parentElement).toContainElement(
      screen.getByRole("button", { name: "Refresh" }),
    );
    expect(saveButton()).not.toHaveAttribute("aria-disabled");
    expect(note()).toBe("");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(srv.s.creates).toEqual([]);
  });

  it("explains instead of opening when the page has no changes yet", async () => {
    const srv = await mount({ layout: null });
    await open();
    expect(saveButton()).toHaveAttribute("aria-disabled", "true");
    expect(note()).toBe("Nothing to save: this page has no changes yet");
    await userEvent.click(saveButton());
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(srv.s.creates).toEqual([]);
  });

  it("explains instead of opening while the browser is offline, and again when it is back", async () => {
    await mount();
    await open();
    const online = vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    window.dispatchEvent(new Event("offline"));
    await waitFor(() => expect(saveButton()).toHaveAttribute("aria-disabled", "true"));
    expect(note()).toMatch(/You are offline/);
    await userEvent.click(saveButton());
    expect(screen.queryByRole("dialog")).toBeNull();
    online.mockReturnValue(true);
    window.dispatchEvent(new Event("online"));
    await waitFor(() => expect(saveButton()).not.toHaveAttribute("aria-disabled"));
  });

  it("explains instead of opening while a template is previewed (the stage is not the stored page)", async () => {
    await mount({
      templates: [own("01J9Z3K6V8Q4M7N2P5R8OWN001", "My cover", 5)],
    });
    await open();
    await userEvent.click(screen.getByRole("radio", { name: /^Template My cover,/ }));
    await screen.findByText(/Previewing the template/);
    expect(saveButton()).toHaveAttribute("aria-disabled", "true");
    expect(note()).toMatch(/Go back to your current page/);
    await userEvent.click(screen.getByRole("button", { name: "Back to current" }));
    expect(saveButton()).not.toHaveAttribute("aria-disabled");
  });

  it("shows the per-account limit instead of an invented count", async () => {
    await mount();
    await open();
    const group = screen.getByRole("group", { name: "My templates" });
    expect(group).toHaveTextContent("Up to 20 own templates per account, across all page types.");
    expect(group.textContent).not.toMatch(/\d+ of 20/);
  });
});

describe("the dialog", () => {
  it("opens with a default name, the summary and the image hint, focus in the name field; Esc cancels and returns focus", async () => {
    const layout: LayoutDoc = {
      v: 1,
      pages: {
        cover: {
          bg: { color: "gold" },
          overrides: { "cover.title": { w: 5 } },
          items: [
            textItem("textAA", "Bản hiện tại"),
            { id: "imgAAA", t: "image", media: MEDIA, x: 1, w: 100, h: 100, cy: 1 },
            { id: "imgBBB", t: "image", media: MEDIA, x: 1, w: 100, h: 100, cy: 1 },
          ],
        },
      },
    };
    const srv = await mount({ layout });
    await open();
    await userEvent.click(saveButton());
    expect(nameField()).toHaveFocus();
    expect(nameField().value).toBe("My cover 2"); // one own template of the cover exists
    expect(dialog()).toHaveTextContent(
      "This saves the cover as it is now: 1 item, 1 moved element, a background. 2 images are not saved (your own images are not copied to templates).",
    );
    expect(dialog()).toHaveTextContent("10 of 60 characters");
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(saveButton()).toHaveFocus();
    expect(srv.s.creates).toEqual([]);
  });

  it("names the default in the language of the app", async () => {
    await i18n.changeLanguage("vi");
    await mount();
    await userEvent.click(screen.getByRole("button", { name: /^Mẫu trang$/ }));
    await screen.findAllByRole("radio");
    await userEvent.click(screen.getByRole("button", { name: "Lưu trang này thành mẫu" }));
    expect(within(screen.getByRole("dialog")).getByLabelText("Tên mẫu")).toHaveValue(
      "Bìa của tôi 2",
    );
  });

  it("refuses an empty and a 61-code-point name inline and sends nothing; 60 passes", async () => {
    const srv = await mount();
    await open();
    await userEvent.click(saveButton());
    setName("   ");
    await userEvent.click(confirm());
    expect(within(dialog()).getByRole("alert")).toHaveTextContent("Enter a name for the template.");
    expect(nameField()).toHaveAttribute("aria-invalid", "true");
    setName("ệ".repeat(61));
    expect(dialog()).toHaveTextContent("61 of 60 characters");
    await userEvent.click(confirm());
    expect(within(dialog()).getByRole("alert")).toHaveTextContent("Use at most 60 characters.");
    expect(srv.s.creates).toEqual([]);
    setName("ệ".repeat(60)); // 180 code points typed decomposed, 60 letters
    expect(dialog()).toHaveTextContent("60 of 60 characters");
    await userEvent.click(confirm());
    await waitFor(() => expect(srv.s.creates).toHaveLength(1));
    expect(srv.s.creates[0]!.name).toBe("ệ".repeat(60)); // sent composed, trimmed
  });

  it("Enter saves, Cancel closes without a request", async () => {
    const srv = await mount();
    await open();
    await userEvent.click(saveButton());
    await userEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    expect(srv.s.creates).toEqual([]);
    await userEvent.click(saveButton());
    await userEvent.clear(nameField());
    await userEvent.type(nameField(), "Bìa của tôi{Enter}");
    await waitFor(() => expect(srv.s.creates).toHaveLength(1));
    expect(srv.s.creates[0]).toEqual({ yearbook_id: BOOK, kind: "cover", name: "Bìa của tôi" });
  });

  it("traps Tab inside the dialog", async () => {
    await mount();
    await open();
    await userEvent.click(saveButton());
    await userEvent.tab(); // Cancel
    await userEvent.tab(); // Save
    expect(confirm()).toHaveFocus();
    await userEvent.tab(); // wraps to the first control
    expect(nameField()).toHaveFocus();
    await userEvent.tab({ shift: true });
    expect(confirm()).toHaveFocus();
  });
});

describe("saving", () => {
  it("settles first, sends one request on a double click, closes, refreshes the list, focuses the new card and announces", async () => {
    const srv = await mount();
    await open();
    await userEvent.click(saveButton());
    setName("Bìa mới");
    await userEvent.dblClick(confirm());
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(srv.s.creates).toEqual([{ yearbook_id: BOOK, kind: "cover", name: "Bìa mới" }]);
    const card = await screen.findByRole("radio", {
      name: "Template Bìa mới, 1 item, my template, new",
    });
    await waitFor(() => expect(card).toHaveFocus());
    expect(srv.s.tcalls).toEqual(["list:cover", "create:cover:Bìa mới", "list:cover"]);
    expect(live()).toBe("Template 'Bìa mới' saved.");
    expect(toast()).toContain("Template 'Bìa mới' saved.");
    expect(toast()).not.toMatch(/Undo in History/);
    // newest first inside "My templates"
    const names = within(screen.getByRole("group", { name: "My templates" }))
      .getAllByRole("radio")
      .map((r) => r.getAttribute("aria-label"));
    expect(names[0]).toMatch(/^Template Bìa mới,/);
  });

  it("saves what the student sees: a pending edit is saved first, and the editor is otherwise untouched", async () => {
    const srv = await mount();
    const seen: number[] = [];
    srv.s.createScript.push(() => {
      seen.push(srv.s.saves.length); // saves made by the time template-create arrives
      return srv.body({
        id: "01J9Z3K6V8Q4M7N2P5R8C00099",
        name: "X",
        kind: "cover",
        template_id: "memory-book",
        item_count: 2,
        bytes: 1,
        created_at: 1,
        stripped: 0,
      });
    });
    await userEvent.click(
      within(screen.getByRole("toolbar", { name: "Editor toolbar" })).getByRole("button", {
        name: "Add text",
      }),
    );
    await userEvent.keyboard("{Escape}");
    const undoWasEnabled = !(screen.getByRole("button", { name: "Undo" }) as HTMLButtonElement)
      .disabled;
    expect(undoWasEnabled).toBe(true);
    const textBefore = document.querySelector(".ed-pagebox")?.textContent;
    await open();
    await userEvent.click(saveButton());
    await userEvent.click(confirm());
    await waitFor(() => expect(srv.s.creates).toHaveLength(1));
    expect(seen).toEqual([1]); // the edit was saved before the template was made
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(srv.s.saves).toHaveLength(1); // saving a template wrote nothing else
    expect(srv.s.version).toBe(6);
    expect(document.querySelector(".ed-pagebox")?.textContent).toBe(textBefore);
    expect((screen.getByRole("button", { name: "Undo" }) as HTMLButtonElement).disabled).toBe(
      false,
    ); // undo stack kept
  });

  it("says how many images were not saved, from the server's count", async () => {
    const layout: LayoutDoc = {
      v: 1,
      pages: {
        cover: {
          items: [
            textItem("textAA", "Bản hiện tại"),
            { id: "imgAAA", t: "image", media: MEDIA, x: 1, w: 100, h: 100, cy: 1 },
          ],
        },
      },
    };
    const srv = await mount({ layout });
    await open();
    await userEvent.click(saveButton());
    expect(dialog()).toHaveTextContent("1 image is not saved");
    await userEvent.click(confirm());
    await waitFor(() => expect(live()).toBe("Template 'My cover 2' saved. 1 image was not saved."));
    expect(srv.s.templates.find((x) => x.label_en === "My cover 2")!.item_count).toBe(1);
  });

  it("keeps the dialog and the typed name on every error, each with its own text", async () => {
    const srv = await mount();
    await open();
    const cases: [() => Promise<Response> | Response, RegExp][] = [
      [
        () => srv.fail(413, "ERROR_TOO_LARGE"),
        /This page is too big to keep as a template\. Remove some items/,
      ],
      [
        () =>
          srv.fail(400, "ERROR_INVALID_LAYOUT", { problems: [{ path: "items[0]", code: "x" }] }),
        /This page could not be saved as a template\. Nothing was changed\./,
      ],
      [() => srv.fail(400, "ERROR_PARAM"), /The name was not accepted/],
      [() => srv.fail(404, "ERROR_NOT_FOUND"), /This book no longer exists/],
      [
        () => srv.fail(429, "ERROR_RATE_LIMITED", undefined, { "Retry-After": "7" }),
        /Try again in 7 s/,
      ],
      [() => Promise.reject(new TypeError("down")) as never, /You are offline/],
      [() => srv.fail(500, "ERROR_INTERNAL"), /Something went wrong\. Nothing was changed\./],
    ];
    await userEvent.click(saveButton());
    setName("Tên tôi gõ");
    for (const [step, text] of cases) {
      srv.s.createScript.push(step as never);
      await userEvent.click(confirm());
      await waitFor(() => expect(within(dialog()).getByRole("alert")).toHaveTextContent(text));
      expect(nameField()).toHaveValue("Tên tôi gõ");
    }
    expect(srv.s.creates).toHaveLength(cases.length);
    expect(srv.s.saves).toHaveLength(0); // nothing in the editor changed
    expect(toast()).toBeUndefined();
    // "invalid" offers Back instead of Cancel
    srv.s.createScript.push(() => srv.fail(400, "ERROR_INVALID_LAYOUT"));
    await userEvent.click(confirm());
    await userEvent.click(await within(dialog()).findByRole("button", { name: "Back" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("the 21st template: its own message with the limit, the dialog keeps the name, and the button then explains until one is deleted", async () => {
    const full = Array.from({ length: 20 }, (_, i) =>
      own(`01J9Z3K6V8Q4M7N2P5R8OWN${String(i).padStart(3, "0")}`, `Mẫu ${i}`, i + 1),
    );
    const srv = await mount({ templates: full });
    await open();
    const scrolled = vi.fn();
    Element.prototype.scrollIntoView = scrolled;
    await userEvent.click(saveButton());
    setName("Cái thứ 21");
    await userEvent.click(confirm());
    const msg =
      "You have 20 own templates, the most you can keep across all page types. Delete one in the panel first.";
    await waitFor(() => expect(within(dialog()).getByRole("alert")).toHaveTextContent(msg));
    expect(nameField()).toHaveValue("Cái thứ 21");
    expect(scrolled).toHaveBeenCalled(); // the own group is brought into view
    await userEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    await userEvent.click(saveButton()); // bound to fail: no dialog, the reason instead
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(note()).toBe(msg);
    expect(srv.s.creates).toHaveLength(1);
    // delete one: the next save is allowed and goes through
    await userEvent.click(
      within(screen.getByRole("group", { name: "My templates" })).getAllByRole("button", {
        name: "Delete",
      })[0]!,
    );
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete" }),
    );
    await waitFor(() => expect(note()).toBe(""));
    await userEvent.click(saveButton());
    setName("Cái thứ 21");
    await userEvent.click(confirm());
    await waitFor(() => expect(srv.s.creates).toHaveLength(2));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView;
  });

  it("sends nothing when the newest edit cannot be saved", async () => {
    const srv = await mount();
    await userEvent.click(
      within(screen.getByRole("toolbar", { name: "Editor toolbar" })).getByRole("button", {
        name: "Add text",
      }),
    );
    await userEvent.keyboard("{Escape}");
    srv.s.script.push(() => Promise.reject(new TypeError("down")) as never); // the autosave fails
    await open();
    await userEvent.click(saveButton());
    await userEvent.click(confirm());
    await waitFor(() =>
      expect(within(dialog()).getByRole("alert")).toHaveTextContent(/could not be saved yet/),
    );
    expect(srv.s.creates).toEqual([]);
  });
});
