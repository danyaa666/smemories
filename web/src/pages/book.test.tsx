import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../api/book";
import i18n from "../i18n";
import { loaded } from "../components/imageQueue";
import { err, mockApi, ok, renderApp, user, type Call } from "../test/api";

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const PHOTO = (n: number) => `01J9Z3K6V8Q4M7N2P5R8T${String(n).padStart(5, "0")}`;

const note = (i: number, over: Partial<BookData["notes"][number]> = {}) => ({
  id: `01J9Z3K6V8Q4M7N2P5R8N${String(i).padStart(5, "0")}`,
  answers: { name: `Bạn ${i}`, relationship: "Bạn cùng lớp", message: `Chúc mừng 😄 số ${i}` },
  signature: null as BookData["notes"][number]["signature"],
  photo_ids: [] as string[],
  ...over,
});

function bookData(notes: BookData["notes"], over: Partial<BookData> = {}): BookData {
  return {
    design: null,
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
/** jsdom never loads an <img>: the photos on the pages settle when the test says so (T-121: the book is ready only after they do). */
const photoImgs = () => [...document.querySelectorAll<HTMLImageElement>("img.bk-photo")];
const settleImgs = (type: "load" | "error") =>
  act(() => void photoImgs().forEach((i) => i.dispatchEvent(new Event(type))));

function serve(data: BookData | Response, calls: Record<string, (c: Call) => Response> = {}) {
  return mockApi({
    "GET /user/get-me": () => ok(user()),
    [`GET /book-data/get?yearbook_id=${BOOK}`]: () => (data instanceof Response ? data : ok(data)),
    ...Object.fromEntries(
      ["A5", "A4", "Letter"].map((s) => [
        `GET /template/get-list?page_size=${s}`,
        () => ok({ items: [], next_id: "" }),
      ]),
    ), // the picker's list; the tests of the picker bring their own
    ...calls,
  });
}
const open = () => renderApp(`/yearbooks/${BOOK}/book`);
const sheets = (label?: string) =>
  document.querySelectorAll(label ? `[data-page="${label}"]` : "section[data-page]");

beforeEach(() => {
  pending = [];
  requested = [];
  loaded.clear();
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
    await new Promise((r) => setTimeout(r, 50));
    expect(button).toBeDisabled(); // fetched, but the photos on the pages are still loading
    settleImgs("load");
    await waitFor(() => expect(button).toBeEnabled());
    expect(screen.getByText("Ready to print: 4 pages.")).toBeInTheDocument();
    await userEvent.click(button);
    expect(print).toHaveBeenCalledTimes(1);
  });

  it("T-121: every photo loads at once even when IntersectionObserver never fires (pages never scrolled to), under the 16 cap", async () => {
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
    const many = Array.from({ length: 40 }, (_, i) => note(i + 1, { photo_ids: [PHOTO(i + 1)] }));
    serve(bookData(many));
    open();
    await waitFor(() => expect(photoImgs()).toHaveLength(40));
    await waitFor(() => expect(photoImgs().filter((i) => i.getAttribute("src"))).toHaveLength(16));
    // each photo that settles frees a slot for the next one: all 40 get their file without any scrolling
    for (let i = 0; i < 4 && photoImgs().some((p) => !p.getAttribute("src")); i++) {
      settleImgs("load");
      await new Promise((r) => setTimeout(r, 20));
    }
    expect(photoImgs().every((p) => p.getAttribute("src"))).toBe(true);
  });

  it("images decode asynchronously on screen and synchronously while the print dialog opens", async () => {
    serve(bookData([note(1, { photo_ids: [PHOTO(1)] })]));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    const img = () => document.querySelector<HTMLImageElement>(".bk-note img")!;
    expect(img().getAttribute("decoding")).toBe("async");
    act(() => void window.dispatchEvent(new Event("beforeprint")));
    expect(img().getAttribute("decoding")).toBe("sync");
    act(() => void window.dispatchEvent(new Event("afterprint")));
    expect(img().getAttribute("decoding")).toBe("async");
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
    // the photo on the page fails too (after its one retry) and still does not block printing
    await waitFor(
      () => {
        settleImgs("error");
        expect(screen.getByRole("button", { name: "Print or save as PDF" })).toBeEnabled();
      },
      { timeout: 4000 },
    );
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

// T-061: the template picker, the print guide per device and the "does the PDF look right?" counter.
const MEMORY = {
  id: "memory-book",
  name: { en: "Memory Book", vi: "Lưu bút" },
  page_sizes: ["A5", "A4", "Letter"],
  renderer: "html",
  preview: "/template-previews/memory-book.jpg",
};
const CLASSIC = {
  ...MEMORY,
  id: "classic",
  name: { en: "Classic", vi: "Cổ điển" },
  renderer: "go",
};
const FUTURE = { ...MEMORY, id: "future-book", name: { en: "Future", vi: "Tương lai" } }; // html, but this build cannot draw it
const NAVY = {
  ...MEMORY,
  id: "navy-classic",
  name: { en: "Navy Classic", vi: "Navy cổ điển" },
  preview: "/template-previews/navy-classic.jpg",
};
const templates =
  (...items: object[]) =>
  () =>
    ok({ items, next_id: "" });
const withTemplates = (...items: object[]) =>
  Object.fromEntries(
    ["A5", "A4", "Letter"].map((s) => [
      `GET /template/get-list?page_size=${s}`,
      templates(...items),
    ]),
  );
const radios = () => screen.queryAllByRole("radio").map((r) => (r as HTMLInputElement).value);

describe("template picker", () => {
  it("asks for the templates of the book's page size and lists the ones the browser can draw, plus the plain layout", async () => {
    const calls = serve(bookData([note(1)]), withTemplates(MEMORY, CLASSIC, FUTURE));
    open();
    expect(await screen.findByRole("group", { name: "Template" })).toBeInTheDocument();
    expect(radios()).toEqual(["", "memory-book"]); // go renderer and unknown html template are not offered
    expect(calls.some((c) => c.path === "/template/get-list?page_size=A5")).toBe(true);
    expect(screen.getByRole("radio", { name: /Plain layout/ })).toBeChecked(); // a book without a template
    expect(
      screen.getByRole("img", { name: "Preview of the cover of Memory Book" }),
    ).toHaveAttribute("src", "/template-previews/memory-book.jpg");
  });

  it("offers navy-classic next to memory-book", async () => {
    serve(bookData([note(1)]), withTemplates(MEMORY, NAVY, CLASSIC));
    open();
    await screen.findByRole("group", { name: "Template" });
    expect(radios()).toEqual(["", "memory-book", "navy-classic"]);
  });

  it("asks for the Letter list for a Letter book", async () => {
    const data = bookData([note(1)]);
    data.yearbook.page_size = "Letter";
    const calls = serve(data, withTemplates(MEMORY));
    open();
    await screen.findByRole("group", { name: "Template" });
    expect(calls.some((c) => c.path === "/template/get-list?page_size=Letter")).toBe(true);
  });

  it("marks the template of the book and names it in Vietnamese when the app is", async () => {
    const data = bookData([note(1)]);
    data.yearbook.template_id = "memory-book";
    serve(data, withTemplates(MEMORY));
    await act(() => i18n.changeLanguage("vi"));
    try {
      open();
      const radio = await screen.findByRole("radio", { name: /Lưu bút/ });
      expect(radio).toBeChecked();
    } finally {
      await act(() => i18n.changeLanguage("en"));
    }
  });

  it("shows the plain layout as chosen when the book has a template this browser does not know", async () => {
    const data = bookData([note(1)]);
    data.yearbook.template_id = "classic";
    serve(data, withTemplates(MEMORY, CLASSIC));
    open();
    expect(await screen.findByRole("radio", { name: /Plain layout/ })).toBeChecked();
  });

  it("is not shown when the server has nothing to offer, and the book still prints", async () => {
    serve(bookData([note(1)]), withTemplates());
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    await waitFor(() => expect(screen.queryByRole("group", { name: "Template" })).toBeNull());
    expect(screen.getByRole("button", { name: "Print or save as PDF" })).toBeInTheDocument();
  });

  it("saves the choice, redraws the pages and does not fetch the book again", async () => {
    let saved: unknown;
    const calls = serve(bookData([note(1), note(2)]), {
      ...withTemplates(MEMORY),
      "POST /yearbook/update": (c) => {
        saved = c.body;
        return ok({ id: BOOK, template_id: "memory-book" });
      },
    });
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4)); // plain layout: cover, profile, one notes page (4 notes fit), back
    const memory = await screen.findByRole("radio", { name: /Memory Book/ });
    await userEvent.click(memory);
    expect(saved).toEqual({ id: BOOK, template_id: "memory-book" });
    expect(await screen.findByText("Template saved: Memory Book.")).toBeInTheDocument();
    expect(memory).toBeChecked();
    expect(memory).toHaveFocus(); // the focus stays where the owner is
    await waitFor(() => expect(sheets()).toHaveLength(5)); // memory-book: one note per page
    expect(document.querySelectorAll(".mb-page").length).toBeGreaterThan(0);
    expect(calls.filter((c) => c.path.startsWith("/book-data/get"))).toHaveLength(1);
  });

  it("clears the template when the plain layout is picked", async () => {
    const data = bookData([note(1)]);
    data.yearbook.template_id = "memory-book";
    let saved: unknown;
    serve(data, {
      ...withTemplates(MEMORY),
      "POST /yearbook/update": (c) => {
        saved = c.body;
        return ok({ id: BOOK, template_id: null });
      },
    });
    open();
    await userEvent.click(await screen.findByRole("radio", { name: /Plain layout/ }));
    await waitFor(() => expect(saved).toEqual({ id: BOOK, template_id: null }));
    await waitFor(() => expect(document.querySelectorAll(".mb-page")).toHaveLength(0));
  });

  it("shows the server's refusal once, keeps the old choice and does not retry a 4xx", async () => {
    const calls = serve(bookData([note(1)]), {
      ...withTemplates(MEMORY),
      "POST /yearbook/update": () => err(400, "invalid_template"),
    });
    open();
    await userEvent.click(await screen.findByRole("radio", { name: /Memory Book/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "That template is not available for this paper size.",
    );
    expect(calls.filter((c) => c.path === "/yearbook/update")).toHaveLength(1);
    expect(screen.getByRole("radio", { name: /Plain layout/ })).toBeChecked();
    expect(document.querySelectorAll(".mb-page")).toHaveLength(0);
  });

  it("says so when the list cannot be loaded, without retrying a 4xx, and the book still prints", async () => {
    const calls = serve(bookData([note(1)]), {
      "GET /template/get-list?page_size=A5": () => err(403, "forbidden"),
    });
    open();
    expect(await screen.findByText("The templates could not be loaded.")).toBeInTheDocument();
    expect(calls.filter((c) => c.path.startsWith("/template/get-list"))).toHaveLength(1);
    await waitFor(() => expect(sheets()).toHaveLength(4));
  });
});

describe("print guide", () => {
  const ua = (userAgent: string) =>
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(userAgent);
  const GUIDES: [string, string, string[]][] = [
    [
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36 Edg/127.0.0.0",
      "Chrome or Edge on a computer",
      [
        "Destination: Save as PDF.",
        "Paper size: A5 (under More settings).",
        "Margins: None. Scale: Default (100). Headers and footers: off.",
        "Background graphics: on. Then press Save.",
      ],
    ],
    [
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
      "Safari on a Mac",
      [
        "Paper Size: A5 (for Letter, choose US Letter).",
        "tick Print backgrounds",
        "Click PDF at the lower left, then Save as PDF.",
      ],
    ],
    [
      "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0",
      "Firefox on a computer",
      [
        "Destination: Save to PDF.",
        "Paper size: A5. Margins: None. Scale: 100.",
        "tick Print backgrounds",
      ],
    ],
    [
      "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Mobile Safari/537.36",
      "Chrome on Android",
      ["At the top, choose Save as PDF.", "Paper size: A5. Margins: None."],
    ],
    [
      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
      "Safari on an iPhone or iPad",
      ["spread two fingers apart on the first page", "then Save to Files"],
    ],
    [
      "Lynx/2.9.0",
      "another browser",
      [
        "Destination: Save as PDF.",
        "Paper size: A5.",
        "Margins: None. Scale: 100 %.",
        "Turn headers and footers off and background graphics on.",
      ],
    ],
  ];

  it.each(GUIDES)("%#: shows the exact steps for the browser", async (agent, name, steps) => {
    ua(agent);
    serve(bookData([note(1)]), withTemplates());
    open();
    const guide = await screen.findByRole("region", { name: "Print settings for your device" });
    expect(guide).toHaveTextContent(`Detected: ${name}.`);
    for (const s of steps) expect(guide.textContent).toContain(s);
  });

  it("names the paper size of the book in the steps", async () => {
    ua(GUIDES[0]![0]);
    const data = bookData([note(1)]);
    data.yearbook.page_size = "A4";
    serve(data, withTemplates());
    open();
    expect(await screen.findByText("Paper size: A4 (under More settings).")).toBeInTheDocument();
  });

  it("is in Vietnamese when the app is", async () => {
    ua(GUIDES[0]![0]);
    serve(bookData([note(1)]), withTemplates());
    await act(() => i18n.changeLanguage("vi"));
    try {
      open();
      const guide = await screen.findByRole("region", { name: "Cài đặt in cho thiết bị của bạn" });
      expect(guide).toHaveTextContent("Khổ giấy: A5");
      expect(guide).toHaveTextContent('Bật "Đồ họa nền"');
    } finally {
      await act(() => i18n.changeLanguage("en"));
    }
  });

  it("says that the settings cannot be checked from the page", async () => {
    serve(bookData([note(1)]), withTemplates());
    open();
    expect(await screen.findByText(/cannot see or change the print window/)).toBeVisible();
  });
});

describe("print feedback", () => {
  const CHROME =
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36";
  const IPHONE =
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1";
  const agent = (u: string) => vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(u);
  const post = (handler: (c: Call) => Response) => ({
    ...withTemplates(),
    "POST /print-feedback/create": handler,
  });

  it("asks nothing until the owner says they printed, then moves to the question", async () => {
    agent(CHROME);
    const calls = serve(
      bookData([note(1)]),
      post(() => ok()),
    );
    open();
    expect(screen.queryByText("Does the PDF look right?")).toBeNull();
    await userEvent.click(
      await screen.findByRole("button", { name: "I printed with these settings" }),
    );
    expect(screen.getByRole("heading", { name: "Does the PDF look right?" })).toHaveFocus();
    expect(calls.some((c) => c.path === "/print-feedback/create")).toBe(false);
  });

  it("opens the question by itself when printing ends, without taking the focus", async () => {
    agent(CHROME);
    serve(
      bookData([note(1)]),
      post(() => ok()),
    );
    open();
    await screen.findByRole("button", { name: "I printed with these settings" });
    act(() => void window.dispatchEvent(new Event("afterprint")));
    const heading = await screen.findByRole("heading", { name: "Does the PDF look right?" });
    expect(heading).not.toHaveFocus();
    expect(screen.queryByRole("button", { name: "I printed with these settings" })).toBeNull();
  });

  it.each([
    [
      "Yes, it looks right",
      "yes",
      CHROME,
      "chromium_desktop",
      /nothing about you or your book is sent/,
    ],
    ["No, something is wrong", "no", IPHONE, "safari_ios", /check the steps above once more/],
  ])(
    "%s sends one anonymous answer: %s for %s",
    async (button, verdict, userAgent, family, thanks) => {
      agent(userAgent);
      const calls = serve(
        bookData([note(1)]),
        post(() => ok()),
      );
      open();
      await userEvent.click(
        await screen.findByRole("button", { name: "I printed with these settings" }),
      );
      await userEvent.click(screen.getByRole("button", { name: button }));
      const status = await screen.findByText(thanks);
      expect(status).toHaveFocus();
      const sent = calls.filter((c) => c.path === "/print-feedback/create");
      expect(sent).toHaveLength(1);
      expect(sent[0]!.body).toEqual({ verdict, browser_family: family }); // nothing else: no book, no user agent
      expect(screen.queryByRole("button", { name: button })).toBeNull(); // answered once
    },
  );

  it("keeps the buttons after a refusal, does not retry a 4xx, and lets the owner try again", async () => {
    agent(CHROME);
    let n = 0;
    const calls = serve(
      bookData([note(1)]),
      post(() => (++n === 1 ? err(429, "rate_limited", { "Retry-After": "120" }) : ok())),
    );
    open();
    await userEvent.click(
      await screen.findByRole("button", { name: "I printed with these settings" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Yes, it looks right" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Too many attempts. Try again in 2 min.",
    );
    expect(calls.filter((c) => c.path === "/print-feedback/create")).toHaveLength(1);
    await userEvent.click(screen.getByRole("button", { name: "Yes, it looks right" }));
    expect(await screen.findByText(/nothing about you or your book is sent/)).toBeInTheDocument();
    expect(calls.filter((c) => c.path === "/print-feedback/create")).toHaveLength(2);
  });
});

describe("navy-classic template", () => {
  const friend = (i: number, over: Record<string, string> = {}) =>
    note(i, {
      answers: {
        name: `Nguyễn Thị Hồng Ánh ${i}`,
        relationship: "Bạn cùng lớp",
        message: `Giữ liên lạc nhé ${i}`,
        ...over,
      },
      photo_ids: [PHOTO(i)],
    });
  const navy = (notes: BookData["notes"], language: "en" | "vi" = "en") => {
    const data = bookData(notes);
    data.yearbook.template_id = "navy-classic";
    data.yearbook.language = language;
    data.yearbook.cover_media_id = PHOTO(90);
    data.profile = {
      ...data.profile,
      quote: "Hãy sống",
      hobbies: "Đọc sách",
      future_plans: "Thiết kế",
      photo_media_id: PHOTO(91),
    };
    return data;
  };

  it.each([
    [0, 3, 0],
    [1, 4, 1],
    [4, 4, 1],
    [5, 5, 2],
    [9, 6, 3],
  ])("a book with %i notes has %i pages: %i notes pages of four", async (n, pages, notePages) => {
    serve(navy(Array.from({ length: n }, (_, i) => friend(i + 1))));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(pages));
    expect(sheets("notes")).toHaveLength(notePages);
    expect(document.querySelectorAll(".nc-note")).toHaveLength(n);
    expect(document.querySelectorAll(".nc-page")).toHaveLength(pages);
  });

  it("binds the cover, the profile (oval photo), every note and the back, in the book's language", async () => {
    serve(navy([friend(1)], "vi"));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    const page = (label: string) => sheets(label)[0] as HTMLElement;
    const cover = page("cover").textContent ?? "";
    for (const s of ["THPT Chu Văn An", "Khóa", "2026", "Lớp 12A1", "Học, học nữa, học mãi"])
      expect(cover).toContain(s);
    const profile = page("profile");
    for (const s of ["Giới thiệu về mình", "Đọc sách", "Thiết kế", "Hãy sống", "Nguyễn Thị Hồng"])
      expect(profile.textContent).toContain(s);
    expect(profile.querySelector("img.nc-oval")).not.toBeNull();
    const notes = page("notes").textContent ?? "";
    for (const s of [
      "Lưu bút",
      "Để lại đôi lời",
      "Nguyễn Thị Hồng Ánh 1",
      "Bạn cùng lớp",
      "Giữ liên lạc nhé 1",
    ])
      expect(notes).toContain(s);
    expect(notes).toContain("Tạm biệt, khóa 2026");
    expect(page("back").textContent).toContain("Cảm ơn bạn đã là một phần của cuốn lưu bút này.");
  });

  it("prints English labels for an English book although the app is in Vietnamese", async () => {
    serve(navy([friend(1)], "en"));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    expect(sheets("cover")[0]!.textContent).toContain("Class of");
    expect(sheets("profile")[0]!.textContent).toContain("All about me");
    expect(sheets("notes")[0]!.textContent).toContain("Autographs");
    expect(sheets("notes")[0]!.textContent).toContain("Farewell, Class of 2026");
  });

  it("shows an empty oval when the owner has no photo, and leaves out what is empty", async () => {
    const data = navy([friend(1, { relationship: "" })]);
    data.profile.photo_media_id = null;
    data.profile.hobbies = "";
    data.yearbook.graduation_year = null;
    data.yearbook.motto = "";
    serve(data);
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    const profile = sheets("profile")[0]!;
    expect(profile.querySelector("div.nc-oval")).not.toBeNull();
    expect(profile.querySelector("img")).toBeNull();
    expect(profile.textContent).not.toContain("Hobbies");
    expect(sheets("cover")[0]!.querySelector(".nc-year")).toBeNull();
    expect(sheets("cover")[0]!.querySelector(".nc-motto")).toBeNull();
    expect(sheets("notes")[0]!.textContent).toContain("Farewell");
    expect(sheets("notes")[0]!.querySelectorAll(".nc-sign-line")).toHaveLength(0);
  });

  it("loads only the photo this design shows (the owner's portrait), not the cover or the friends' photos", async () => {
    serve(navy([friend(1), friend(2)]));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    expect(requested).toHaveLength(1);
    expect(requested[0]).toContain(PHOTO(91));
    expect(document.querySelectorAll("img")).toHaveLength(1);
  });

  it("draws user text as text, never as HTML", async () => {
    const evil = "<img src=x onerror=alert(1)><b>đậm</b> & 😄";
    serve(navy([friend(1, { name: evil, message: evil, relationship: evil })]));
    open();
    await waitFor(() => expect(sheets()).toHaveLength(4));
    expect(document.querySelector("img[src='x']")).toBeNull();
    expect(document.querySelector(".nc-page b")).toBeNull();
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
      const medium = note(2, { answers: { name: "Hoa", message: "y".repeat(50) } });
      const short = note(3, { answers: { name: "Minh", message: "ok" } });
      serve(navy([long, medium, short]));
      open();
      const summary = await screen.findByText("Notes in a smaller size or cut off: 2");
      const box = summary.closest("details")!;
      expect(box).toHaveAttribute("open");
      const items = [...box.querySelectorAll("li")];
      expect(items).toHaveLength(2); // Minh fits as designed
      // A5: the floor is 50 % of 19 design px = 9.5 px = 4.9 pt
      expect(items[0]!.textContent).toContain("Linh, 5 pt, and still cut off");
      expect(items[1]!.textContent).toContain("Hoa, ");
      expect(items[1]!.textContent).not.toContain("cut off");
      const links = [...box.querySelectorAll("a")].map((a) => a.getAttribute("href"));
      expect(links).toEqual([
        `/yearbooks/${BOOK}/notes?note=${long.id}`,
        `/yearbooks/${BOOK}/notes?note=${medium.id}`,
      ]);
    });

    it("says nothing when every note fits", async () => {
      serve(navy([note(1, { answers: { name: "Minh", message: "ok" } })]));
      open();
      await waitFor(() => expect(sheets()).toHaveLength(4));
      expect(screen.queryByText(/Notes in a smaller size/)).not.toBeInTheDocument();
    });
  });
});
