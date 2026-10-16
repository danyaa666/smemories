import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../api/book";
import i18n from "../i18n";
import { err, mockApi, ok, renderApp, user, type Call } from "../test/api";

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const PHOTO = (n: number) => `01J9Z3K6V8Q4M7N2P5R8T${String(n).padStart(5, "0")}`;

const note = (i: number, over: Partial<BookData["notes"][number]> = {}) => ({
  id: `01J9Z3K6V8Q4M7N2P5R8N${String(i).padStart(5, "0")}`,
  answers: { name: `Bạn ${i}`, relationship: "Bạn cùng lớp", message: `Chúc mừng 😄 số ${i}` },
  photo_ids: [] as string[],
  ...over,
});

function bookData(notes: BookData["notes"], over: Partial<BookData> = {}): BookData {
  return {
    yearbook: {
      title: "Lớp 12A1",
      school_name: "THPT Chu Văn An",
      class_name: "12A1",
      graduation_year: 2026,
      motto: "Học, học nữa, học mãi",
      language: "vi",
      page_size: "A5",
      template_id: null,
      cover_media_id: null,
    },
    profile: {
      full_name: "Nguyễn Thị Hồng",
      nickname: "Hồng",
      quote: "",
      hobbies: "",
      future_plans: "",
      photo_media_id: null,
    },
    notes,
    note_count: notes.length,
    approved_total: notes.length,
    truncated: false,
    ...over,
  };
}

/** Image elements made in script (the preload) finish when the test says so. */
let pending: (() => void)[] = [];
let requested: string[] = [];
class FakeImage {
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  set src(url: string) {
    requested.push(url);
    pending.push(() => this.onload?.());
  }
}
const finishPhotos = () => act(() => void pending.splice(0).forEach((f) => f()));

function serve(data: BookData | Response, calls: Record<string, (c: Call) => Response> = {}) {
  return mockApi({
    "GET /user/get-me": () => ok(user()),
    [`GET /book-data/get?yearbook_id=${BOOK}`]: () => (data instanceof Response ? data : ok(data)),
    ...calls,
  });
}
const open = () => renderApp(`/yearbooks/${BOOK}/book`);
const sheets = (label?: string) =>
  document.querySelectorAll(label ? `[data-page="${label}"]` : "section[data-page]");

beforeEach(() => {
  pending = [];
  requested = [];
  vi.stubGlobal("Image", FakeImage);
});
afterEach(() => vi.unstubAllGlobals());

describe("book preview and print", () => {
  it("shows a loading state, then the pages", async () => {
    serve(bookData([note(1)]));
    open();
    expect(await screen.findByText("Loading your book...")).toBeInTheDocument();
    await waitFor(() => expect(sheets()).toHaveLength(4)); // cover, profile, one notes page, back
    expect(sheets("notes")).toHaveLength(1);
  });

  it.each([
    [0, 3],
    [1, 4],
    [4, 4],
    [5, 5],
    [40, 13],
  ])("a book with %i notes has %i pages", async (n, pages) => {
    serve(bookData(Array.from({ length: n }, (_, i) => note(i + 1))));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(pages));
    expect(screen.queryByText(/no approved notes yet/i) !== null).toBe(n === 0);
  });

  it("draws 300 notes of dense Vietnamese, emoji and markup as text, in the book's language", async () => {
    const nasty = "<img src=x onerror=alert(1)> & <b>đậm</b> 😄👩‍👩‍👧 Nguyễn Thị Bích Ngọc ".repeat(20);
    const notes = Array.from({ length: 300 }, (_, i) =>
      note(i + 1, {
        answers: { name: `Đặng Ngọc ${i + 1}`, relationship: "Bạn", message: nasty },
        photo_ids: [PHOTO(i * 3 + 1), PHOTO(i * 3 + 2), PHOTO(i * 3 + 3)],
      }),
    );
    serve(bookData(notes));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(3 + 75));
    expect(document.querySelector("img[src='x']")).toBeNull();
    expect(document.querySelector(".bk-sheet b")).toBeNull();
    expect(document.body.textContent).toContain("<img src=x onerror=alert(1)>");
    // the page is in the book's language (vi) although the app is in English
    expect(i18n.language).toBe("en");
    expect(screen.getAllByText("Lời nhắn từ bạn bè").length).toBeGreaterThan(0);
    expect(document.querySelectorAll(".bk-note img").length).toBe(900);
  });

  it("warns when the book is cut: 'X of Y notes are included'", async () => {
    serve(bookData([note(1), note(2)], { truncated: true, approved_total: 350 }));
    open();
    expect(await screen.findByText(/Only 2 of 350 approved notes are included/)).toBeVisible();
    expect(screen.getByText(/will not be printed/)).toBeInTheDocument();
  });

  it("shows no cut warning for a whole book", async () => {
    serve(bookData([note(1)]));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    expect(screen.queryByText(/approved notes are included/)).not.toBeInTheDocument();
  });

  it("keeps Print disabled until every photo is fetched, then prints", async () => {
    const print = vi.spyOn(window, "print").mockImplementation(() => {});
    serve(
      bookData([note(1, { photo_ids: [PHOTO(1), PHOTO(2)] }), note(2, { photo_ids: [PHOTO(1)] })]),
    );
    open();
    const button = await screen.findByRole("button", { name: "Print or save as PDF" });
    await waitFor(() => expect(requested).toHaveLength(2)); // PHOTO(1) once, at the print size
    expect(requested[0]).toBe(`/api/media/get-content?id=${PHOTO(1)}&variant=print`);
    expect(button).toBeDisabled();
    expect(
      screen.getByText(/Getting fonts and photos ready \(0 of 2 photos\)/),
    ).toBeInTheDocument();
    await userEvent.click(button);
    expect(print).not.toHaveBeenCalled();
    await finishPhotos();
    await waitFor(() => expect(button).toBeEnabled());
    expect(screen.getByText("Ready to print: 4 pages.")).toBeInTheDocument();
    await userEvent.click(button);
    expect(print).toHaveBeenCalledTimes(1);
  });

  it("images are lazy on screen and eager while the print dialog opens", async () => {
    serve(bookData([note(1, { photo_ids: [PHOTO(1)] })]));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    const img = () => document.querySelector<HTMLImageElement>(".bk-note img")!;
    expect(img().getAttribute("loading")).toBe("lazy");
    act(() => void window.dispatchEvent(new Event("beforeprint")));
    expect(img().getAttribute("loading")).toBe("eager");
    act(() => void window.dispatchEvent(new Event("afterprint")));
    expect(img().getAttribute("loading")).toBe("lazy");
  });

  it("says so when a photo cannot be loaded, and still lets the owner print", async () => {
    vi.stubGlobal(
      "Image",
      class {
        onerror: (() => void) | null = null;
        set src(_: string) {
          queueMicrotask(() => this.onerror?.());
        }
      },
    );
    serve(bookData([note(1, { photo_ids: [PHOTO(1)] })]));
    open();
    expect(await screen.findByText(/Photos that could not be loaded: 1/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Print or save as PDF" })).toBeEnabled();
  });

  it("asks for the page size of the book in @page and in the print help", async () => {
    const data = bookData([note(1)]);
    data.yearbook.page_size = "Letter";
    serve(data);
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    expect(document.querySelector("style")?.textContent ?? "").toContain(
      "@page { size: 215.9mm 279.4mm; margin: 0 }",
    );
    expect(screen.getByText("Paper size: Letter.")).toBeInTheDocument();
  });

  it("tells the owner how many notes are cut off, from what the fit reports", async () => {
    // jsdom has no layout: a text containing TOOLONG never fits, anything else always does.
    const text = (el: Element) => el.textContent ?? "";
    vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockImplementation(function (
      this: HTMLElement,
    ) {
      return text(this).includes("TOOLONG") ? 1000 : 10;
    });
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(100);
    serve(
      bookData([
        note(1, { answers: { name: "A", message: "TOOLONG" } }),
        note(2),
        note(3, { answers: { name: "C", message: "TOOLONG" } }),
      ]),
    );
    open();
    expect(
      await screen.findByText(
        "Notes that are too long for their space and are cut off in the printout: 2.",
      ),
    ).toBeInTheDocument();
  });

  it("does not retry a 4xx and shows the error", async () => {
    const calls = serve(err(404, "not_found"));
    open();
    expect(await screen.findByRole("alert")).toHaveTextContent("We could not find that.");
    expect(calls.filter((c) => c.path.startsWith("/book-data/get"))).toHaveLength(1);
  });
});
