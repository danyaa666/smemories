import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import type { LayoutDoc } from "./document";
import { EditorShell } from "./EditorShell";
import { mockServer, type MockVersion } from "./mockServer";

// T-165: the version history panel in the editor: nothing is fetched before it opens, keyboard walk, read-only preview, restore (settle first, one
// request, history cleared), the errors, and the dropped message. The real browser path is tools/e2e/editor-history.spec.mjs.

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const NOW = Date.now();
const cover = (text: string, extra: LayoutDoc["pages"][string]["items"] = []): LayoutDoc => ({
  v: 1,
  pages: {
    cover: {
      items: [{ id: "textAA", t: "text", text, x: 100, w: 300, h: 100, cy: 700 }, ...(extra ?? [])],
    },
  },
});
const CURRENT = cover("Bản hiện tại");
const id = (n: number) => `V${n}`.padEnd(26, "0");
const versions = (): MockVersion[] => [
  {
    id: id(3),
    version: 3,
    kind: "restore",
    created_at: NOW - 12 * 60_000,
    layout: cover("Bản giữa"),
  },
  {
    id: id(2),
    version: 2,
    kind: "auto",
    created_at: NOW - 5 * 3600_000,
    layout: cover("Bản cũ", [
      { id: "goneAA", t: "asset", asset: "sticker.removed-long-ago", x: 1, w: 100, h: 100, cy: 1 },
    ]),
  },
  {
    id: id(1),
    version: 1,
    kind: "reset",
    created_at: NOW - 3 * 86400_000,
    layout: cover("Bản đầu"),
  },
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

const mount = async (v: MockVersion[] = versions()) => {
  const srv = mockServer({ version: 5, layout: CURRENT, versions: v });
  render(
    <MemoryRouter>
      <EditorShell bookId={BOOK} data={data()} safeMm={5} />
    </MemoryRouter>,
  );
  await screen.findByRole("toolbar");
  return srv;
};
const open = async () => {
  await userEvent.click(screen.getByRole("button", { name: /History/ }));
  return screen.findAllByRole("radio");
};
const stageText = () => document.querySelector(".ed-pagebox")?.textContent ?? "";
const live = () => document.querySelector(".visually-hidden[role=status]")?.textContent;
const restoreButton = () => screen.getByRole("button", { name: "Restore this version" });
const confirm = () =>
  within(screen.getByRole("dialog")).getByRole("button", { name: "Restore this version" });

beforeEach(async () => {
  await i18n.changeLanguage("en");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("opening", () => {
  it("fetches nothing before the panel opens, then lists once, newest first, with kind and item count", async () => {
    const srv = await mount();
    await new Promise((r) => setTimeout(r, 50));
    expect(srv.s.history).toEqual([]);
    const rows = await open();
    expect(srv.s.history).toEqual(["list"]);
    expect(rows.map((r) => r.getAttribute("aria-label"))).toEqual([
      "12 minutes ago, Before restore, 1 item",
      "5 hours ago, Autosave, 2 items",
      "3 days ago, Before reset, 1 item",
    ]);
    expect(rows[0]).toHaveAttribute("title");
    expect(screen.getByRole("button", { name: /History/ })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("shows at most 30 rows", async () => {
    const many: MockVersion[] = Array.from({ length: 35 }, (_, i) => ({
      id: id(100 + i),
      version: i + 1,
      kind: "auto",
      created_at: NOW - (i + 1) * 60_000,
      layout: cover(`v${i}`),
    }));
    await mount(many);
    expect(await open()).toHaveLength(30);
  });

  it("shows the empty text and a Refresh that calls the list again", async () => {
    const srv = await mount([]);
    await userEvent.click(screen.getByRole("button", { name: /History/ }));
    expect(await screen.findByText(/No earlier versions yet/)).toBeInTheDocument();
    srv.s.versions.push(...versions());
    await userEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(await screen.findAllByRole("radio")).toHaveLength(3);
    expect(srv.s.history).toEqual(["list", "list"]);
  });

  it("shows an error with a retry when the list fails, and the offline text for a network failure", async () => {
    const srv = await mount();
    const f = vi.mocked(fetch);
    const real = f.getMockImplementation()!;
    f.mockImplementationOnce(() => Promise.reject(new TypeError("down")));
    await userEvent.click(screen.getByRole("button", { name: /History/ }));
    expect(await screen.findByText(/You are offline/)).toBeInTheDocument();
    f.mockImplementation(real);
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findAllByRole("radio")).toHaveLength(3);
    expect(srv.s.history).toEqual(["list"]);
  });
});

describe("keyboard", () => {
  it("walks the rows with the arrows, previews with Enter, Escape leaves the preview, then closes and gives focus back", async () => {
    const srv = await mount();
    const button = screen.getByRole("button", { name: /History/ });
    const rows = await open();
    await userEvent.tab(); // from the panel to Refresh
    await userEvent.tab(); // and to the first row
    expect(rows[0]).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}");
    expect(rows[1]).toHaveFocus();
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    expect(rows[2]).toHaveFocus(); // stops at the last row
    await userEvent.keyboard("{ArrowUp}{Enter}");
    await screen.findByText(/Previewing a version from/);
    expect(srv.s.history).toEqual(["list", `get:${id(2)}`]);
    expect(rows[1]).toHaveAttribute("aria-checked", "true");
    await waitFor(() => expect(stageText()).toContain("Bản cũ"));
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByText(/Previewing a version from/)).toBeNull();
    expect(stageText()).toContain("Bản hiện tại");
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("radiogroup", { name: "Earlier versions" })).toBeNull();
    await waitFor(() => expect(button).toHaveFocus());
  });
});

describe("keyboard restore", () => {
  it("reaches Restore with Shift+Tab from the list and confirms with the keys", async () => {
    const srv = await mount();
    await open();
    await userEvent.tab();
    await userEvent.tab();
    await userEvent.keyboard("{Enter}");
    await screen.findByText(/Previewing a version from/);
    await userEvent.tab({ shift: true }); // Refresh
    await userEvent.tab({ shift: true }); // Back to current
    await userEvent.tab({ shift: true }); // Restore this version
    expect(restoreButton()).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus();
    await userEvent.tab();
    expect(confirm()).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(live()).toContain("Version restored."));
    expect(srv.s.restores).toHaveLength(1);
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.getByRole("button", { name: /History/ })).toHaveFocus());
  });
});

describe("preview", () => {
  it("is read-only, says how many items will be dropped, and caches the body", async () => {
    const srv = await mount();
    const rows = await open();
    await userEvent.click(rows[1]!);
    const bar = await screen.findByText(/Previewing a version from/);
    expect(bar).toHaveTextContent("1 item will be dropped");
    expect(bar.closest("[role=status]")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Add text" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Back to current" }));
    expect(stageText()).toContain("Bản hiện tại");
    await userEvent.click(rows[1]!); // second time: from the cache
    await screen.findByText(/Previewing a version from/);
    expect(srv.s.history.filter((h) => h.startsWith("get:"))).toEqual([`get:${id(2)}`]);
    expect(srv.s.saves).toHaveLength(0); // the preview never saves
  });

  it("closing the panel while previewing returns to the current document", async () => {
    await mount();
    const rows = await open();
    await userEvent.click(rows[0]!);
    await screen.findByText(/Previewing a version from/);
    await userEvent.click(screen.getByRole("button", { name: /History/ }));
    expect(screen.queryByText(/Previewing a version from/)).toBeNull();
    expect(stageText()).toContain("Bản hiện tại");
  });

  it("refreshes the list when the version is gone (404)", async () => {
    const srv = await mount();
    const rows = await open();
    srv.s.versions.splice(0, 1);
    await userEvent.click(rows[0]!);
    expect(await screen.findByText(/no longer exists/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getAllByRole("radio")).toHaveLength(2));
  });
});

describe("restore", () => {
  it("confirms, restores with the store's version, adopts, clears undo, announces and refreshes the list", async () => {
    const srv = await mount();
    const rows = await open();
    await userEvent.click(rows[0]!);
    await userEvent.click(await screen.findByRole("button", { name: "Restore this version" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("Your current version is kept in the history");
    expect(dialog).toHaveTextContent("Undo and redo start again");
    await userEvent.click(confirm());
    await waitFor(() => expect(live()).toContain("Version restored."));
    expect(srv.s.restores).toMatchObject([{ id: id(3), base_version: 5 }]);
    expect(srv.s.version).toBe(6);
    expect(screen.queryByText(/Previewing a version from/)).toBeNull();
    expect(stageText()).toContain("Bản giữa");
    expect(screen.getByRole("button", { name: "Undo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Redo" })).toBeDisabled();
    expect(screen.getByText("Version restored.", { selector: ".banner-text" })).toBeVisible();
    await waitFor(() => expect(screen.getAllByRole("radio")).toHaveLength(4)); // the replaced document is a new row
    expect(screen.getAllByRole("radio")[0]).toHaveAccessibleName(/Before restore/);
    expect(srv.s.history.at(-1)).toBe("list");
  });

  it("sends one request when the button is clicked twice", async () => {
    const srv = await mount();
    let release!: () => void;
    const f = vi.mocked(fetch);
    const real = f.getMockImplementation()!;
    const hold = new Promise<void>((r) => (release = r));
    f.mockImplementation(async (url, req) => {
      if (String(url) === "/api/book-layout/restore") await hold;
      return real(url, req);
    });
    const rows = await open();
    await userEvent.click(rows[0]!);
    await userEvent.click(await screen.findByRole("button", { name: "Restore this version" }));
    const b = confirm();
    fireEvent.click(b);
    fireEvent.click(b);
    release();
    await waitFor(() => expect(live()).toContain("Version restored."));
    expect(srv.s.restores).toHaveLength(1);
  });

  it("saves a pending edit first, so the restore asks with the version after that save", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const srv = await mount();
    fireEvent.click(screen.getByRole("button", { name: "Cover title" }));
    const w = screen.getByLabelText("Width") as HTMLInputElement;
    fireEvent.change(w, { target: { value: "40" } });
    fireEvent.keyDown(w, { key: "Enter" });
    const rows = await open();
    await userEvent.click(rows[0]!);
    await userEvent.click(await screen.findByRole("button", { name: "Restore this version" }));
    await userEvent.click(confirm());
    await waitFor(() => expect(live()).toContain("Version restored."));
    expect(srv.s.calls).toEqual(["save"]);
    expect(srv.s.restores).toMatchObject([{ id: id(3), base_version: 6 }]); // 5 + the save
  });

  it("says how many items could not be restored, in the toast and the live region", async () => {
    const srv = await mount();
    srv.s.dropped = 2;
    const rows = await open();
    await userEvent.click(rows[1]!);
    await userEvent.click(await screen.findByRole("button", { name: "Restore this version" }));
    await userEvent.click(confirm());
    await waitFor(() =>
      expect(live()).toContain(
        "2 items could not be restored because their image or sticker no longer exists",
      ),
    );
    expect(
      screen.getByText(/2 items could not be restored/, { selector: ".banner-text" }),
    ).toBeVisible();
  });

  const fail = async (
    answer: (s: ReturnType<typeof mockServer>) => Response | Promise<Response>,
  ) => {
    const srv = await mount();
    srv.s.restoreScript.push(() => answer(srv));
    const rows = await open();
    await userEvent.click(rows[0]!);
    await userEvent.click(await screen.findByRole("button", { name: "Restore this version" }));
    await userEvent.click(confirm());
    await waitFor(() => expect(srv.s.restores).toHaveLength(1));
    return srv;
  };
  it("invalid: its own text, the preview stays with Back to current, nothing changed", async () => {
    await fail((s) =>
      s.fail(400, "ERROR_INVALID_LAYOUT", { problems: [{ path: "x", code: "y" }] }),
    );
    expect(await screen.findByText(/no longer fits the template/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Back to current" })).toBeEnabled();
    expect(restoreButton()).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: "Back to current" }));
    expect(stageText()).toContain("Bản hiện tại");
  });
  it("template changed", async () => {
    await fail((s) => s.fail(409, "ERROR_VERSION_TEMPLATE_CHANGED"));
    expect(await screen.findByText(/made for another template/)).toBeInTheDocument();
  });
  it("rate limit: the retry time", async () => {
    await fail((s) => s.fail(429, "ERROR_RATE_LIMITED", undefined, { "Retry-After": "7" }));
    expect(await screen.findByText(/Try again in 7 s/)).toBeInTheDocument();
  });
  it("offline", async () => {
    await fail(() => Promise.reject(new TypeError("down")));
    expect(await screen.findByText(/You are offline/)).toBeInTheDocument();
    expect(restoreButton()).toBeEnabled();
  });
  it("conflict: the editor's conflict banner takes over and nothing is overwritten", async () => {
    const srv = await fail((s) =>
      s.fail(409, "ERROR_VERSION_CONFLICT", { version: 9, layout: cover("Từ tab khác") }),
    );
    expect(await screen.findByText(/Editing is paused/)).toBeInTheDocument();
    expect(screen.getByText(/another tab or device, so nothing was restored/)).toBeInTheDocument();
    expect(srv.s.layout).toEqual(CURRENT);
    await userEvent.click(screen.getByRole("button", { name: "Use the saved version" }));
    await waitFor(() => expect(stageText()).toContain("Từ tab khác"));
  });
});

describe("vietnamese", () => {
  it("has the button, the kind labels and the preview text in Vietnamese", async () => {
    await i18n.changeLanguage("vi");
    await mount();
    const rows = await userEvent
      .click(screen.getByRole("button", { name: /Lịch sử/ }))
      .then(() => screen.findAllByRole("radio"));
    expect(rows[0]).toHaveAccessibleName(/Trước khi khôi phục/);
    await userEvent.click(rows[0]!);
    expect(await screen.findByText(/Đang xem trước phiên bản lúc/)).toBeInTheDocument();
    await act(async () => {});
  });
});
