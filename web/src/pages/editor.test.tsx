import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BookData } from "../api/book";
import { err, mockApi, ok, renderApp, user } from "../test/api";

// T-159: the editor route: owner only, narrow screens, templates without a layout, and the way in from the customise screen.

const BOOK = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";
const yearbook = {
  id: BOOK,
  title: "Lớp 12A1",
  school_name: "THPT Chu Văn An",
  class_name: "12A1",
  graduation_year: 2026,
  motto: "",
  language: "vi" as const,
  page_size: "A5" as const,
  template_id: "memory-book",
  cover_media_id: null,
  created_at: 1760000000000,
  updated_at: 1760086400000,
  profile: {
    id: "p",
    is_owner: true,
    full_name: "Lan",
    nickname: "",
    birthday: null,
    quote: "",
    hobbies: "",
    future_plans: "",
    photo_media_id: null,
  },
};
const bookData = (template = "memory-book"): BookData => ({
  design: null,
  layout: null,
  yearbook: {
    ...yearbook,
    language: "vi",
    page_size: "A5",
    template_id: template,
  },
  profile: yearbook.profile,
  notes: [],
  note_count: 0,
  approved_total: 0,
  truncated: false,
});
const serve = (opts: { template?: string; gone?: boolean } = {}) =>
  mockApi({
    "GET /user/get-me": () => ok(user()),
    [`GET /yearbook/get?id=${BOOK}`]: () => (opts.gone ? err(404, "not_found") : ok(yearbook)),
    [`GET /book-data/get?yearbook_id=${BOOK}`]: () => ok(bookData(opts.template)),
    [`GET /book-design/get?yearbook_id=${BOOK}`]: () =>
      ok({
        template_id: opts.template ?? "memory-book",
        options: null,
        updated_at: null,
        design: null,
      }),
    [`GET /book-layout/get?yearbook_id=${BOOK}`]: () =>
      ok({ layout: null, version: 0, template_id: "memory-book", updated_at: null }),
  });
const narrow = (on: boolean) =>
  vi.stubGlobal("matchMedia", (q: string) => ({
    matches: on && q.includes("max-width: 767px"),
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
afterEach(() => vi.unstubAllGlobals());

describe("the editor route", () => {
  it("mounts the editor for the owner, with a link back to customise", async () => {
    const calls = serve();
    renderApp(`/yearbooks/${BOOK}/editor`);
    expect(await screen.findByRole("navigation", { name: "Pages" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Free layout" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to customise" })).toBeInTheDocument();
    expect(calls.filter((c) => c.path.startsWith("/book-layout/get")).length).toBe(1);
  });

  it("does not mount below 768 px: a message, a link back and no layout request", async () => {
    narrow(true);
    const calls = serve();
    renderApp(`/yearbooks/${BOOK}/editor`);
    expect(await screen.findByText(/needs a screen at least 768 px wide/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to customise" })).toHaveAttribute(
      "href",
      `/yearbooks/${BOOK}/customise`,
    );
    expect(screen.queryByRole("navigation", { name: "Pages" })).toBeNull();
    expect(calls.some((c) => c.path.startsWith("/book-layout"))).toBe(false);
  });

  it("asks nothing more for a book that is not the caller's", async () => {
    const calls = serve({ gone: true });
    renderApp(`/yearbooks/${BOOK}/editor`);
    await waitFor(() => expect(calls.some((c) => c.path.startsWith("/yearbook/get"))).toBe(true));
    await screen.findByText("This yearbook does not exist.");
    expect(calls.some((c) => c.path.startsWith("/book-data"))).toBe(false);
    expect(calls.some((c) => c.path.startsWith("/book-layout"))).toBe(false);
  });

  it("says so for a template that cannot hold a layout", async () => {
    const calls = serve({ template: "classic" });
    renderApp(`/yearbooks/${BOOK}/editor`);
    expect(await screen.findByText(/does not support free layout/)).toBeInTheDocument();
    expect(calls.some((c) => c.path.startsWith("/book-layout"))).toBe(false);
  });
});
