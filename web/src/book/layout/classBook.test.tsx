import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ClassBook, ClassBookMember } from "../../api/classBook";
import i18n from "../../i18n";
import { ClassBookPages } from "../ClassBookPages";
import { memberBook } from "../classBook";
import type { LayoutDoc } from "./runtime";

// T-154 (AC4): the class book draws each student's layout on their own pages only. class-book/get does not carry `layout` yet (the web reads it as soon as it does).

const text = (id: string, s: string) => ({
  id,
  t: "text" as const,
  x: 100,
  w: 400,
  h: 120,
  cy: 800,
  text: s,
});
const doc = (kind: string, id: string, s: string): LayoutDoc => ({
  v: 1,
  pages: { [kind]: { items: [text(id, s)] } },
});

const member = (
  n: number,
  layout: LayoutDoc | null,
  template = "memory-book",
): ClassBookMember & { layout: LayoutDoc | null } => ({
  member_id: `M${n}`,
  name: `S${n}`,
  linked: true,
  yearbook: { template_id: template, page_size: "A5", language: "vi" },
  profile: {
    full_name: `S${n}`,
    nickname: "",
    quote: "",
    hobbies: "",
    future_plans: "",
    photo_media_id: null,
  },
  notes: [{ id: `N${n}`, answers: { name: "x" }, signature: null, photo_ids: [] }],
  note_count: 1,
  approved_total: 1,
  truncated: false,
  design: null,
  layout,
});
const book = (members: ClassBookMember[]): ClassBook => ({
  class: { id: "C", name: "12A1", school_name: "", graduation_year: null, language: "vi" },
  front_pages: [],
  members,
  back_pages: [],
});
const html = (b: ClassBook) =>
  renderToStaticMarkup(
    <ClassBookPages book={b} size="A5" t={i18n.getFixedT("vi")} eager={false} track={() => {}} />,
  );

describe("the class book draws each student's layout", () => {
  const a = member(1, doc("profile", "aaaaaa", "ALPHA"));
  const b = member(2, doc("notes", "bbbbbb", "BETA"));
  it("memberBook hands the student's own layout to their pages, null when the API has none", () => {
    expect(memberBook(book([]).class, a, "A5")?.layout).toEqual(a.layout);
    const bare: ClassBookMember = { ...a };
    delete (bare as { layout?: unknown }).layout;
    expect(memberBook(book([]).class, bare, "A5")?.layout).toBeNull();
  });
  it("each layout appears on its own student's pages and nowhere else", () => {
    const out = html(book([a, b, member(3, null)]));
    const pages = out.split('<div class="bk-frame">');
    const has = (needle: string) => pages.filter((p) => p.includes(needle));
    expect(has("ALPHA")).toHaveLength(1);
    expect(has("ALPHA")[0]).toContain('data-page="profile"');
    expect(has("BETA")).toHaveLength(1); // student B has one note: one notes page
    expect(has("BETA")[0]).toContain('data-page="notes"');
    expect(out.indexOf("ALPHA")).toBeLessThan(out.indexOf("BETA"));
  });
  it("a student on a template without a layout block, or with no layout, draws as before", () => {
    const plain = html(book([member(1, null), member(2, null, "navy-classic")]));
    expect(html(book([member(1, doc("cover", "aaaaaa", "ALPHA"), "navy-classic")]))).not.toContain(
      "ALPHA",
    );
    expect(plain).not.toContain("data-layout");
  });
});
