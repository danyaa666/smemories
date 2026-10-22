import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { BookData } from "../api/book";
import { canonical, parseSignature, signaturePath, type Stroke } from "../signature";
import i18n from "../i18n";
import { BookPages } from "./BookPages";
import { BookSignature, hasSignature } from "./BookSignature";
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

function book(
  template: string,
  size: PageSize,
  signature: BookData["notes"][number]["signature"],
): BookData {
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

const SIG = {
  v: 1 as const,
  w: 1000 as const,
  h: 400 as const,
  s: [
    [120, 260, 300, 120, 480, 280],
    [500, 260, 880, 240],
  ],
};
/** The largest signature the API stores: 12 strokes of three-digit points, as close to 3072 canonical bytes as it gets. */
function maximal(): BookData["notes"][number]["signature"] {
  let strokes: Stroke[] = [];
  for (let n = 30; n < 60; n++) {
    const next: Stroke[] = Array.from({ length: 12 }, (_, k) =>
      Array.from({ length: n * 2 }, (_, i) => 100 + ((i * 37 + k * 11) % (i % 2 ? 290 : 880))),
    );
    if (canonical(next).length > 3072) break;
    strokes = next;
  }
  return { v: 1, w: 1000, h: 400, s: strokes } as BookData["notes"][number]["signature"];
}

describe("BookSignature", () => {
  const draw = (value: unknown) =>
    renderToStaticMarkup(<BookSignature value={value} label="Chữ ký" className="x" />);
  it("draws strokes only: no fill, round caps and joins, the template's ink, a name", () => {
    const out = draw(SIG);
    expect(out).toContain('d="M120 260L300 120L480 280M500 260L880 240"');
    expect(out).toContain('fill="none"');
    expect(out).toContain('stroke="currentColor"');
    expect(out).toContain('stroke-linecap="round"');
    expect(out).toContain('stroke-linejoin="round"');
    expect(out).toContain('aria-label="Chữ ký"');
    expect(out).not.toContain("vector-effect");
    expect(out).not.toMatch(/<(image|foreignObject|script|style)/i);
  });
  it.each([
    ["null", null],
    ["undefined", undefined],
    ["a string", "<svg onload=alert(1)>"],
    ["a float", { ...SIG, s: [[1.5, 2, 3, 4]] }],
    ["an out-of-range point", { ...SIG, s: [[0, 0, 1001, 4]] }],
    ["a string coordinate", { ...SIG, s: [["10", 2, 300, 4]] }],
    ["an extra key", { ...SIG, x: 1 }],
    ["no stroke", { ...SIG, s: [] }],
  ])("renders nothing for %s", (_, value) => {
    expect(draw(value)).toBe("");
    expect(hasSignature(value)).toBe(false);
  });
  it("the maximal signature is valid, within the byte cap, and drawn", () => {
    const m = maximal();
    expect(parseSignature(m)).not.toBeNull();
    expect(canonical((m as { s: Stroke[] }).s).length).toBeGreaterThan(2900);
    expect(draw(m)).toContain(`d="${signaturePath(m)}"`);
  });
});

const count = (html: string, re: RegExp) => (html.match(re) ?? []).length;

describe("signatures in the templates", () => {
  for (const tpl of TEMPLATES)
    for (const size of SIZES) {
      it(`${tpl} ${size}: each signed note shows its signature, an unsigned one does not`, () => {
        const b = book(tpl, size, SIG);
        b.notes[1]!.signature = null;
        const out = html(b, size);
        expect(count(out, /<svg class="bk-sig /g)).toBe(4);
        expect(count(out, /d="M120 260L300 120L480 280M500 260L880 240"/g)).toBe(4);
        expect(out).toContain('aria-label="Chữ ký của Nguyễn Thị Hồng Ánh 1"');
        expect(out).not.toContain('Nguyễn Thị Hồng Ánh 2"');
      });
      it(`${tpl} ${size}: the maximal signature is drawn`, () => {
        const m = maximal();
        const out = html(book(tpl, size, m), size);
        expect(count(out, /<svg class="bk-sig /g)).toBe(5);
        expect(out).toContain(`d="${signaturePath(m)}"`);
      });
      it(`${tpl} ${size}: an invalid stored value draws nothing and changes nothing`, () => {
        const bad = {
          v: 1,
          w: 1000,
          h: 400,
          s: [[1.5, 2, 3, 4]],
        } as unknown as BookData["notes"][number]["signature"];
        expect(html(book(tpl, size, bad), size)).toBe(html(book(tpl, size, null), size));
      });
    }
  it("memory-book puts the signature inside the message card, after the text; navy-classic at the end of the name line", () => {
    const m = html(book("memory-book", "A5", SIG), "A5");
    expect(m).toMatch(
      /mb-lines[^>]*>Giữ liên lạc nhé 1<\/div><\/div><div class="mb-sig-foot"><svg class="bk-sig mb-sig"/,
    );
    const n = html(book("navy-classic", "A5", SIG), "A5");
    expect(n).toMatch(/nc-signed-name[\s\S]*?<\/div><svg class="bk-sig nc-sig"/);
  });
});
