import type { TFunction } from "i18next";
import { memo, useMemo } from "react";
import type { BookData } from "../api/book";
import { BookEnvProvider } from "./env";
import { paginate } from "./paginate";
import type { PageSize } from "./sizes";
import { getTemplate } from "./templates";

/** Cover, profile, the notes pages and the back: 3 + ceil(notes / capacity) pages. */
export function pageCount(data: BookData): number {
  const per = getTemplate(data.yearbook.template_id).notesPerPage;
  return 3 + paginate(data.notes, per).length;
}

/** Every page of the book as fixed-size sheets. Memoised: a change of fit state or photo progress does not re-render the pages. */
export const BookPages = memo(function BookPages({
  data,
  size,
  t,
  eager,
}: {
  data: BookData;
  size: PageSize;
  t: TFunction;
  eager: boolean;
}) {
  const tpl = getTemplate(data.yearbook.template_id);
  const env = useMemo(() => ({ size, t, eager }), [size, t, eager]);
  const noteGroups = useMemo(() => paginate(data.notes, tpl.notesPerPage), [data.notes, tpl]);
  return (
    <BookEnvProvider value={env}>
      <tpl.Cover data={data} />
      <tpl.Profile data={data} />
      {noteGroups.map((g, gi) => (
        <tpl.Notes key={g[0]?.id} notes={g} data={data} first={gi * tpl.notesPerPage} />
      ))}
      <tpl.Back data={data} />
    </BookEnvProvider>
  );
});
