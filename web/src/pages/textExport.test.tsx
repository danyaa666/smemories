import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../api/book";
import type { BookText } from "../api/bookExport";
import { err, mockApi, ok, renderApp, user } from "../test/api";

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const TEXT = "SMemories — Lớp 12A1\nẢnh: https://x.test/api/public-media/get-content?token=T0KEN\n";
const exported = (over: Partial<BookText> = {}): BookText => ({
  filename: "lop-12a1-20261021.txt",
  text: TEXT,
  note_count: 3,
  photo_count: 1,
  approved_total: 3,
  truncated: false,
  ...over,
});
const book: BookData = {
  yearbook: {
    title: "Lớp 12A1",
    school_name: "",
    class_name: "",
    graduation_year: 2026,
    motto: "",
    language: "vi",
    page_size: "A5",
    template_id: null,
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
};

function serve(over: Record<string, () => Response> = {}) {
  return mockApi({
    "GET /user/get-me": () => ok(user()),
    [`GET /book-data/get?yearbook_id=${BOOK}`]: () => ok(book),
    "GET /template/get-list?page_size=A5": () => ok({ items: [], next_id: "" }),
    [`GET /book-export/get-text?yearbook_id=${BOOK}`]: () => ok(exported()),
    "POST /yearbook/revoke-photo-links": () => ok({}),
    ...over,
  });
}
const open = async () => {
  renderApp(`/yearbooks/${BOOK}/book`);
  await screen.findByRole("heading", { name: "Export as text" });
};
const setClipboard = (c: unknown) =>
  Object.defineProperty(navigator, "clipboard", { value: c, configurable: true });
const btn = (name: string) => screen.getByRole("button", { name });
const exports = (calls: { path: string }[]) => calls.filter((c) => c.path.includes("get-text"));

let saved: { name: string; blob: Blob }[];
let revoked: string[];
beforeEach(() => {
  saved = [];
  revoked = [];
  const blobs = new Map<string, Blob>();
  URL.createObjectURL = (b: Blob) => {
    const u = `blob:t/${blobs.size}`;
    blobs.set(u, b);
    return u;
  };
  URL.revokeObjectURL = (u: string) => void revoked.push(u);
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    saved.push({ name: this.download, blob: blobs.get(this.href)! });
  });
  vi.stubGlobal(
    "Image",
    class {
      set src(_: string) {}
    },
  );
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  Reflect.deleteProperty(URL, "createObjectURL");
  Reflect.deleteProperty(URL, "revokeObjectURL");
  Reflect.deleteProperty(navigator, "clipboard");
  Reflect.deleteProperty(document, "execCommand");
});

describe("export as text", () => {
  it("warns before the first download only, Cancel does nothing, Continue downloads", async () => {
    const calls = serve();
    const u = userEvent.setup();
    await open();
    await u.click(btn("Download text file"));
    const warn = screen.getByRole("dialog", { name: "Photo links are public" });
    expect(warn).toHaveTextContent("Anyone who has a link can open that photo without signing in");
    await u.click(within(warn).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(exports(calls)).toHaveLength(0);

    await u.click(btn("Download text file"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(saved).toHaveLength(1));
    expect(saved[0]!.name).toBe("lop-12a1-20261021.txt");
    expect(saved[0]!.blob.type).toBe("text/plain;charset=utf-8");
    expect(await saved[0]!.blob.text()).toBe(TEXT);
    await waitFor(() => expect(revoked).toEqual(["blob:t/0"]));

    await u.click(btn("Download text file")); // not asked again
    await waitFor(() => expect(saved).toHaveLength(2));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(exports(calls)).toHaveLength(2);
  });

  it("asks the book's language implicitly (no lang parameter) and does not cache", async () => {
    const calls = serve();
    const u = userEvent.setup();
    await open();
    await u.click(btn("Download text file"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(exports(calls)).toHaveLength(1));
    expect(exports(calls)[0]!.path).not.toContain("lang=");
  });

  it("Escape closes the warning, focus starts on Cancel and stays inside", async () => {
    serve();
    const u = userEvent.setup();
    await open();
    await u.click(btn("Download text file"));
    expect(btn("Cancel")).toHaveFocus();
    await u.tab();
    expect(btn("Continue")).toHaveFocus();
    await u.tab();
    expect(btn("Cancel")).toHaveFocus(); // wrapped
    await u.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(btn("Download text file")).toHaveFocus();
  });

  it("copies with the clipboard API and says so", async () => {
    serve();
    const writeText = vi.fn().mockResolvedValue(undefined);
    const u = userEvent.setup();
    setClipboard({ writeText });
    await open();
    await u.click(btn("Copy text"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    expect(await screen.findByText("Copied")).toBeInTheDocument();
    expect(writeText).toHaveBeenCalledWith(TEXT);
  });

  it("falls back to execCommand when the clipboard is refused", async () => {
    serve();
    const exec = vi.fn(() => true);
    Object.defineProperty(document, "execCommand", { value: exec, configurable: true });
    const u = userEvent.setup();
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error("denied")) });
    await open();
    await u.click(btn("Copy text"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    expect(await screen.findByText("Copied")).toBeInTheDocument();
    expect(exec).toHaveBeenCalledWith("copy");
  });

  it("shows a selected read-only textarea when nothing can copy", async () => {
    serve();
    Object.defineProperty(document, "execCommand", { value: () => false, configurable: true });
    const u = userEvent.setup();
    setClipboard(undefined);
    await open();
    await u.click(btn("Copy text"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    const box = (await screen.findByRole("textbox", {
      name: "The exported text",
    })) as HTMLTextAreaElement;
    expect(box).toHaveAttribute("readonly");
    expect(box.value).toBe(TEXT);
    expect(box.selectionEnd - box.selectionStart).toBe(TEXT.length);
    expect(screen.queryByText("Copied")).toBeNull();
  });

  it("says when the text holds only part of the notes", async () => {
    serve({
      [`GET /book-export/get-text?yearbook_id=${BOOK}`]: () =>
        ok(exported({ note_count: 1000, approved_total: 1200, truncated: true })),
    });
    const u = userEvent.setup();
    await open();
    await u.click(btn("Download text file"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    expect(await screen.findByText("1000 of 1200 notes are included")).toBeInTheDocument();
  });

  it("sends one request for a double click and disables the buttons meanwhile", async () => {
    let release: (r: Response) => void = () => {};
    const slow = new Promise<Response>((r) => (release = r));
    const calls = serve({ [`GET /book-export/get-text?yearbook_id=${BOOK}`]: () => slow as never });
    const u = userEvent.setup();
    await open();
    await u.click(btn("Download text file"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    const working = await screen.findByRole("button", { name: "Working..." });
    expect(working).toBeDisabled();
    expect(btn("Copy text")).toBeDisabled();
    expect(btn("Disable photo links")).toBeDisabled();
    await u.dblClick(working);
    await act(async () => release(ok(exported())));
    await waitFor(() => expect(saved).toHaveLength(1));
    expect(exports(calls)).toHaveLength(1);
    expect(btn("Download text file")).toBeEnabled();
  });

  it("shows the retry time when rate limited and stays usable", async () => {
    serve({
      [`GET /book-export/get-text?yearbook_id=${BOOK}`]: () =>
        err(429, "rate_limited", { "Retry-After": "300" }),
    });
    const u = userEvent.setup();
    await open();
    await u.click(btn("Download text file"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Try again in 5 min");
    expect(btn("Download text file")).toBeEnabled();
    expect(btn("Print or save as PDF")).toBeInTheDocument();
  });

  it("shows the generic error for anything else", async () => {
    serve({ [`GET /book-export/get-text?yearbook_id=${BOOK}`]: () => err(500, "internal_error") });
    const u = userEvent.setup();
    await open();
    await u.click(btn("Download text file"));
    await u.click(screen.getByRole("button", { name: "Continue" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Something went wrong");
    expect(saved).toHaveLength(0);
  });

  it("disables the links after a confirm, and the next export warns again", async () => {
    const calls = serve();
    const u = userEvent.setup();
    await open();
    await u.click(btn("Download text file")); // warned once
    await u.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(saved).toHaveLength(1));

    await u.click(btn("Disable photo links"));
    const confirm = screen.getByRole("dialog", { name: "Disable photo links?" });
    expect(confirm).toHaveTextContent("Every photo link you have already shared stops working.");
    await u.click(within(confirm).getByRole("button", { name: "Cancel" }));
    expect(calls.filter((c) => c.path.includes("revoke"))).toHaveLength(0);

    await u.click(btn("Disable photo links"));
    await u.click(screen.getByRole("button", { name: "Disable links" }));
    expect(await screen.findByText("Photo links disabled")).toBeInTheDocument();
    expect(calls.find((c) => c.path.includes("revoke"))!.body).toEqual({ id: BOOK });
    expect(screen.queryByRole("dialog")).toBeNull();

    await u.click(btn("Download text file"));
    expect(screen.getByRole("dialog", { name: "Photo links are public" })).toBeInTheDocument();
  });

  it("keeps the page usable when disabling fails", async () => {
    serve({ "POST /yearbook/revoke-photo-links": () => err(404, "not_found") });
    const u = userEvent.setup();
    await open();
    await u.click(btn("Disable photo links"));
    await u.click(screen.getByRole("button", { name: "Disable links" }));
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByText("Photo links disabled")).toBeNull();
    expect(btn("Disable photo links")).toBeEnabled();
  });
});
