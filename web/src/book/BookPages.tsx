import type { TFunction } from "i18next";
import { memo, useMemo } from "react";
import type { BookData } from "../api/book";
import { DesignProvider } from "./design";
import { BookEnvProvider } from "./env";
import { paginate } from "./paginate";
import type { PageSize } from "./sizes";
import { getTemplate } from "./templates";

/** The profile page and the notes pages of one person: 1 + ceil(notes / capacity) pages. */
export function profilePageCount(data: BookData): number {
  const per = getTemplate(data.yearbook.template_id).notesPerPage;
  return 1 + paginate(data.notes, per).length;
}

/** Cover, profile, the notes pages and the back: 3 + ceil(notes / capacity) pages. */
export function pageCount(data: BookData): number {
  return 2 + profilePageCount(data);
}

/** One person's group of pages (profile, then the notes pages) in their template. Needs a BookEnvProvider; the class book (T-130) repeats it per student. */
export function ProfilePages({ data }: { data: BookData }) {
  const tpl = getTemplate(data.yearbook.template_id);
  const noteGroups = useMemo(() => paginate(data.notes, tpl.notesPerPage), [data.notes, tpl]);
  return (
    <>
      <tpl.Profile data={data} />
      {noteGroups.map((g, gi) => (
        <tpl.Notes key={g[0]?.id} notes={g} data={data} first={gi * tpl.notesPerPage} />
      ))}
    </>
  );
}

/** Every page of the book as fixed-size sheets. Memoised: a change of fit state or photo progress does not re-render the pages. */
export const BookPages = memo(function BookPages({
  data,
  size,
  t,
  eager,
  track,
}: {
  data: BookData;
  size: PageSize;
  t: TFunction;
  eager: boolean;
  track: (delta: number) => void;
}) {
  const tpl = getTemplate(data.yearbook.template_id);
  const env = useMemo(() => ({ size, t, eager, track }), [size, t, eager, track]);
  return (
    <BookEnvProvider value={env}>
      <DesignProvider design={data.design} ids={tpl.design}>
        <tpl.Cover data={data} />
        <ProfilePages data={data} />
        <tpl.Back data={data} />
      </DesignProvider>
    </BookEnvProvider>
  );
});
