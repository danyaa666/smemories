// The REAL Memory Book cover (web/src/book) with a layer portalled into its sheet.
import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import i18n from "@web/i18n";
import "@web/book/book.css";
import { BookPages } from "@web/book/BookPages";
import { pageCss, SIZES } from "@web/book/sizes";
import { H, W } from "./units.mjs";

export const t = i18n.getFixedT("vi");
const noop = () => {};

export function bookData(size, page = "cover") {
  const order = ["cover", "profile", "notes", "back"];
  return {
    design: { sections: { order, hidden: order.filter((s) => s !== page) }, theme: "lavender", fonts: "playful", texts: {}, images: {} },
    yearbook: { template_id: "memory-book", page_size: size, language: "vi", title: "Lớp 12A1", graduation_year: 2026, class_name: "12A1", school_name: "THPT Lê Quý Đôn", cover_media_id: null },
    profile: { full_name: "Nguyễn Minh Anh", photo_media_id: null },
    notes: [],
  };
}

/** One sheet of the real book; `layer(host)` is rendered inside .bk-inner (design px coordinates, origin at the page corner). */
export function Sheet({ size, scale = 1, page = "cover", print = false, children }) {
  const [host, setHost] = useState(null);
  const data = useMemo(() => bookData(size, page), [size, page]);
  const stage = useMemo(() => ({ "--pw": `${SIZES[size].w}mm`, "--ph": `${SIZES[size].h}mm`, "--s": print ? 1 : scale }), [size, scale, print]);
  useEffect(() => {
    const el = document.querySelector(`[data-sheet="${size}-${scale}-${print}"] .bk-inner`);
    setHost(el);
  }, [size, scale, print]);
  return (
    <div data-sheet={`${size}-${scale}-${print}`} className="bk-stage" style={{ ...stage, padding: 0, background: "none", width: "auto" }}>
      {print && <style>{pageCss(size)}</style>}
      <BookPages data={data} size={size} t={t} eager={false} track={noop} />
      {host && createPortal(children, host)}
    </div>
  );
}
export { H, W };
