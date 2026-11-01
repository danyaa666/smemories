import type { TFunction } from "i18next";
import { memo, useMemo } from "react";
import type { BookData } from "../api/book";
import { DesignProvider, printSections, type SectionId } from "./design";
import { BookEnvProvider } from "./env";
import { LayoutProvider, type TextEdit } from "./layout/runtime";
import { paginate } from "./paginate";
import type { PageSize } from "./sizes";
import { getTemplate } from "./templates";

const notePages = (data: BookData) =>
  paginate(data.notes, getTemplate(data.yearbook.template_id).notesPerPage).length;

/** The profile page and the notes pages of one person, as far as the design lets them print: 1 + ceil(notes / capacity) pages by default. */
export function profilePageCount(data: BookData): number {
  const s = printSections(data.design);
  return (s.includes("profile") ? 1 : 0) + (s.includes("notes") ? notePages(data) : 0);
}

/** Every page that prints: cover, profile, the notes pages and the back (3 + ceil(notes / capacity) by default), without the hidden sections. */
export function pageCount(data: BookData): number {
  const s = printSections(data.design);
  return profilePageCount(data) + (s.includes("cover") ? 1 : 0) + (s.includes("back") ? 1 : 0);
}

/** The pages of one section of the book (the notes section is one page per group of notes). */
function Section({ id, data, only }: { id: SectionId; data: BookData; only?: number }) {
  const tpl = getTemplate(data.yearbook.template_id);
  const noteGroups = useMemo(() => paginate(data.notes, tpl.notesPerPage), [data.notes, tpl]);
  if (id === "cover") return <tpl.Cover data={data} />;
  if (id === "profile") return <tpl.Profile data={data} />;
  if (id === "back") return <tpl.Back data={data} />;
  return (
    <>
      {noteGroups.map((g, gi) =>
        only === undefined || only === gi ? (
          <tpl.Notes key={g[0]?.id} notes={g} data={data} first={gi * tpl.notesPerPage} />
        ) : null,
      )}
    </>
  );
}

/** The pages that print, in book order, each as the `OnePage` that draws it (the notes section is one page per group of notes). */
export function listPages(data: BookData): OnePage[] {
  const per = getTemplate(data.yearbook.template_id).notesPerPage;
  return printSections(data.design).flatMap((section): OnePage[] =>
    section === "notes"
      ? paginate(data.notes, per).map((_, index) => ({ section, index }))
      : [{ section }],
  );
}

/** One person's group of pages (profile, then the notes pages, or the order and visibility of their design) in their template. Needs a BookEnvProvider; the class book (T-130) repeats it per student. */
export function ProfilePages({ data }: { data: BookData }) {
  return (
    <>
      {printSections(data.design)
        .filter((s) => s === "profile" || s === "notes")
        .map((s) => (
          <Section key={s} id={s} data={data} />
        ))}
    </>
  );
}

/** One page of the book: a section, and for the notes section which of its pages (T-159: the editor's stage and page strip draw one page at a time). */
export type OnePage = { section: SectionId; index?: number };

/** Every page of the book as fixed-size sheets, or just `only` one of them. Memoised: a change of fit state or photo progress does not re-render the pages. */
export const BookPages = memo(function BookPages({
  data,
  size,
  t,
  eager,
  track,
  only,
  edit,
}: {
  data: BookData;
  size: PageSize;
  t: TFunction;
  eager: boolean;
  track: (delta: number) => void;
  only?: OnePage;
  /** The editor's text field over one text item (T-161); absent everywhere else. */
  edit?: TextEdit;
}) {
  const tpl = getTemplate(data.yearbook.template_id);
  const env = useMemo(() => ({ size, t, eager, track }), [size, t, eager, track]);
  return (
    <BookEnvProvider value={env}>
      <DesignProvider design={data.design} ids={tpl.design}>
        <LayoutProvider layout={data.layout} spec={tpl.layout} edit={edit}>
          {only ? (
            <Section id={only.section} data={data} only={only.index} />
          ) : (
            printSections(data.design).map((s) => <Section key={s} id={s} data={data} />)
          )}
        </LayoutProvider>
      </DesignProvider>
    </BookEnvProvider>
  );
});
