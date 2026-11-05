import { useMemo, type CSSProperties } from "react";
import i18n from "../../i18n";
import type { BookData } from "../../api/book";
import { BookPages, type OnePage } from "../BookPages";
import { SECTION_IDS, type SectionId } from "../design";
import { pagePx, SIZES, type PageSize } from "../sizes";
import type { PageContent } from "./runtime";
import "../book.css";

// A page template drawn by the book's own runtime (T-172, ADR 0005 Q3): the template's `page` becomes a one-page layout document of a sample book,
// so the preview is exactly what applying the template would print, with no stored images to keep in step. Read-only: no handles, no text field.

const isSection = (k: string): k is SectionId => (SECTION_IDS as readonly string[]).includes(k);

/** A small book whose names and notes are placeholders, so every page kind has something to draw. */
function sampleBook(
  templateId: string,
  size: PageSize,
  language: "en" | "vi",
  layout: BookData["layout"],
): BookData {
  const vi = language === "vi";
  const notes = [1, 2].map((i) => ({
    id: `01J9Z3K6V8Q4M7N2P5R8N0000${i}`,
    answers: {
      name: vi ? `Bạn ${i}` : `Friend ${i}`,
      message: vi ? "Giữ liên lạc nhé" : "Stay in touch",
    },
    signature: null,
    photo_ids: [],
  }));
  return {
    layout,
    design: null,
    yearbook: {
      title: vi ? "Lớp 12A1" : "Class 12A1",
      school_name: vi ? "Trường của chúng mình" : "Our school",
      class_name: "12A1",
      graduation_year: 2026,
      motto: vi ? "Học, học nữa" : "Learn, and learn more",
      language,
      page_size: size,
      template_id: templateId,
      cover_media_id: null,
    },
    profile: {
      full_name: vi ? "Tên của bạn" : "Your name",
      nickname: "",
      quote: vi ? "Một câu bạn thích" : "A line you like",
      hobbies: "",
      future_plans: "",
      photo_media_id: null,
    },
    notes,
    note_count: notes.length,
    approved_total: notes.length,
    truncated: false,
  };
}

const noop = () => {};

/**
 * Draws `page` (the value of `pages.<kind>`) of a book template at `width` screen pixels. `kind` is a page kind of the template (cover, profile, notes,
 * back); anything else draws nothing. The preview never waits for photos (it has none) and cannot be clicked or focused.
 */
export function TemplatePreview({
  templateId,
  kind,
  page,
  width,
  size = "A5",
  language = "vi",
  label,
}: {
  templateId: string;
  kind: string;
  page: PageContent;
  width: number;
  size?: PageSize;
  language?: "en" | "vi";
  /** The accessible name of the picture (the template's name). */
  label: string;
}) {
  const data = useMemo(
    () => sampleBook(templateId, size, language, { v: 1, pages: { [kind]: page } }),
    [templateId, size, language, kind, page],
  );
  const t = useMemo(() => i18n.getFixedT(language), [language]);
  const px = pagePx(size);
  const scale = width / px.w;
  if (!isSection(kind)) return null;
  const only: OnePage = kind === "notes" ? { section: kind, index: 0 } : { section: kind };
  const style = {
    "--pw": `${SIZES[size].w}mm`,
    "--ph": `${SIZES[size].h}mm`,
    "--s": scale,
    width,
    height: px.h * scale,
    pointerEvents: "none",
  } as CSSProperties;
  return (
    <div className="tp-preview" role="img" aria-label={label} style={style}>
      <div inert aria-hidden="true">
        <BookPages data={data} size={size} t={t} eager={false} track={noop} only={only} />
      </div>
    </div>
  );
}
