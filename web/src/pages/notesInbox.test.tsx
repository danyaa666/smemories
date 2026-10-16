import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { NoteItem, NoteStatus } from "../api/notes";
import { err, mockApi, ok, renderApp, user, type Call } from "../test/api";
import { UNDO_MS } from "./NotesInbox";

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const nid = (n: number) => `01J9Z3K6V8Q4M7N2P5R8T0N${String(n).padStart(3, "0")}`;
const PHOTO = "01J9Z3K6V8Q4M7N2P5R8T0PPPP";

const note = (n: number, status: NoteStatus, over: Partial<NoteItem> = {}): NoteItem => ({
  id: nid(n),
  status,
  answers: { name: `Friend ${n}`, relationship: "classmate", message: `Message ${n}` },
  photos: [],
  link_label: "",
  created_at: Date.now() - 2 * 3600_000,
  position: null,
  ...over,
});

/** matchMedia that answers the (min-width: 960px) query for a window of this width. */
function viewport(width: number) {
  vi.stubGlobal("matchMedia", (q: string) => ({
    matches: width >= Number(/min-width:\s*(\d+)px/.exec(q)?.[1]),
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

const yearbook = {
  id: BOOK,
  title: "Class 12A1",
  school_name: "",
  class_name: "",
  graduation_year: 2026,
  motto: "",
  language: "en",
  page_size: "A5",
  template_id: null,
  cover_media_id: null,
  profile: {
    id: "01J9Z3K6V8Q4M7N2P5R8T0BBBB",
    is_owner: true,
    full_name: "",
    nickname: "",
    birthday: null,
    quote: "",
    hobbies: "",
    future_plans: "",
    photo_media_id: null,
  },
  created_at: 1760000000000,
  updated_at: 1760086400000,
};

/** A small in-memory API: lists with paging and counts, moderation writes, optional failures. */
function fake(initial: NoteItem[], opts: { pageSize?: number } = {}) {
  let notes = initial.map((n) => ({ ...n }));
  const fail: Record<string, Response | undefined> = {};
  const log: Call[] = [];
  const counts = () => ({
    all: notes.length,
    pending: notes.filter((n) => n.status === "pending").length,
    approved: notes.filter((n) => n.status === "approved").length,
    hidden: notes.filter((n) => n.status === "hidden").length,
  });
  const renumber = () => {
    let p = 0;
    notes = notes.map((n) => ({ ...n, position: n.status === "approved" ? ++p : null }));
  };
  renumber();
  const route = (key: string, call: Call): Response => {
    log.push(call);
    const path = call.path.split("?")[0]!;
    const q = new URLSearchParams(call.path.split("?")[1] ?? "");
    const failing = fail[`${call.method} ${path}`];
    if (failing) return failing.clone();
    if (key === "GET /user/get-me") return ok(user());
    if (key.startsWith("GET /yearbook/get?")) return ok(yearbook);
    if (call.method === "GET" && path === "/note/get-list") {
      const status = q.get("status");
      const rows = notes.filter((n) => !status || n.status === status);
      const size = Math.min(Number(q.get("limit") ?? 50), opts.pageSize ?? 100);
      const from = Number(q.get("next_id") ?? 0);
      const items = rows.slice(from, from + size);
      const next = from + size < rows.length ? String(from + size) : "";
      return ok({ items, next_id: next, counts: counts() });
    }
    const body = call.body as { ids: string[]; status?: NoteStatus };
    if (path === "/note/batch-update-status") {
      notes = notes.map((n) => (body.ids.includes(n.id) ? { ...n, status: body.status! } : n));
      renumber();
      return ok({ counts: counts() });
    }
    if (path === "/note/batch-delete") {
      notes = notes.filter((n) => !body.ids.includes(n.id));
      renumber();
      return ok({ counts: counts() });
    }
    if (path === "/note/reorder") {
      const approved = notes.filter((n) => n.status === "approved").map((n) => n.id);
      if (body.ids.length !== approved.length || !body.ids.every((i) => approved.includes(i)))
        return err(409, "order_stale");
      notes = body.ids
        .map((i) => notes.find((n) => n.id === i)!)
        .concat(notes.filter((n) => n.status !== "approved"));
      renumber();
      return ok();
    }
    throw new Error(`unexpected request ${key}`);
  };
  const calls = mockApi(
    new Proxy({}, { get: (_t, k) => (call: Call) => route(String(k), call) }) as never,
  );
  return {
    calls,
    log,
    fail,
    get notes() {
      return notes;
    },
    set notes(v) {
      notes = v;
    },
    writes: (path: string) => calls.filter((c) => c.method === "POST" && c.path === path),
  };
}

const S = { selector: "strong" };
const open = async (width = 1280, path = `/yearbooks/${BOOK}/notes`) => {
  viewport(width);
  return renderApp(path);
};
const card = async (name: string) =>
  (await screen.findByText(name, { selector: "strong" })).closest("li") as HTMLElement;

describe("inbox list", () => {
  it("shows the counts, the answers with their labels and the photos", async () => {
    fake([
      note(1, "pending", {
        answers: {
          name: "Khoa",
          relationship: "roommate",
          message: "Go be brilliant",
          best_memory: "The balcony",
          zz_new_field: "Raw id kept",
        },
        photos: [{ id: PHOTO }],
        link_label: "Cafe friends",
      }),
      note(2, "approved", { answers: { message: "No name given" } }),
      note(3, "hidden"),
    ]);
    await open();
    const c = await card("Khoa");
    expect(within(c).getByText("Go be brilliant")).toBeInTheDocument();
    expect(within(c).getByText(/roommate · .*ago · Cafe friends/)).toBeInTheDocument();
    expect(within(c).getByText("Best memory together")).toBeInTheDocument();
    expect(within(c).getByText("zz_new_field")).toBeInTheDocument(); // unknown ids are never hidden
    const thumb = c.querySelector("img")!;
    expect(thumb).toHaveAttribute("src", `/api/media/get-content?id=${PHOTO}&variant=thumb`);
    expect(thumb.closest("a")).toHaveAttribute(
      "href",
      `/api/media/get-content?id=${PHOTO}&variant=display`,
    );
    expect(await card("Anonymous")).toBeInTheDocument();
    const tabs = screen.getByRole("group", { name: "Show notes" });
    expect(within(tabs).getByRole("button", { name: /Pending 1/ })).toBeInTheDocument();
    expect(within(tabs).getByRole("button", { name: /All 3/ })).toBeInTheDocument();
    expect(screen.getByText(/1 waiting for you · 1 in your book/)).toBeInTheDocument();
  });

  it("filters by tab with the status in the request and pages with next_id", async () => {
    const s = fake([note(1, "pending"), note(2, "pending"), note(3, "pending")], { pageSize: 2 });
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Pending 3/ }));
    await screen.findByText("Friend 1", S);
    expect(screen.queryByText("Friend 3", S)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(await screen.findByText("Friend 3", S)).toBeInTheDocument();
    expect(s.calls.map((c) => c.path)).toContain(
      `/note/get-list?yearbook_id=${BOOK}&status=pending&limit=100&next_id=2`,
    );
  });

  it("invites a first run to create a link", async () => {
    fake([]);
    await open();
    expect(await screen.findByText("No notes yet")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create a note link" })).toHaveAttribute(
      "href",
      `/yearbooks/${BOOK}/note-links`,
    );
  });

  it("says so when a tab is empty", async () => {
    fake([note(1, "approved")]);
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Pending 0/ }));
    expect(await screen.findByText("You're all caught up")).toBeInTheDocument();
  });

  it("shows a load error with Try again, and does not retry a 4xx", async () => {
    const s = fake([note(1, "approved")]);
    s.fail["GET /note/get-list"] = err(404, "not_found");
    viewport(1280);
    renderApp(`/yearbooks/${BOOK}/notes`, { retry: 3 }); // as in main.tsx
    expect(await screen.findByText("We couldn't load your notes")).toBeInTheDocument();
    expect(screen.getByText("We could not find that.")).toBeInTheDocument();
    // main.tsx retries queries 3 times; the inbox must not, or the dead end takes seconds to show
    expect(
      s.calls.filter(
        (c) => c.path.startsWith("/note/get-list?yearbook_id") && c.path.includes("limit=100"),
      ),
    ).toHaveLength(1);
    s.fail["GET /note/get-list"] = undefined;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Friend 1", S)).toBeInTheDocument();
  });

  it("shows the offline banner when the server cannot be reached", async () => {
    fake([]);
    const served = fetch;
    vi.stubGlobal("fetch", (url: string, init?: RequestInit) =>
      url.includes("/note/") ? Promise.reject(new TypeError("offline")) : served(url, init),
    );
    await open();
    // a network error gets one retry, after a second
    expect(await screen.findByText("You're offline.", {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });

  it("puts the pending count on the sidebar entry and lists the new routes", async () => {
    fake([note(1, "pending"), note(2, "pending")]);
    await open(1280, `/yearbooks/${BOOK}/notes`);
    const nav = await screen.findByRole("navigation", { name: "Yearbook" });
    const link = within(nav).getByRole("link", { name: /Notes from friends/ });
    expect(link).toHaveAttribute("href", `/yearbooks/${BOOK}/notes`);
    expect(link).toHaveAttribute("aria-current", "page");
    await waitFor(() => expect(within(link).getByText("2")).toBeInTheDocument());
    expect(within(nav).getByRole("link", { name: "Note links" })).toHaveAttribute(
      "href",
      `/yearbooks/${BOOK}/note-links`,
    );
  });
});

describe("moderation", () => {
  it("approves one note at once, tells the server and shows a toast", async () => {
    const s = fake([note(1, "pending", { answers: { name: "Khoa", message: "hi" } })]);
    await open();
    const c = await card("Khoa");
    await userEvent.click(within(c).getByRole("button", { name: /Approve/ }));
    expect(within(c).getByText("Approved")).toBeInTheDocument();
    expect(await screen.findByText("Khoa's note approved.")).toBeInTheDocument();
    expect(s.writes("/note/batch-update-status")[0]?.body).toEqual({
      yearbook_id: BOOK,
      ids: [nid(1)],
      status: "approved",
    });
    expect(screen.getByRole("button", { name: /Pending 0/ })).toBeInTheDocument();
  });

  it("takes a note out of a filtered tab when it changes status", async () => {
    fake([note(1, "pending"), note(2, "pending")]);
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Pending 2/ }));
    await userEvent.click(within(await card("Friend 1")).getByRole("button", { name: /Hide/ }));
    await waitFor(() => expect(screen.queryByText("Friend 1", S)).not.toBeInTheDocument());
    expect(screen.getByText("Friend 2", S)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Hidden 1/ })).toBeInTheDocument();
  });

  it("rolls back and says why when the server refuses", async () => {
    const s = fake([note(1, "pending", { answers: { name: "Khoa", message: "hi" } })]);
    s.fail["POST /note/batch-update-status"] = err(429, "rate_limited", { "Retry-After": "60" });
    await open();
    const c = await card("Khoa");
    await userEvent.click(within(c).getByRole("button", { name: /Approve/ }));
    expect(await screen.findByText(/Too many attempts/)).toBeInTheDocument();
    await waitFor(() => expect(within(c).getByText("Pending")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /Pending 1/ })).toBeInTheDocument();
  });

  it("applies a bulk action to the selected notes in one request", async () => {
    const s = fake([note(1, "pending"), note(2, "pending"), note(3, "pending")]);
    await open();
    await userEvent.click(await screen.findByRole("checkbox", { name: "Select Friend 1's note" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Select Friend 3's note" }));
    expect(screen.getByText("2 selected")).toBeInTheDocument();
    await userEvent.click(
      within(screen.getByRole("region", { name: "Actions for the selected notes" })).getByRole(
        "button",
        { name: "Approve" },
      ),
    );
    await screen.findByText("2 notes approved.");
    expect(s.writes("/note/batch-update-status")[0]?.body).toEqual({
      yearbook_id: BOOK,
      ids: [nid(1), nid(3)],
      status: "approved",
    });
  });

  it("selects all", async () => {
    fake([note(1, "pending"), note(2, "pending")]);
    await open();
    await userEvent.click(await screen.findByRole("checkbox", { name: "Select all" }));
    expect(screen.getByText("2 selected")).toBeInTheDocument();
  });
});

describe("delete with Undo", () => {
  /**
   * Holds the timers of the Undo window (and nothing else) so a test can let the window run out on demand; faking all timers
   * would stall Testing Library's own waiting.
   */
  function holdUndoTimers() {
    const held = new Map<number, () => void>();
    const real = { set: window.setTimeout.bind(window), clear: window.clearTimeout.bind(window) };
    let n = 9_000_000;
    vi.spyOn(window, "setTimeout").mockImplementation(((
      fn: () => void,
      ms?: number,
      ...a: never[]
    ) => {
      if (ms !== UNDO_MS) return real.set(fn, ms, ...a);
      held.set(++n, fn);
      return n;
    }) as never);
    vi.spyOn(window, "clearTimeout").mockImplementation(((id?: number) => {
      if (id !== undefined && held.delete(id)) return;
      real.clear(id);
    }) as never);
    return {
      count: () => held.size,
      expire: async () => {
        await act(async () => {
          for (const [id, fn] of [...held]) {
            held.delete(id);
            fn();
          }
        });
      },
    };
  }

  it("deletes a pending note only when the Undo window runs out", async () => {
    const s = fake([note(1, "pending"), note(2, "pending")]);
    await open();
    const c = await card("Friend 1");
    const timers = holdUndoTimers();
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 1's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(screen.queryByText("Friend 1", S)).not.toBeInTheDocument();
    expect(screen.getByText("Note deleted.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /All 1/ })).toBeInTheDocument(); // the count already counts it out
    expect(s.writes("/note/batch-delete")).toHaveLength(0);
    await timers.expire();
    await waitFor(() => expect(s.writes("/note/batch-delete")).toHaveLength(1));
    expect(s.writes("/note/batch-delete")[0]?.body).toEqual({ yearbook_id: BOOK, ids: [nid(1)] });
    await waitFor(() => expect(screen.getByRole("button", { name: /All 1/ })).toBeInTheDocument());
    expect(screen.queryByText("Friend 1", S)).not.toBeInTheDocument();
  });

  it("Undo brings the note back and sends nothing", async () => {
    const s = fake([note(1, "pending")]);
    await open();
    const c = await card("Friend 1");
    const timers = holdUndoTimers();
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 1's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(screen.queryByText("Friend 1", S)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(screen.getByText("Friend 1", S)).toBeInTheDocument();
    expect(timers.count()).toBe(0);
    await timers.expire();
    expect(s.writes("/note/batch-delete")).toHaveLength(0);
  });

  it("keeps the Undo toast while another note is approved, and a second delete joins the first", async () => {
    const s = fake([note(1, "pending"), note(2, "pending"), note(3, "pending")]);
    await open();
    const c1 = await card("Friend 1");
    const c2 = await card("Friend 2");
    const timers = holdUndoTimers();
    await userEvent.click(
      within(c1).getByRole("button", { name: "More actions for Friend 1's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await userEvent.click(within(c2).getByRole("button", { name: /Approve/ }));
    await screen.findByText("Friend 2's note approved.");
    expect(screen.getByRole("button", { name: "Undo" })).toBeInTheDocument();
    const c3 = await card("Friend 3");
    await userEvent.click(
      within(c3).getByRole("button", { name: "More actions for Friend 3's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(screen.getByText("2 notes deleted.")).toBeInTheDocument();
    await timers.expire();
    await waitFor(() => expect(s.writes("/note/batch-delete")).toHaveLength(1));
    expect(s.writes("/note/batch-delete")[0]?.body).toEqual({
      yearbook_id: BOOK,
      ids: [nid(1), nid(3)],
    });
  });

  it("drops the delete when the owner leaves the page", async () => {
    const s = fake([note(1, "pending")]);
    const view = await open();
    const c = await card("Friend 1");
    const timers = holdUndoTimers();
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 1's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    view.unmount();
    expect(timers.count()).toBe(0); // the timer is cleared, so the delete can never be sent late
    expect(s.writes("/note/batch-delete")).toHaveLength(0);
    expect(s.notes).toHaveLength(1);
  });

  it("asks first for an approved note, and Hide instead hides it", async () => {
    const s = fake([note(1, "approved")]);
    await open();
    const c = await card("Friend 1");
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 1's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog", { name: "Delete this note?" });
    expect(s.writes("/note/batch-delete")).toHaveLength(0);
    await userEvent.click(within(dialog).getByRole("button", { name: "Hide instead" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() =>
      expect(s.writes("/note/batch-update-status")[0]?.body).toMatchObject({ status: "hidden" }),
    );
  });

  it("asks first for several notes", async () => {
    fake([note(1, "pending"), note(2, "pending")]);
    await open();
    await userEvent.click(await screen.findByRole("checkbox", { name: "Select all" }));
    const bar = screen.getByRole("region", { name: "Actions for the selected notes" });
    await userEvent.click(within(bar).getByRole("button", { name: "Delete" }));
    expect(await screen.findByRole("dialog", { name: "Delete 2 notes?" })).toBeInTheDocument();
  });

  it("shows a failed delete again with the reason", async () => {
    const s = fake([note(1, "pending")]);
    s.fail["POST /note/batch-delete"] = err(502, "storage_error");
    await open();
    const c = await card("Friend 1");
    const timers = holdUndoTimers();
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 1's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await timers.expire();
    expect(await screen.findByText(/Photo storage is not responding/)).toBeInTheDocument();
    expect(await screen.findByText("Friend 1", S)).toBeInTheDocument();
  });
});

describe("book order", () => {
  const three = () => [note(1, "approved"), note(2, "approved"), note(3, "approved")];
  const names = () =>
    screen
      .getAllByRole("listitem")
      .filter((li) => li.hasAttribute("data-note-id"))
      .map((li) => li.querySelector("strong")!.textContent);

  it("loads every page of the approved tab so the whole set can be reordered", async () => {
    const s = fake(three(), { pageSize: 2 });
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Approved 3/ }));
    expect(await screen.findByText("Friend 3", S)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
    expect(
      s.calls.some((c) => c.path.includes("status=approved") && c.path.includes("next_id=2")),
    ).toBe(true);
  });

  it("moves a note with the menu, announces it and saves once after a pause", async () => {
    const s = fake(three());
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Approved 3/ }));
    const c = await card("Friend 3");
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 3's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Move up" }));
    expect(names()).toEqual(["Friend 1", "Friend 3", "Friend 2"]);
    expect(screen.getByText("Friend 3's note, moved to position 2 of 3")).toBeInTheDocument();
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 3's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Move up" }));
    expect(names()).toEqual(["Friend 3", "Friend 1", "Friend 2"]);
    await waitFor(() => expect(s.writes("/note/reorder")).toHaveLength(1));
    expect(s.writes("/note/reorder")[0]?.body).toEqual({
      yearbook_id: BOOK,
      ids: [nid(3), nid(1), nid(2)],
    });
  });

  it("moves a note with the arrow keys on its grip", async () => {
    const s = fake(three());
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Approved 3/ }));
    await card("Friend 2");
    const grip = screen.getByRole("button", { name: "Drag to reorder Friend 2's note" });
    grip.focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(names()).toEqual(["Friend 1", "Friend 3", "Friend 2"]);
    await waitFor(() => expect(s.writes("/note/reorder")).toHaveLength(1));
  });

  it("reorders by dragging the grip over another card", async () => {
    const s = fake(three());
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Approved 3/ }));
    const first = await card("Friend 1");
    const last = await card("Friend 3");
    const grip = within(first).getByRole("button", { name: "Drag to reorder Friend 1's note" });
    document.elementFromPoint = () => last.querySelector("strong");
    fireEvent.pointerDown(grip, { pointerId: 1 });
    fireEvent.pointerMove(grip, { pointerId: 1, clientX: 5, clientY: 5 });
    expect(last).toHaveClass("drop-below");
    fireEvent.pointerUp(grip, { pointerId: 1 });
    expect(names()).toEqual(["Friend 2", "Friend 3", "Friend 1"]);
    await waitFor(() => expect(s.writes("/note/reorder")).toHaveLength(1));
    expect(s.writes("/note/reorder")[0]?.body).toMatchObject({ ids: [nid(2), nid(3), nid(1)] });
  });

  it("reloads the list and says so when the order is stale", async () => {
    const s = fake(three());
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Approved 3/ }));
    const c = await card("Friend 2");
    s.notes = s.notes.filter((n) => n.id !== nid(1)); // changed on another device
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 2's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Move down" }));
    expect(await screen.findByText(/The order changed somewhere else/)).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Friend 1", S)).not.toBeInTheDocument());
  });

  it("saves a pending reorder when the owner leaves the page", async () => {
    const s = fake(three());
    const view = await open();
    await userEvent.click(await screen.findByRole("button", { name: /Approved 3/ }));
    const c = await card("Friend 2");
    await userEvent.click(
      within(c).getByRole("button", { name: "More actions for Friend 2's note" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Move up" }));
    view.unmount();
    await waitFor(() => expect(s.writes("/note/reorder")).toHaveLength(1));
  });

  it("has a Book order mode with arrows on a narrow screen", async () => {
    const s = fake(three());
    await open(375);
    await userEvent.click(await screen.findByRole("button", { name: /Approved 3/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Book order" }));
    const row = (await screen.findByText("Friend 3", S)).closest("li")!;
    expect(within(row).getByRole("button", { name: "Move Friend 3's note down" })).toBeDisabled();
    await userEvent.click(within(row).getByRole("button", { name: "Move Friend 3's note up" }));
    expect(screen.getByText("Friend 3's note, moved to position 2 of 3")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(s.writes("/note/reorder")).toHaveLength(1));
    expect(screen.getByRole("button", { name: "Book order" })).toBeInTheDocument();
  });

  it("offers no reordering outside the approved tab", async () => {
    fake(three());
    await open();
    await screen.findByText("Friend 1", S);
    expect(screen.queryByRole("button", { name: /Drag to reorder/ })).not.toBeInTheDocument();
  });
});

describe("keyboard focus", () => {
  const three = () => [note(1, "approved"), note(2, "approved"), note(3, "approved")];
  const names = () =>
    screen
      .getAllByRole("listitem")
      .filter((li) => li.hasAttribute("data-note-id"))
      .map((li) => li.querySelector("strong")!.textContent);
  const more = (c: HTMLElement, name: string) =>
    within(c).getByRole("button", { name: `More actions for ${name}'s note` });

  it("returns to the ••• button after a move from the menu", async () => {
    fake(three());
    await open();
    await userEvent.click(await screen.findByRole("button", { name: /Approved 3/ }));
    const c = await card("Friend 2");
    await userEvent.click(more(c, "Friend 2"));
    await userEvent.click(screen.getByRole("button", { name: "Move down" }));
    expect(names()).toEqual(["Friend 1", "Friend 3", "Friend 2"]);
    expect(more(await card("Friend 2"), "Friend 2")).toHaveFocus();
  });

  it.each(["cancel", "escape"])(
    "returns to the ••• button when the delete confirm is left with %s",
    async (how) => {
      fake([note(1, "approved")]);
      await open();
      const c = await card("Friend 1");
      await userEvent.click(more(c, "Friend 1"));
      await userEvent.click(screen.getByRole("button", { name: "Delete" }));
      const dialog = await screen.findByRole("dialog");
      if (how === "cancel")
        await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
      else await userEvent.keyboard("{Escape}");
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(more(c, "Friend 1")).toHaveFocus();
    },
  );

  it("moves to Undo when the deleted card is gone", async () => {
    fake([note(1, "pending")]);
    await open();
    const c = await card("Friend 1");
    await userEvent.click(more(c, "Friend 1"));
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(await screen.findByRole("button", { name: "Undo" })).toHaveFocus();
  });

  it("returns to the restored card's ••• button after Undo", async () => {
    fake([note(1, "pending"), note(2, "pending")]);
    await open();
    const c = await card("Friend 1");
    await userEvent.click(more(c, "Friend 1"));
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await userEvent.keyboard("{Enter}"); // the focus is on Undo
    expect(more(await card("Friend 1"), "Friend 1")).toHaveFocus();
  });
});
