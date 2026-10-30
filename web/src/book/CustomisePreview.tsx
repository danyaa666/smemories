import type { TFunction } from "i18next";
import { useCallback, useMemo, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import type { BookData } from "../api/book";
import type { DesignBlock } from "../api/bookDesign";
import { useStageScale } from "../pages/Book";
import { BookPages, pageCount } from "./BookPages";
import { previewOf, type Draft } from "./customise";
import { SIZES } from "./sizes";
import "./book.css";

/**
 * The live preview of the customise screen: the real book render (BookPages) of the draft, at screen scale, showing only the pages of the control that has
 * the focus (`focus`; all the sections when it is null) with one notes page. It is driven by local state, never by the server, and loads photos through the
 * same queue as the book (never more than 16 in flight). There is no printing from here.
 */
export function CustomisePreview({
  data,
  draft,
  block,
  focus,
  bookT,
}: {
  data: BookData;
  draft: Draft;
  block: DesignBlock;
  focus: string | null;
  bookT: TFunction;
}) {
  const { t } = useTranslation();
  const size = data.yearbook.page_size;
  const shown = useMemo(() => previewOf(data, draft, block, focus), [data, draft, block, focus]);
  const { ref, scale } = useStageScale(size);
  const track = useCallback(() => {}, []); // the preview never waits for its photos
  const stageStyle = {
    "--pw": `${SIZES[size].w}mm`,
    "--ph": `${SIZES[size].h}mm`,
    "--s": scale,
  } as CSSProperties;
  const empty = pageCount(shown) === 0;
  return (
    <section className="cu-preview" aria-labelledby="cu-preview-h">
      <h2 id="cu-preview-h">{t("customise.preview")}</h2>
      <p className="cu-help">{t("customise.previewHelp")}</p>
      {empty && <p role="status">{t("customise.previewNoNotes")}</p>}
      <div ref={ref} className="bk-stage" style={stageStyle}>
        <BookPages data={shown} size={size} t={bookT} eager={false} track={track} />
      </div>
    </section>
  );
}
