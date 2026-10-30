// The fixture book of the print check: invented people, Vietnamese names, long notes, emoji, 30 synthetic photos (fixtures/).
// Shaped like the answer of GET /api/book-data/get (api/openapi.yaml); the check serves it with Playwright routes, so no API or database is needed.
const ulid = (kind, n) => `01J9Z3K6V8Q4M7N2P5${kind}${String(n).padStart(6, "0")}`.slice(0, 26).padEnd(26, "0");
export const photoId = (n) => ulid("P", n);
export const BOOK_ID = "01J9Z3K6V8Q4M7N2P5R8T0AAAA";

const NAMES = ["Nguyễn Thị Hồng Ánh", "Đặng Ngọc Bích Trâm", "Huỳnh Ngọc Diệp", "Trần Quốc Việt", "Lê Hoàng Phương Uyên", "Phạm Thị Thùy Dương"];
const MESSAGES = [
  "Chuyến dã ngoại ở Đà Lạt là kỷ niệm đẹp nhất của mình 😄 Cảm ơn bạn vì đã luôn ở bên cạnh.",
  "Chúc bạn thi đỗ trường mơ ước! Đừng quên nhóm mình nhé 🎓",
  "Mình nhớ những buổi trưa trốn tiết ra căn tin, cười không ngớt. Giữ liên lạc nhé!",
];

/** Exactly 2000 characters (the form's limit), Vietnamese with emoji. */
const LONG_2000 = [..."Mình nhớ những buổi trưa ở căn tin 😄 ".repeat(80)].slice(0, 2000).join("");
/** Messages that stay cut off at the smallest font (cutOff in makeBook): 400 short lines, and 2000 characters in one paragraph. */
const CUT_OFF = ["Dòng\n".repeat(400).slice(0, 2000), [..."Một đoạn văn rất dài không xuống dòng, ".repeat(60)].slice(0, 2000).join("")];
const WISH = ["Chúc bạn luôn mạnh khỏe, vui vẻ và thành công trên mọi nẻo đường.", "Mong bạn tìm được ngành học mình yêu thích 🌟", "Cầu mong mọi điều tốt đẹp nhất đến với bạn."];

/** The notes of the memory-book template: 21 notes make a 24-page book. Every field of the form, 21 + 7 = 28 photos (7 notes have two), plus cover and profile = 30. */
function memoryNotes(notes, longNote, cutOff) {
  let photo = 3;
  return Array.from({ length: notes }, (_, i) => ({
    id: ulid("N", i + 1),
    answers: {
      name: NAMES[i % NAMES.length],
      nickname: ["Ánh", "Trâm", "Diệp", "Việt", "Uyên", "Dương"][i % 6],
      relationship: "Bạn cùng lớp",
      how_we_met: "Ngồi cạnh nhau trong buổi học đầu tiên của lớp 10.",
      first_impression: "Một cô bạn nói rất nhiều và cười rất to 😄",
      best_memory: "Chuyến dã ngoại ở Đà Lạt, cả lớp ngồi quanh lửa trại hát vang đến khuya.",
      wish: WISH[i % WISH.length],
      message: cutOff && i < 2 ? CUT_OFF[i] : i === 3 ? MESSAGES[0].repeat(14) : i === longNote ? LONG_2000 : MESSAGES[i % MESSAGES.length],
    },
    photo_ids: i < 21 ? Array.from({ length: i % 3 === 0 ? 2 : 1 }, () => photoId(photo++)) : [],
  }));
}

/** Signatures for the fixture (T-097): a maximal one (12 strokes, as near the 3072-byte cap as the format allows, touching all four edges of the drawing space), a tiny one (the 5 % minimum extent) and a normal one. */
export const SIGNATURES = {
  maximal: (() => {
    const bytes = (s) => JSON.stringify({ v: 1, w: 1000, h: 400, s }).length;
    let best;
    for (let n = 4; n < 80; n++) {
      const s = Array.from({ length: 12 }, (_, k) =>
        Array.from({ length: n * 2 }, (_, i) => (i % 2 ? Math.round(200 + 200 * Math.sin((i * 0.45 + k) % 6.3) * (k % 2 ? -1 : 1)) : Math.round((i / 2 / (n - 1)) * 1000))),
      );
      s[0][0] = 0; s[0][1] = 0; s[1][s[1].length - 2] = 1000; s[1][s[1].length - 1] = 400; // the corners
      if (bytes(s) > 3072) break;
      best = s;
    }
    return { v: 1, w: 1000, h: 400, s: best };
  })(),
  tiny: { v: 1, w: 1000, h: 400, s: [[480, 200, 540, 200]] },
  normal: { v: 1, w: 1000, h: 400, s: [[120, 260, 300, 120, 480, 280], [500, 260, 880, 240]] },
};

/** `notes` approved notes; the first 28 carry one photo each, the cover and the profile one more: 30 distinct photos. */
export function makeBook({ notes = 84, size = "A5", truncatedOf = null, template = null, longNote = -1, stress = false, cutOff = false, profileMax = false, portrait = true, signatures = false, design = null } = {}) {
  const list = template === "memory-book" ? memoryNotes(notes, longNote, cutOff) : Array.from({ length: notes }, (_, i) => ({
    id: ulid("N", i + 1),
    answers: {
      name: NAMES[i % NAMES.length],
      relationship: "Bạn cùng lớp",
      message: cutOff && i < 2 ? CUT_OFF[i] : i === 3 ? MESSAGES[0].repeat(14) : i === longNote ? LONG_2000 : MESSAGES[i % MESSAGES.length], // note 4: a long one that must shrink to fit
      ...(i % 5 === 0 ? { best_memory: "Buổi diễn văn nghệ cuối năm, cả lớp hát vang 🎤" } : {}),
    },
    photo_ids: i < 28 ? [photoId(i + 3)] : [],
  }));
  const book = {
    yearbook: {
      title: "Lớp 12A1 - Trường THPT Chu Văn An",
      school_name: "THPT Chu Văn An",
      class_name: "12A1",
      graduation_year: 2026,
      motto: "Học, học nữa, học mãi",
      language: "vi",
      page_size: size,
      template_id: template,
      cover_media_id: photoId(1),
    },
    profile: {
      full_name: "Nguyễn Thị Hồng",
      nickname: "Hồng",
      quote: "Hãy sống như thể hôm nay là ngày cuối cùng.",
      hobbies: "Đọc sách, vẽ, đi bộ đường dài",
      future_plans: "Học thiết kế đồ họa ở Thành phố Hồ Chí Minh",
      photo_media_id: photoId(2),
    },
    notes: list,
    note_count: list.length,
    approved_total: truncatedOf ?? list.length,
    truncated: truncatedOf !== null,
  };
  if (stress) {
    // Long Vietnamese and emoji everywhere, at the form's limits for names (60, 40, 60) and long but realistic answers (200 characters, message 600).
    const cut = (s, n) => [...s.repeat(Math.ceil(n / [...s].length))].slice(0, n).join("");
    const name = "Nguyễn Thị Thanh Thủy Phương Uyên Hoàng Ngọc Diệp Anh 😄 ";
    const prose = "Chúc bạn thật nhiều sức khỏe, thành công và luôn hạnh phúc nhé 😄🎓 ";
    for (const n of book.notes)
      n.answers = {
        name: cut(name, 60), nickname: cut(name, 40), relationship: cut(name, 60),
        how_we_met: cut(prose, 200), first_impression: cut(prose, 200), best_memory: cut(prose, 200), wish: cut(prose, 200), advice: cut(prose, 200),
        message: cut(prose, 600),
      };
    book.yearbook.title = cut(name, 80);
    book.yearbook.motto = cut(prose, 150);
    book.profile = { ...book.profile, full_name: cut(name, 60), nickname: cut(name, 50), hobbies: cut(prose, 300), future_plans: cut(prose, 300), quote: cut(prose, 500) };
  }
  if (profileMax) {
    // The validated maximum of every owner field (internal/yearbook/validate.go), each starting with a marker word the PDF check looks for.
    const fill = (marker, n) => [...`${marker} ${"Chúc bạn thật nhiều sức khỏe, thành công và hạnh phúc nhé. ".repeat(12)}`].slice(0, n).join("");
    Object.assign(book.yearbook, { title: fill("Tmark", 120), school_name: fill("Smark", 120), class_name: fill("Cmark", 120), motto: fill("Mmark", 200) });
    Object.assign(book.profile, { full_name: fill("Zname", 100), nickname: fill("Knick", 50), hobbies: fill("Hmark", 300), future_plans: fill("Pmark", 300), quote: fill("Qmark", 500) });
  }
  // signatures: note 1 maximal, 2 tiny, 3 none, 4 normal, then the cycle again (the 2000-character note is the 5th of a navy page's cycle).
  for (const [i, n] of book.notes.entries()) n.signature = signatures ? [SIGNATURES.maximal, SIGNATURES.tiny, null, SIGNATURES.normal][i % 4] : null;
  if (!portrait) book.profile.photo_media_id = null;
  if (design) book.design = design;
  return book;
}

/** The `design` of book-data/get (T-141) for a theme and a font set: by default no hidden section, no replaced text, the template's own picture choices. */
export const designOf = (theme, fonts, { order = ["cover", "profile", "notes", "back"], hidden = [], texts = {}, images = { "cover.pattern": "dots" } } = {}) => ({
  theme,
  fonts,
  sections: { order, hidden },
  texts,
  images,
});

// ---- The class book (T-130): the answer of GET /api/class-book/get for 30 students ----
export const CLASS_ID = "01J9Z3K6V8Q4M7N2P5R8T0CCCC";
/** Notes per page of each template (web/src/book/templates): the test counts the pages itself from this. */
export const NOTES_PER_PAGE = { basic: 4, "navy-classic": 4, "memory-book": 1 };
const GIVEN = ["Nguyễn Thị Hồng Ánh", "Đặng Ngọc Bích Trâm", "Huỳnh Ngọc Diệp", "Trần Quốc Việt", "Lê Hoàng Phương Uyên", "Phạm Thị Thùy Dương"];
/** The student's name; the number makes it unique, so the test can find the student's page in the PDF text. */
export const studentName = (i) => `${GIVEN[i % GIVEN.length]} ${String(i + 1).padStart(2, "0")}`;
/** Special students: one whose notes were cut at 40 (41 approved), one whose note is too long for its box, one with no page. */
export const CLASS_BOOK_CASES = { limit: 8, tooLong: 12, noPage: 29 };

/**
 * 30 students in three designs (memory-book every 5th, navy-classic every other, the built-in layout), 29 profile photos (the 30 files are reused),
 * 2 front and back class pages, one of them with newlines. 77 pages in all, under the 120 page budget.
 */
export function makeClassBook({ students = 30 } = {}) {
  let photo = 0;
  const members = Array.from({ length: students }, (_, i) => {
    const base = { member_id: ulid("M", i + 1), name: studentName(i) };
    if (i === CLASS_BOOK_CASES.noPage) return { ...base, linked: false, yearbook: null, profile: null, notes: [], note_count: 0, approved_total: 0, truncated: false, design: null };
    const template = i % 5 === 0 ? "memory-book" : i % 2 === 1 ? "navy-classic" : null;
    const count = i === CLASS_BOOK_CASES.limit ? 40 : template === "memory-book" ? 2 : 3;
    const notes = Array.from({ length: count }, (_, k) => ({
      id: ulid("N", i * 100 + k + 1),
      answers: {
        name: GIVEN[(i + k + 1) % GIVEN.length],
        relationship: "Bạn cùng lớp",
        message: i === CLASS_BOOK_CASES.tooLong && k === 0 ? CUT_OFF[0] : MESSAGES[(i + k) % MESSAGES.length],
        wish: WISH[(i + k) % WISH.length],
      },
      signature: null,
      photo_ids: k < 2 ? [photoId((photo++ % 30) + 1)] : [],
    }));
    return {
      ...base,
      linked: true,
      yearbook: { template_id: template, page_size: "A5", language: "vi" },
      profile: {
        full_name: studentName(i),
        nickname: "",
        quote: "Hãy sống như thể hôm nay là ngày cuối cùng.",
        hobbies: "Đọc sách, vẽ, đi bộ đường dài",
        future_plans: "Học thiết kế đồ họa 🎨",
        photo_media_id: photoId((i % 30) + 1),
      },
      notes,
      note_count: notes.length,
      approved_total: i === CLASS_BOOK_CASES.limit ? 41 : notes.length,
      truncated: i === CLASS_BOOK_CASES.limit,
      design: null, // T-146: the student's own design; null (the template's defaults) for the 30-student fixture
    };
  });
  const page = (n, placement, title, body) => ({ id: ulid("G", n), kind: "intro", title, body, placement, created_at: 1759276800000, updated_at: 1759276800000 });
  return {
    class: { id: CLASS_ID, name: "Lớp 12A1 Chu Văn An", school_name: "THPT Chu Văn An", graduation_year: 2026, language: "vi" },
    front_pages: [page(1, "front", "Lời chào của cả lớp", "Ba năm cùng nhau.\nCảm ơn thầy cô và các bạn 😄\n\nHẹn gặp lại!")],
    members,
    back_pages: [page(2, "back", "Thầy cô chủ nhiệm", "Cô Nguyễn Thị Lan\nThầy Trần Văn Minh")],
  };
}

/** The pages the class book must have: cover, front pages, each linked student's profile and notes pages (less a profile their design hides), back pages, back cover. */
export function classBookPages(cb) {
  const students = cb.members.reduce((n, m) => {
    if (!m.linked) return n;
    const profile = m.design?.sections.hidden.includes("profile") ? 0 : 1;
    return n + profile + Math.ceil(m.notes.length / NOTES_PER_PAGE[m.yearbook.template_id ?? "basic"]);
  }, 0);
  return 2 + cb.front_pages.length + students + cb.back_pages.length;
}

/**
 * T-146: six students with their own designs, on top of the 30-student fixture's shapes: 0 memory-book lavender/classic with the profile hidden, 1 navy-classic burgundy/modern with the
 * profile hidden, 2 built-in layout (no design block), 3 navy-classic forest/classic with a replaced title, 4 built-in layout, 5 memory-book mint/playful with the notes before the profile.
 * Student 0 and 1 have no profile page; the profile quote is on the pages of the other four.
 */
export const HIDDEN_PHOTOS = [photoId(25), photoId(26)];
export const DESIGN_TITLE = "Về mình Tmark";
export function makeDesignClassBook() {
  const cb = makeClassBook({ students: 6 });
  const designs = [
    designOf("lavender", "classic", { hidden: ["profile"] }),
    designOf("burgundy", "modern", { hidden: ["profile"] }),
    null,
    designOf("forest", "classic", { texts: { "profile.heading": DESIGN_TITLE } }),
    null,
    designOf("mint", "playful", { order: ["cover", "notes", "profile", "back"] }),
  ];
  cb.members.forEach((m, i) => (m.design = designs[i]));
  cb.members[0].profile.photo_media_id = HIDDEN_PHOTOS[0]; // a photo only a hidden profile shows: it must never be fetched
  cb.members[1].profile.photo_media_id = HIDDEN_PHOTOS[1];
  return cb;
}
