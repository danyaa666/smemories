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

/** `notes` approved notes; the first 28 carry one photo each, the cover and the profile one more: 30 distinct photos. */
export function makeBook({ notes = 84, size = "A5", truncatedOf = null } = {}) {
  const list = Array.from({ length: notes }, (_, i) => ({
    id: ulid("N", i + 1),
    answers: {
      name: NAMES[i % NAMES.length],
      relationship: "Bạn cùng lớp",
      message: i === 3 ? MESSAGES[0].repeat(14) : MESSAGES[i % MESSAGES.length], // note 4: a long one that must shrink to fit
      ...(i % 5 === 0 ? { best_memory: "Buổi diễn văn nghệ cuối năm, cả lớp hát vang 🎤" } : {}),
    },
    photo_ids: i < 28 ? [photoId(i + 3)] : [],
  }));
  return {
    yearbook: {
      title: "Lớp 12A1 - Trường THPT Chu Văn An",
      school_name: "THPT Chu Văn An",
      class_name: "12A1",
      graduation_year: 2026,
      motto: "Học, học nữa, học mãi",
      language: "vi",
      page_size: size,
      template_id: null,
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
}
