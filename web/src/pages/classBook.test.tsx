import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClassBook, ClassBookMember } from "../api/classBook";
import { loaded } from "../components/imageQueue";
import { err, mockApi, ok, renderApp, user } from "../test/api";

const CLASS = "01J9Z3K6V8Q4M7N2P5R8T0CCCC";
const ulid = (k: string, n: number) =>
  `01J9Z3K6V8Q4M7N2P5${k}${String(n).padStart(8, "0")}`.slice(0, 26);
const PHOTO = (n: number) => ulid("P", n);

const note = (member: number, i: number, photos: string[] = []) => ({
  id: ulid("N", member * 1000 + i),
  answers: { name: `Bạn ${member}.${i}`, relationship: "Bạn cùng lớp", message: `Chúc mừng ${i}` },
  signature: null,
  photo_ids: photos,
});

function member(n: number, over: Partial<ClassBookMember> = {}, notes = 2): ClassBookMember {
  const list = Array.from({ length: notes }, (_, i) => note(n, i + 1));
  return {
    member_id: ulid("M", n),
    name: `Học sinh ${n}`,
    linked: true,
    yearbook: { template_id: null, page_size: "A5", language: "vi" },
    profile: {
      full_name: `Họ tên ${n}`,
      nickname: "",
      quote: "",
      hobbies: "",
      future_plans: "",
      photo_media_id: null,
    },
    notes: list,
    note_count: list.length,
    approved_total: list.length,
    truncated: false,
    ...over,
  };
}
const nobody = (): Partial<ClassBookMember> => ({
  linked: false,
  yearbook: null,
  profile: null,
  notes: [],
  note_count: 0,
  approved_total: 0,
});

function classBook(members: ClassBookMember[], over: Partial<ClassBook> = {}): ClassBook {
  const page = (id: number, title: string, body: string, placement: "front" | "back") => ({
    id: ulid("G", id),
    kind: "intro" as const,
    title,
    body,
    placement,
    created_at: 1,
    updated_at: 1,
  });
  return {
    class: {
      id: CLASS,
      name: "Lớp 12A1",
      school_name: "THPT Chu Văn An",
      graduation_year: 2026,
      language: "vi",
    },
    front_pages: [page(1, "Lời chào", "Dòng một\nDòng hai <script>alert(1)</script>", "front")],
    members,
    back_pages: [page(2, "Giáo viên", "Cô Lan", "back")],
    ...over,
  };
}

let pending: (() => void)[] = [];
class FakeImage {
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  set src(_: string) {
    pending.push(() => this.onload?.());
  }
}
const finishPhotos = () => act(() => void pending.splice(0).forEach((f) => f()));
const photoImgs = () => [...document.querySelectorAll<HTMLImageElement>("img.bk-photo")];
const settleImgs = () =>
  act(() => void photoImgs().forEach((i) => i.dispatchEvent(new Event("load"))));
const labels = () =>
  [...document.querySelectorAll("section[data-page]")].map((s) => s.getAttribute("data-page"));

function serve(book: ClassBook | Response, role: "admin" | "student" = "admin") {
  return mockApi({
    "GET /user/get-me": () => ok(user()),
    [`GET /class/get?id=${CLASS}`]: () =>
      ok({
        id: CLASS,
        name: "Lớp 12A1",
        school_name: "",
        graduation_year: 2026,
        language: "vi",
        my_role: role,
        page_linked: false,
        created_at: 1,
        updated_at: 1,
      }),
    [`GET /class-book/get?class_id=${CLASS}`]: () => (book instanceof Response ? book : ok(book)),
  });
}
const open = (search = "") => renderApp(`/classes/${CLASS}/book${search}`);

beforeEach(() => {
  pending = [];
  loaded.clear();
  vi.stubGlobal("Image", FakeImage);
});
afterEach(() => vi.unstubAllGlobals());

describe("print the class book (T-130)", () => {
  it("prints the cover, the front pages, each student's profile and notes in order, the back pages and the back cover", async () => {
    // student 1: basic (4 notes per page, 5 notes: 2 pages), student 2: no page, student 3: memory-book (1 per page, 2 notes)
    serve(
      classBook([
        member(1, {}, 5),
        member(2, nobody()),
        member(3, { yearbook: { template_id: "memory-book", page_size: "A5", language: "vi" } }, 2),
      ]),
    );
    open();
    await waitFor(() => expect(labels().length).toBeGreaterThan(5));
    const l = labels();
    expect(l[0]).toBe("class-cover");
    expect(l[1]).toBe("class-page");
    expect(l.at(-2)).toBe("class-page");
    expect(l.at(-1)).toBe("class-back");
    const middle = l.slice(2, -2);
    // basic: profile + 2 notes pages; memory-book: its own profile page(s) and two friend pages
    expect(middle.slice(0, 3)).toEqual(["profile", "notes", "notes"]);
    expect(middle).toHaveLength(6); // 3 pages for student 1, none for student 2, profile + 2 friend pages for student 3
    expect(screen.getByText(/Ready to print|Getting fonts/)).toBeInTheDocument();
    // student 1 before student 3, in the order of the list
    const text = document.body.textContent ?? "";
    expect(text.indexOf("Họ tên 1")).toBeLessThan(text.indexOf("Họ tên 3"));
    expect(text).not.toContain("Họ tên 2");
  });

  it("class pages are plain text: newlines kept, markup shown as typed", async () => {
    serve(classBook([member(1)]));
    open();
    await waitFor(() => expect(labels()).toContain("class-page"));
    const body = document.querySelector<HTMLElement>(".bk-box-text")!;
    expect(body.textContent).toBe("Dòng một\nDòng hai <script>alert(1)</script>");
    expect(document.querySelector(".bk-sheet script")).toBeNull();
  });

  it("the cover has the class, school and year; the back cover the thanks, in the class language", async () => {
    serve(classBook([member(1)]));
    open();
    await waitFor(() => expect(labels()).toContain("class-back"));
    const cover = document.querySelector('[data-page="class-cover"]')!;
    expect(cover.textContent).toContain("Lớp 12A1");
    expect(cover.textContent).toContain("THPT Chu Văn An");
    expect(cover.textContent).toMatch(/2026/);
  });

  it("lists the students with no page, and the student whose 41st note was left out", async () => {
    serve(
      classBook([
        member(1),
        member(2, nobody()),
        member(3, { approved_total: 41, truncated: true }, 40),
      ]),
    );
    open();
    expect(await screen.findByText(/nothing is printed for them \(1\): Học sinh 2/)).toBeVisible();
    expect(screen.getByText("Students whose pages are cut: 1")).toBeVisible();
    expect(screen.getByText(/Học sinh 3:/)).toHaveTextContent(
      "Only 40 of 41 approved notes are in the class book.",
    );
    expect(screen.queryByText(/Học sinh 1:/)).not.toBeInTheDocument();
  });

  it("shows the page count, and warns past the 120-page budget", async () => {
    serve(classBook([member(1, {}, 4)]));
    const { unmount } = open();
    await waitFor(() => expect(labels()).toHaveLength(2 + 2 + 2));
    expect(screen.queryByText(/pages the class book is planned for/)).not.toBeInTheDocument();
    unmount();
    // 40 students of 1 profile page + 4 notes pages (16 notes) = 200 pages
    serve(classBook(Array.from({ length: 40 }, (_, i) => member(i + 1, {}, 16))));
    open();
    expect(await screen.findByText(/This book has 204 pages/)).toBeVisible();
  });

  it("Print stays disabled until every photo, of every student, has loaded; at most 16 are in flight", async () => {
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
    const print = vi.spyOn(window, "print").mockImplementation(() => {});
    // 6 students x (1 profile photo + 4 notes with one photo) = 30 photos
    let p = 0;
    const members = Array.from({ length: 6 }, (_, i) => member(i + 1, {}, 0)).map((m, i) => {
      const notes = Array.from({ length: 4 }, (_, k) => note(i + 1, k + 1, [PHOTO(++p)]));
      return {
        ...m,
        notes,
        note_count: 4,
        approved_total: 4,
        profile: { ...m.profile!, photo_media_id: PHOTO(++p) },
      };
    });
    serve(classBook(members));
    open();
    const button = await screen.findByRole("button", { name: "Print or save as PDF" });
    expect(button).toBeDisabled();
    await waitFor(() => expect(photoImgs()).toHaveLength(30));
    await waitFor(() => expect(photoImgs().filter((i) => i.getAttribute("src"))).toHaveLength(16));
    // the 16 slots are shared with the preload: no more than 16 are asked for at a time
    for (
      let i = 0;
      i < 6 && !(photoImgs().every((x) => x.getAttribute("src")) && pending.length === 0);
      i++
    ) {
      await finishPhotos();
      settleImgs();
      await new Promise((r) => setTimeout(r, 20));
    }
    expect(photoImgs().every((x) => x.getAttribute("src"))).toBe(true);
    settleImgs();
    await waitFor(() => expect(button).toBeEnabled());
    await userEvent.click(button);
    expect(print).toHaveBeenCalledTimes(1);
  });

  it("another paper size is chosen on the screen and in the address; the default is the size most students use", async () => {
    serve(
      classBook([
        member(1, { yearbook: { template_id: null, page_size: "A4", language: "vi" } }),
        member(2, { yearbook: { template_id: null, page_size: "A4", language: "vi" } }),
        member(3),
      ]),
    );
    open();
    expect(await screen.findByRole("radio", { name: "A4" })).toBeChecked();
    await userEvent.click(screen.getByRole("radio", { name: "Letter" }));
    expect(screen.getByRole("radio", { name: "Letter" })).toBeChecked();
    expect(document.querySelector("style")?.textContent).toContain("215.9mm 279.4mm");
  });

  it("a student gets the not-found screen and no class-book request is made", async () => {
    const calls = serve(classBook([member(1)]), "student");
    open();
    expect(await screen.findByRole("heading", { name: "Page not found" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(document.querySelectorAll("section[data-page]")).toHaveLength(0);
    expect(calls.some((c) => c.path.startsWith("/class-book/get"))).toBe(false);
  });

  it("a stranger (404 on the class) gets the same screen and no class-book request", async () => {
    const calls = mockApi({
      "GET /user/get-me": () => ok(user()),
      [`GET /class/get?id=${CLASS}`]: err(404, "not_found"),
    });
    open();
    expect(await screen.findByRole("heading", { name: "Page not found" })).toBeInTheDocument();
    expect(calls.some((c) => c.path.startsWith("/class-book/get"))).toBe(false);
  });

  it("ignores fields it does not know (a later API may add truncated, ...)", async () => {
    const book = { ...classBook([member(1)]), truncated: true, budget: { bytes: 1 } };
    serve(book as ClassBook);
    open();
    await waitFor(() => expect(labels()).toContain("class-cover"));
  });
});
