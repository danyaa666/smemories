import type { BookData } from "../api/book";
import type { ClassBook, ClassBookMember } from "../api/classBook";
import { profilePageCount } from "./BookPages";
import type { FitInfo } from "./fit";
import { noteFits } from "./fitReport";
import { SIZES, type PageSize } from "./sizes";
import { getTemplate } from "./templates";

/** The page budget of a class book (E07 design): a longer book is printed, but the screen warns. */
export const PAGE_BUDGET = 120;

/** The keys of the fit report: one prefix per student, one per class page. */
export const memberPrefix = (m: ClassBookMember) => `m:${m.member_id}:`;
/** The fit key of the class name on the cover. */
export const COVER_KEY = "cc:title";
export const pageKey = (id: string) => `cp:${id}`;

/** The student's own yearbook as the existing templates want it (`tpl.Profile`, `tpl.Notes`); null when nothing is printed for them. */
export function memberBook(
  cls: ClassBook["class"],
  m: ClassBookMember,
  size: PageSize,
): BookData | null {
  if (!m.linked || !m.yearbook || !m.profile) return null;
  return {
    yearbook: {
      title: cls.name,
      school_name: cls.school_name,
      class_name: cls.name,
      graduation_year: cls.graduation_year,
      motto: "",
      language: m.yearbook.language, // the student's own page keeps its language
      page_size: size, // one paper size for the whole book
      template_id: m.yearbook.template_id,
      cover_media_id: null,
    },
    profile: m.profile,
    notes: m.notes,
    note_count: m.note_count,
    approved_total: m.approved_total,
    truncated: m.truncated,
  };
}

/** The size most students chose for their own book (a tie goes to the first of A5, A4, Letter): the default paper of the class book. */
export function commonSize(members: readonly ClassBookMember[]): PageSize {
  const count: Record<string, number> = {};
  for (const m of members)
    if (m.linked && m.yearbook)
      count[m.yearbook.page_size] = (count[m.yearbook.page_size] ?? 0) + 1;
  const sizes = Object.keys(SIZES) as PageSize[];
  return sizes.reduce((best, s) => ((count[s] ?? 0) > (count[best] ?? 0) ? s : best), "A5");
}

/** Cover, class pages in front, every student's pages, class pages at the back, back cover. */
export function classPageCount(book: ClassBook, size: PageSize): number {
  const students = book.members.reduce((n, m) => {
    const d = memberBook(book.class, m, size);
    return n + (d ? profilePageCount(d) : 0);
  }, 0);
  return 2 + book.front_pages.length + students + book.back_pages.length;
}

/** Public ids of the photos the student's pages show (the profile photo and, if the template draws them, the notes' photos). */
export function memberPhotoIds(d: BookData): string[] {
  const ids = [
    d.profile.photo_media_id,
    ...(getTemplate(d.yearbook.template_id).photos?.notes === false
      ? []
      : d.notes.flatMap((n) => n.photo_ids)),
  ];
  return ids.filter((x): x is string => !!x);
}

export type StudentIssue = {
  id: string;
  name: string;
  /** Approved notes the class book leaves out (the per-student limit). */
  dropped: number;
  shown: number;
  total: number;
  /** Notes still cut off at the template's smallest text size. */
  cutNotes: number;
  /** Profile texts still cut off. */
  cutProfile: number;
};

export type ClassSummary = {
  /** Students for whom nothing is printed. */
  noPage: ClassBookMember[];
  /** Students whose notes were cut by the limit or by the template's floor, or whose profile text was cut. */
  issues: StudentIssue[];
  /** The class name on the cover is cut off at the smallest size. */
  coverCut: boolean;
  /** Class pages whose text is cut off at the smallest size. */
  cutPages: ClassBook["front_pages"];
};

/** What the printout leaves out, from the book and what the fit reported. Nothing is cut that is not listed here. */
export function classSummary(
  book: ClassBook,
  size: PageSize,
  states: Record<string, FitInfo>,
): ClassSummary {
  const noPage: ClassBookMember[] = [];
  const issues: StudentIssue[] = [];
  for (const m of book.members) {
    const d = memberBook(book.class, m, size);
    if (!d) {
      noPage.push(m);
      continue;
    }
    const prefix = memberPrefix(m);
    const dropped = Math.max(0, m.approved_total - m.note_count);
    const cutNotes = noteFits(states, d).filter((f) => f.cut).length;
    const cutProfile = Object.entries(states).filter(
      ([k, i]) => k.startsWith(prefix) && i.state === "overflow",
    ).length;
    if (dropped || cutNotes || cutProfile)
      issues.push({
        id: m.member_id,
        name: m.name,
        dropped,
        shown: m.note_count,
        total: m.approved_total,
        cutNotes,
        cutProfile,
      });
  }
  const cutPages = [...book.front_pages, ...book.back_pages].filter(
    (p) => states[pageKey(p.id)]?.state === "overflow",
  );
  return { noPage, issues, coverCut: states[COVER_KEY]?.state === "overflow", cutPages };
}
