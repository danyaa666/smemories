import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import { EditorShell } from "./EditorShell";
import { mockServer } from "./mockServer";

// T-170: page by page, Done, the print label and the deep link of the editor. The shell sits in a tiny router so a navigation can be seen.

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const data = (design: BookData["design"] = null): BookData => ({
  design,
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

const Where = () => <p data-testid="where">{useLocation().pathname + useLocation().search}</p>;
const mount = async (
  opts: { design?: BookData["design"]; url?: string; onDone?: () => void } = {},
) => {
  const srv = mockServer({ version: 2, layout: null });
  render(
    <MemoryRouter initialEntries={[opts.url ?? `/yearbooks/${BOOK}/editor`]}>
      <Where />
      <Routes>
        <Route
          path="/yearbooks/:id/editor"
          element={
            <EditorShell bookId={BOOK} data={data(opts.design)} safeMm={5} onDone={opts.onDone} />
          }
        />
        <Route path="/yearbooks/:id" element={<p>manage page</p>} />
      </Routes>
    </MemoryRouter>,
  );
  await screen.findByRole("toolbar");
  return srv;
};
const sheetKind = () => document.querySelector(".ed-pagebox .bk-sheet")?.getAttribute("data-page");
const position = () => screen.getByTestId("page-position").textContent;
const live = () => document.querySelector(".visually-hidden[role=status]")?.textContent;
const hidden = (...h: string[]) =>
  ({ sections: { order: ["cover", "profile", "notes", "back"], hidden: h } }) as BookData["design"];

beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("page by page", () => {
  it("steps through the real book order with the position text and the live region", async () => {
    await mount();
    expect(position()).toBe("Page 1 of 5: Cover");
    const next = screen.getByRole("button", { name: "Next page" });
    const prev = screen.getByRole("button", { name: "Previous page" });
    expect(prev).toHaveAttribute("aria-disabled", "true");
    const seen: (string | null | undefined)[] = [];
    for (let i = 0; i < 4; i++) {
      await userEvent.click(next);
      seen.push(sheetKind());
    }
    expect(seen).toEqual(["profile", "notes", "notes", "back"]);
    expect(position()).toBe("Page 5 of 5: Back cover");
    expect(live()).toBe("Page 5 of 5: Back cover");
    expect(next).toHaveAttribute("aria-disabled", "true");
    await userEvent.click(next); // at the end: nothing happens
    expect(position()).toBe("Page 5 of 5: Back cover");
    await userEvent.click(prev);
    expect(sheetKind()).toBe("notes");
  });

  it("skips a hidden section", async () => {
    await mount({ design: hidden("profile") });
    expect(position()).toBe("Page 1 of 4: Cover");
    await userEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(sheetKind()).toBe("notes");
    expect(position()).toBe("Page 2 of 4: Notes page 1");
  });

  it("steps with Page Down and Page Up, but not while a number field has the keys", async () => {
    await mount();
    fireEvent.keyDown(document.body, { key: "PageDown" });
    expect(sheetKind()).toBe("profile");
    fireEvent.keyDown(document.body, { key: "PageUp" });
    expect(sheetKind()).toBe("cover");
    const input = document.createElement("input");
    document.body.append(input);
    fireEvent.keyDown(input, { key: "PageDown" });
    expect(sheetKind()).toBe("cover");
    input.remove();
  });

  it("clears the selection on a page change and keeps the undo history", async () => {
    await mount();
    await userEvent.click(screen.getByRole("button", { name: "Cover title" }));
    expect(screen.getByRole("button", { name: "Cover title" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    fireEvent.change(screen.getByLabelText("Width"), { target: { value: "40" } });
    fireEvent.keyDown(screen.getByLabelText("Width"), { key: "Enter" });
    await userEvent.click(screen.getByRole("button", { name: "Next page" }));
    await userEvent.click(screen.getByRole("button", { name: "Previous page" }));
    expect(screen.getByRole("button", { name: "Cover title" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
  });
});

describe("deep link", () => {
  it.each([
    ["profile", "Page 2 of 5: Profile"],
    ["notes", "Page 3 of 5: Notes page 1"],
    ["back", "Page 5 of 5: Back cover"],
    ["cover", "Page 1 of 5: Cover"],
    ["bogus", "Page 1 of 5: Cover"],
  ])("?page=%s opens %s", async (page, text) => {
    await mount({ url: `/yearbooks/${BOOK}/editor?page=${page}` });
    expect(position()).toBe(text);
  });

  it("a hidden section falls back to the cover", async () => {
    await mount({ design: hidden("profile"), url: `/yearbooks/${BOOK}/editor?page=profile` });
    expect(position()).toBe("Page 1 of 4: Cover");
  });
});

// The page list can change after the first render (cached book-data answers first, a fresh answer follows): the REQUESTED page is resolved
// against the current list on every render (the remount race QA found with browser Back, T-170 round 2).
describe("the page list changes after the first render", () => {
  const without = (d: BookData): BookData => ({
    ...d,
    notes: [],
    note_count: 0,
    approved_total: 0,
  });
  const shell = (d: BookData) => (
    <MemoryRouter initialEntries={[`/yearbooks/${BOOK}/editor?page=notes`]}>
      <Routes>
        <Route
          path="/yearbooks/:id/editor"
          element={<EditorShell bookId={BOOK} data={d} safeMm={5} />}
        />
      </Routes>
    </MemoryRouter>
  );
  const open = async (d: BookData) => {
    mockServer({ version: 2, layout: null });
    const r = render(shell(d));
    await screen.findByRole("toolbar");
    return r;
  };

  it("a notes link on stale data without notes opens the cover, then the notes page when the fresh data has one", async () => {
    const r = await open(without(data()));
    expect(position()).toBe("Page 1 of 3: Cover");
    r.rerender(shell(data()));
    expect(position()).toBe("Page 3 of 5: Notes page 1");
    expect(screen.getByTestId("notes-shared")).toBeInTheDocument();
  });

  it("once the user moved, fresh data does not move them", async () => {
    const r = await open(without(data()));
    await userEvent.click(screen.getByRole("button", { name: "Next page" })); // profile
    expect(position()).toBe("Page 2 of 3: Profile");
    r.rerender(shell(data()));
    expect(position()).toBe("Page 2 of 5: Profile");
  });

  it("the user's notes page stays when the notes pages become fewer", async () => {
    const r = await open(data()); // the link opens notes page 1
    await userEvent.click(screen.getByRole("button", { name: "Next page" })); // notes page 2
    expect(position()).toBe("Page 4 of 5: Notes page 2");
    r.rerender(shell({ ...data(), notes: data().notes.slice(0, 1) }));
    expect(position()).toBe("Page 3 of 4: Notes page 1");
  });
});

describe("notes pages share one layout (ADR 0004)", () => {
  it("says so on a notes page, in the live region on arrival, and only there", async () => {
    await mount();
    expect(screen.queryByTestId("notes-shared")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Next page" })); // profile
    await userEvent.click(screen.getByRole("button", { name: "Next page" })); // notes page 1
    expect(screen.getByTestId("notes-shared")).toHaveTextContent("applies to every notes page");
    expect(live()).toMatch(/Notes page 1\. This layout applies to every notes page/);
    await userEvent.click(screen.getByRole("button", { name: "Next page" })); // notes page 2
    expect(screen.getByTestId("notes-shared")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Next page" })); // back
    expect(screen.queryByTestId("notes-shared")).toBeNull();
  });

  it("shows it when a deep link opens a notes page", async () => {
    await mount({ url: `/yearbooks/${BOOK}/editor?page=notes` });
    expect(screen.getByTestId("notes-shared")).toBeInTheDocument();
  });
});

const edit = async () => {
  await userEvent.click(screen.getByRole("button", { name: "Cover title" }));
  const w = screen.getByRole("spinbutton", { name: "Width" });
  fireEvent.change(w, { target: { value: "40" } });
  fireEvent.keyDown(w, { key: "Enter" });
};

describe("Done", () => {
  it("with nothing to save goes straight to the manage page", async () => {
    const onDone = vi.fn();
    const srv = await mount({ onDone });
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(await screen.findByText("manage page")).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent(`/yearbooks/${BOOK}`);
    expect(srv.s.saves).toHaveLength(0);
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it("does not navigate before the save settled, then navigates", async () => {
    const srv = await mount();
    let release!: () => void;
    const held = new Promise<void>((r) => (release = r));
    srv.s.script.push(async () => {
      await held;
      srv.s.version++;
      return srv.body({ version: srv.s.version, updated_at: 1 });
    });
    await edit();
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(srv.s.saves).toHaveLength(1));
    expect(screen.queryByText("manage page")).toBeNull(); // the save is still in flight
    release();
    expect(await screen.findByText("manage page")).toBeInTheDocument();
    expect(srv.s.saves[0]?.layout.pages.cover?.overrides?.["cover.title"]?.w).toBe(400);
  });

  it("stays in the editor with the message when the save is refused (409), and loses nothing", async () => {
    const onDone = vi.fn();
    const srv = await mount({ onDone });
    srv.s.script.push(() => srv.fail(409, "ERROR_VERSION_CONFLICT", { version: 9, layout: null }));
    await edit();
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(
        screen.getAllByRole("alert").some((a) => /editor stays open/.test(a.textContent ?? "")),
      ).toBe(true),
    );
    expect(screen.getByRole("toolbar")).toBeInTheDocument();
    expect(screen.queryByText("manage page")).toBeNull();
    expect(onDone).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Done" })).toBeEnabled(); // it can be pressed again
    expect(screen.getAllByText(/changed in another tab/).length).toBeGreaterThan(0); // the existing conflict banner
  });

  it("stays when the server cannot save (offline)", async () => {
    const srv = await mount();
    srv.s.script.push(() => Promise.reject(new TypeError("offline")));
    await edit();
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(
        screen.getAllByRole("alert").some((a) => /editor stays open/.test(a.textContent ?? "")),
      ).toBe(true),
    );
    expect(screen.queryByText("manage page")).toBeNull();
  });
});

describe("leave guard while dirty", () => {
  it("asks before an in-app link is followed and stays on Stay", async () => {
    await mount();
    await edit();
    const a = document.createElement("a");
    a.href = "/yearbooks";
    a.textContent = "elsewhere";
    document.body.append(a);
    await userEvent.click(a);
    expect(await screen.findByText("Leave the editor?")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Stay" }));
    expect(screen.getByTestId("where")).toHaveTextContent("/editor");
    a.remove();
  });

  it("asks before the tab is closed while dirty and not when everything is saved", async () => {
    const srv = await mount();
    await edit();
    const ev = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    await screen.findByText("manage page");
    expect(srv.s.saves).toHaveLength(1);
    const after = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(after);
    expect(after.defaultPrevented).toBe(false);
  });
});

describe("print", () => {
  it("is labelled, says the page size and that all pages print", async () => {
    await mount();
    const a = screen.getByRole("link", { name: "Print or save as PDF" });
    expect(a).toHaveAccessibleDescription("Prints all pages of the book, A5 (148 × 210 mm).");
    expect(screen.queryByText("Preview as PDF")).toBeNull();
  });
});
