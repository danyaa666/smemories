import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { BookData } from "../api/book";
import i18n from "../i18n";
import { BookPages } from "./BookPages";
import type { PageSize } from "./sizes";

// T-097. The print templates draw a friend's signature as inline SVG from validated integers (BookSignature). A book with no
// signature must render exactly as it did before signatures existed: the snapshots below were taken from the code of T-096.

const t = i18n.getFixedT("vi");
const SIZES: PageSize[] = ["A5", "A4", "Letter"];
const TEMPLATES = ["memory-book", "navy-classic"] as const;

const note = (i: number, signature: BookData["notes"][number]["signature"]) => ({
  id: `01J9Z3K6V8Q4M7N2P5R8N${String(i).padStart(5, "0")}`,
  answers: {
    name: `Nguyễn Thị Hồng Ánh ${i}`,
    nickname: "Ánh",
    relationship: "Bạn cùng lớp",
    how_we_met: "Ngồi cạnh nhau",
    first_impression: "Nói nhiều",
    best_memory: "Đà Lạt",
    wish: "Thi đỗ nhé",
    message: `Giữ liên lạc nhé ${i}`,
  },
  signature,
  photo_ids: [],
});

function book(template: string, size: PageSize, signature: BookData["notes"][number]["signature"]): BookData {
  const notes = Array.from({ length: 5 }, (_, i) => note(i + 1, signature));
  return {
    yearbook: {
      title: "Lớp 12A1",
      school_name: "THPT Chu Văn An",
      class_name: "12A1",
      graduation_year: 2026,
      motto: "Học, học nữa, học mãi",
      language: "vi",
      page_size: size,
      template_id: template,
      cover_media_id: null,
    },
    profile: {
      full_name: "Nguyễn Thị Hồng",
      nickname: "Hồng",
      quote: "Hãy sống",
      hobbies: "Đọc sách",
      future_plans: "Thiết kế",
      photo_media_id: null,
    },
    notes,
    note_count: notes.length,
    approved_total: notes.length,
    truncated: false,
  };
}

const html = (b: BookData, size: PageSize) =>
  renderToStaticMarkup(<BookPages data={b} size={size} t={t} eager={false} />);

describe("a book with no signature renders as before", () => {
  for (const tpl of TEMPLATES)
    for (const size of SIZES)
      it(`${tpl} ${size}`, () => {
        expect(html(book(tpl, size, null), size)).toMatchSnapshot();
      });
});
