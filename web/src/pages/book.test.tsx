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

describe("memory-book template", () => {
  const full = (i: number, over: Record<string, string> = {}) =>
    note(i, {
      answers: {
        name: `Nguyễn Thị Hồng Ánh ${i}`,
        nickname: "Ánh",
        relationship: "Bạn cùng lớp",
        how_we_met: "Ngồi cạnh nhau",
        first_impression: "Nói nhiều 😄",
        best_memory: "Đà Lạt",
        wish: "Thi đỗ nhé",
        advice: "Đừng bỏ cuộc",
        message: "Giữ liên lạc nhé",
        ...over,
      },
    });
  const memory = (notes: BookData["notes"], language: "en" | "vi" = "en") => {
    const data = bookData(notes);
    data.yearbook.template_id = "memory-book";
    data.yearbook.language = language;
    data.profile = {
      ...data.profile,
      quote: "Hãy sống",
      hobbies: "Đọc sách",
      future_plans: "Thiết kế",
    };
    return data;
  };

  it.each([
    [0, 3],
    [1, 4],
    [21, 24],
  ])("a book with %i notes has %i pages: one note per page", async (n, pages) => {
    serve(memory(Array.from({ length: n }, (_, i) => full(i + 1))));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(pages));
    expect(sheets("notes")).toHaveLength(n);
    expect(document.querySelectorAll(".mb-page")).toHaveLength(pages);
  });

  it("binds the profile, the cover and every answer of the note, in the book's language", async () => {
    serve(memory([full(1)], "vi"));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    const page = (label: string) => sheets(label)[0] as HTMLElement;
    const cover = page("cover").textContent ?? "";
    expect(cover).toContain("Nguyễn Thị Hồng");
    expect(cover).toContain("Lớp 12A1");
    expect(cover).toContain("Học, học nữa, học mãi");
    const profile = page("profile").textContent ?? "";
    for (const s of ["Giới thiệu về mình", "Đọc sách", "Thiết kế", "Hãy sống", "THPT Chu Văn An"])
      expect(profile).toContain(s);
    const friend = page("notes").textContent ?? "";
    for (const s of [
      "Từ một người bạn",
      "Nguyễn Thị Hồng Ánh 1",
      "Bạn cùng lớp",
      "Ngồi cạnh nhau",
      "Nói nhiều 😄",
      "Đà Lạt",
      "Thi đỗ nhé",
      "Đừng bỏ cuộc",
      "Giữ liên lạc nhé",
      "Chúng mình quen nhau thế nào",
      "Số 1",
    ])
      expect(friend).toContain(s);
    expect(page("back").textContent).toContain("Giữ liên lạc nhé");
  });

  it("prints English labels for an English book although the app is in Vietnamese", async () => {
    serve(memory([full(1)], "en"));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    expect(sheets("notes")[0]!.textContent).toContain("From a Friend");
    expect(sheets("notes")[0]!.textContent).toContain("How we met");
    expect(sheets("back")[0]!.textContent).toContain("Let’s Stay in Touch");
    expect(sheets("cover")[0]!.textContent).toContain("Class of 2026");
  });

  it("leaves out the boxes of fields the template did not ask, and shows a photo frame when there is no photo", async () => {
    serve(
      memory([
        note(1, { answers: { name: "Linh", message: "Chúc bạn vui" }, photo_ids: [] }),
        note(2, { answers: { name: "Mai", message: "Nhớ bạn" }, photo_ids: [PHOTO(1), PHOTO(2)] }),
      ]),
    );
    open();
    await waitFor(() => expect(sheets("notes")).toHaveLength(2));
    const [a, b] = [...sheets("notes")];
    expect(a!.querySelectorAll(".mb-ruled")).toHaveLength(1); // only the message
    expect(a!.querySelector(".mb-stick")).not.toBeNull(); // "stick your photo here"
    expect(a!.querySelector("img")).toBeNull();
    expect(b!.querySelectorAll("img")).toHaveLength(2); // the first photo in the frame, the second beside the message
    expect(b!.querySelector(".mb-side img")).not.toBeNull();
  });

  it("draws user text as text, never as HTML", async () => {
    const evil = "<img src=x onerror=alert(1)><b>đậm</b> & 😄";
    serve(memory([full(1, { name: evil, message: evil, wish: evil })]));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    expect(document.querySelector("img[src='x']")).toBeNull();
    expect(document.querySelector(".mb-page b")).toBeNull();
    expect(sheets("notes")[0]!.textContent).toContain("<img src=x onerror=alert(1)>");
  });

  describe("which notes were shrunk or cut", () => {
    // jsdom has no layout: a box needs 1/8 px of height per character and font px, and has 100 px.
    beforeEach(() => {
      vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockImplementation(function (
        this: HTMLElement,
      ) {
        return ((this.textContent ?? "").length * (parseFloat(this.style.fontSize) || 20)) / 8;
      });
      vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(100);
    });
    afterEach(() => vi.restoreAllMocks());

    it("lists a note of 2000 characters as cut, a longer note as smaller, each with a link to the inbox", async () => {
      const long = note(1, { answers: { name: "Linh", message: "x".repeat(2000) } });
      const medium = note(2, { answers: { name: "Hoa", message: "y".repeat(60) } });
      const short = note(3, { answers: { name: "Minh", message: "ok" } });
      serve(memory([long, medium, short]));
      open();
      const summary = await screen.findByText("Notes in a smaller size or cut off: 2");
      const box = summary.closest("details")!;
      expect(box).toHaveAttribute("open"); // a cut note: open without a click
      const items = [...box.querySelectorAll("li")];
      expect(items).toHaveLength(2); // Minh fits as designed
      // A5: 20 design px are 10.3 pt. The cut note comes first: 10 design px (the floor of 50 %) are 5 pt.
      expect(items[0]!.textContent).toContain("Linh, 5 pt, and still cut off");
      expect(items[1]!.textContent).toContain("Hoa, 7 pt");
      expect(items[1]!.textContent).not.toContain("cut off");
      expect(box.textContent).not.toContain("Minh");
      const links = [...box.querySelectorAll("a")].map((a) => a.getAttribute("href"));
      expect(links).toEqual([
        `/yearbooks/${BOOK}/notes?note=${long.id}`,
        `/yearbooks/${BOOK}/notes?note=${medium.id}`,
      ]);
      expect(
        screen.getByRole("link", { name: "Open the note of Linh in the inbox" }),
      ).toBeVisible();
      expect(
        screen.getByText(
          "Notes that are too long for their space and are cut off in the printout: 1.",
        ),
      ).toBeInTheDocument();
    });

    it("says nothing when every note fits", async () => {
      serve(memory([note(1, { answers: { name: "Minh", message: "ok" } })]));
      open();
      await waitFor(() => expect(sheets()).toHaveLength(4));
      expect(screen.queryByText(/Notes in a smaller size/)).not.toBeInTheDocument();
    });

    it("names a note without a name by its position", async () => {
      serve(memory([note(1, { answers: { message: "z".repeat(2000) } })]));
      open();
      expect(await screen.findByText(/Note 1, 5 pt, and still cut off/)).toBeInTheDocument();
    });
  });
});
