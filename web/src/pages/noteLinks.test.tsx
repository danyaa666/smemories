import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { NoteCollection } from "../api/notes";
import { err, mockApi, ok, renderApp, user, type Call } from "../test/api";

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const TOKEN = "q3Zp0Vn8xYk2LwR5tB7mJc9aHdE4sUgF";
const link = (n: number, over: Partial<NoteCollection> = {}): NoteCollection => ({
  id: `01J9Z3K6V8Q4M7N2P5R8T0L${String(n).padStart(3, "0")}`,
  label: `Link ${n}`,
  deadline_at: null,
  created_at: 1760000000000,
  revoked_at: null,
  note_count: 3,
  ...over,
});

function serve(links: NoteCollection[], extra: Record<string, (c: Call) => Response> = {}) {
  let items = [...links];
  return mockApi({
    "GET /user/get-me": () => ok(user()),
    [`GET /note-collection/get-list?yearbook_id=${BOOK}`]: () => ok({ items, next_id: "" }),
    "POST /note-collection/create": (c) => {
      const b = c.body as { label?: string; deadline_at: number | null };
      items = [
        link(9, { label: b.label ?? "", deadline_at: b.deadline_at, note_count: 0 }),
        ...items,
      ];
      return ok({ ...items[0]!, token: TOKEN });
    },
    "POST /note-collection/revoke": (c) => {
      items = items.map((l) =>
        l.id === (c.body as { id: string }).id ? { ...l, revoked_at: 1 } : l,
      );
      return ok();
    },
    ...extra,
  });
}
const open = () => renderApp(`/yearbooks/${BOOK}/note-links`);

describe("note links", () => {
  it("lists the links with their state and never a token", async () => {
    serve([
      link(1),
      link(2, { revoked_at: 1760000100000, label: "" }),
      link(3, { deadline_at: Date.now() - 1000 }),
      link(4, { deadline_at: Date.now() + 86_400_000 * 3 }),
    ]);
    open();
    const items = await screen.findAllByRole("listitem");
    const text = (i: number) => items[i]?.textContent ?? "";
    expect(text(0)).toMatch(/Link 1.*Open.*Notes: 3.*No deadline/);
    expect(text(1)).toMatch(/Untitled link.*Revoked/);
    expect(text(2)).toMatch(/Link 3.*Closed/);
    expect(text(3)).toMatch(/Link 4.*Open until/);
    // a revoked link cannot be revoked again
    expect(within(items[1]!).queryByRole("button", { name: /Revoke/ })).not.toBeInTheDocument();
  });

  it("creates a link, shows <base>/c/<token> once and keeps the token out of storage", async () => {
    const calls = serve([]);
    open();
    await userEvent.type(await screen.findByLabelText(/Name/), "Class chat");
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));
    const field = await screen.findByLabelText("Link for Class chat");
    expect(field).toHaveValue(`${window.location.origin}/c/${TOKEN}`);
    expect(field).toHaveAttribute("readonly");
    expect(calls.find((c) => c.path === "/note-collection/create")?.body).toEqual({
      yearbook_id: BOOK,
      label: "Class chat",
      deadline_at: null,
    });
    expect(JSON.stringify({ ...window.localStorage, ...window.sessionStorage })).not.toContain(
      TOKEN,
    );
    // the list refreshed without any token
    expect((await screen.findAllByText("Class chat")).length).toBeGreaterThan(0);
    expect(document.body.innerHTML.split(TOKEN).length - 1).toBe(1); // only in the one field
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(document.body.innerHTML).not.toContain(TOKEN);
  });

  it("moves focus to the new link and back to the form when it is closed", async () => {
    serve([]);
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Create link" }));
    expect(await screen.findByRole("heading", { name: "Your link is ready" })).toHaveFocus();
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.getByLabelText(/Name/)).toHaveFocus();
  });

  it("copies the link", async () => {
    serve([]);
    open();
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    await userEvent.click(await screen.findByRole("button", { name: "Create link" }));
    await userEvent.click(await screen.findByRole("button", { name: "Copy link" }));
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/c/${TOKEN}`);
    expect(await screen.findByText("Copied")).toBeInTheDocument();
  });

  it("sends the picked day as the end of that day", async () => {
    const calls = serve([]);
    open();
    const day = new Date(Date.now() + 5 * 86_400_000);
    const value = `${day.getFullYear()}-${String(day.getMonth() + 1).padStart(2, "0")}-${String(day.getDate()).padStart(2, "0")}`;
    await userEvent.type(await screen.findByLabelText(/Last day to write/), value);
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));
    await screen.findByLabelText("Link");
    const sent = (
      calls.find((c) => c.path === "/note-collection/create")?.body as { deadline_at: number }
    ).deadline_at;
    expect(sent).toBe(
      new Date(day.getFullYear(), day.getMonth(), day.getDate(), 23, 59, 59, 999).getTime(),
    );
  });

  it("shows server errors next to the field or above the button", async () => {
    let n = 0;
    serve([], {
      "POST /note-collection/create": () =>
        n++ === 0 ? err(400, "invalid_label") : err(409, "limit_reached"),
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Create link" }));
    expect(await screen.findByText(/The name can have at most 60/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));
    expect(await screen.findByText(/at most 5 open links/)).toBeInTheDocument();
  });

  it("stops offering new links at five open ones", async () => {
    serve([1, 2, 3, 4, 5].map((n) => link(n)));
    open();
    expect(await screen.findAllByText(/at most 5 open links/)).not.toHaveLength(0);
    expect(screen.getByRole("button", { name: "Create link" })).toBeDisabled();
  });

  it("revokes after a confirmation", async () => {
    const calls = serve([link(1)]);
    open();
    await screen.findByText("Link 1", { selector: "strong" });
    await userEvent.click(screen.getByRole("button", { name: /Revoke Link 1/ }));
    const dialog = await screen.findByRole("dialog", { name: "Revoke this link?" });
    expect(calls.some((c) => c.path === "/note-collection/revoke")).toBe(false);
    await userEvent.click(within(dialog).getByRole("button", { name: "Revoke" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(calls.find((c) => c.path === "/note-collection/revoke")?.body).toEqual({
      id: link(1).id,
    });
    expect(await screen.findByText("Revoked")).toBeInTheDocument();
  });

  it("shows a load error with Try again and does not retry a 4xx", async () => {
    const calls = serve([], {
      [`GET /note-collection/get-list?yearbook_id=${BOOK}`]: () => err(404, "not_found"),
    });
    renderApp(`/yearbooks/${BOOK}/note-links`, { retry: 3 });
    expect(await screen.findByText("We couldn't load your links")).toBeInTheDocument();
    expect(calls.filter((c) => c.path.startsWith("/note-collection/get-list"))).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });

  it("speaks Vietnamese", async () => {
    const { default: i18n } = await import("../i18n");
    await i18n.changeLanguage("vi");
    serve([link(1)]);
    open();
    expect(await screen.findByRole("heading", { name: "Liên kết lời nhắn" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tạo liên kết" })).toBeInTheDocument();
    expect(await screen.findByText("Đang mở")).toBeInTheDocument();
  });
});
