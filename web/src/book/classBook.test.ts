import { describe, expect, it } from "vitest";
import type { ClassBook, ClassBookMember } from "../api/classBook";
import {
  COVER_KEY,
  classPageCount,
  classSummary,
  commonSize,
  memberBook,
  memberPrefix,
  pageKey,
} from "./classBook";
import type { FitInfo } from "./fit";

const m = (n: number, over: Partial<ClassBookMember> = {}): ClassBookMember => ({
  member_id: `M${n}`,
  name: `S${n}`,
  linked: true,
  yearbook: { template_id: null, page_size: "A5", language: "vi" },
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
  ...over,
});
const none = {
  linked: false,
  yearbook: null,
  profile: null,
  notes: [],
  note_count: 0,
  approved_total: 0,
};
const book = (members: ClassBookMember[]): ClassBook => ({
  class: { id: "C", name: "12A1", school_name: "", graduation_year: null, language: "vi" },
  front_pages: [],
  members,
  back_pages: [],
});
const cut: FitInfo = { state: "overflow", base: 16, px: 8 };

describe("class book arithmetic", () => {
  it("counts cover, back cover and each student's profile and notes pages; a student with no page adds none", () => {
    expect(classPageCount(book([]), "A5")).toBe(2);
    expect(classPageCount(book([m(1), m(2, none)]), "A5")).toBe(2 + 2);
  });
  it("the common size is the most used one, A5 on a tie or with no pages", () => {
    const a4 = {
      yearbook: { template_id: null, page_size: "A4" as const, language: "vi" as const },
    };
    expect(commonSize([])).toBe("A5");
    expect(commonSize([m(1, a4), m(2, a4), m(3)])).toBe("A4");
    expect(commonSize([m(1, a4), m(2)])).toBe("A5");
  });
  it("draws a student's book in the class's paper size and the student's own template", () => {
    const d = memberBook(
      book([]).class,
      m(1, { yearbook: { template_id: "navy-classic", page_size: "A4", language: "vi" } }),
      "Letter",
    );
    expect(d?.yearbook).toMatchObject({
      page_size: "Letter",
      template_id: "navy-classic",
      title: "12A1",
    });
    expect(memberBook(book([]).class, m(2, none), "A5")).toBeNull();
  });
});

describe("what the summary names", () => {
  it("lists the students whose notes were cut by the limit or by the template floor, whose profile was cut, and those with no page", () => {
    const b = book([
      m(1), // fine
      m(2, { approved_total: 41, truncated: true }), // 41st note left out
      m(3), // the template floor
      m(4), // profile text
      m(5, none),
    ]);
    const states = {
      "note:N3": cut,
      "note:N1": { state: "shrunk", base: 16, px: 14 } as FitInfo, // smaller is not cut
      [`${memberPrefix(m(4))}profile:quote`]: cut,
    };
    const s = classSummary(b, "A5", states);
    expect(s.noPage.map((x) => x.name)).toEqual(["S5"]);
    expect(s.issues.map((i) => [i.name, i.dropped, i.cutNotes, i.cutProfile])).toEqual([
      ["S2", 40, 0, 0],
      ["S3", 0, 1, 0],
      ["S4", 0, 0, 1],
    ]);
  });
  it("flags a class name that is cut off on the cover", () => {
    const b = book([m(1)]);
    expect(classSummary(b, "A5", { [COVER_KEY]: cut }).coverCut).toBe(true);
    expect(classSummary(b, "A5", {}).coverCut).toBe(false);
    expect(classSummary(b, "A5", { [COVER_KEY]: { state: "ok", base: 56, px: 56 } }).coverCut).toBe(
      false,
    );
  });
  it("names a class page that is cut off", () => {
    const b = book([m(1)]);
    b.front_pages = [
      {
        id: "P1",
        kind: "intro",
        title: "Hi",
        body: "x",
        placement: "front",
        created_at: 1,
        updated_at: 1,
      },
    ];
    expect(classSummary(b, "A5", { [pageKey("P1")]: cut }).cutPages.map((p) => p.id)).toEqual([
      "P1",
    ]);
    expect(classSummary(b, "A5", {}).cutPages).toEqual([]);
  });
});
