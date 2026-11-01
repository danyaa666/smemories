// The palette search (T-162): case- and diacritic-insensitive, so "nang" finds "nắng" and "dong" finds "Đồng". Every word of the query must appear
// in the label (either language) or a tag of the asset.
import type { Asset } from "../../assets";

/** Lower case, no combining marks; Vietnamese đ has no decomposition, so it is mapped by hand. */
export const fold = (s: string) =>
  s.normalize("NFD").replace(/\p{M}/gu, "").replace(/[đĐ]/g, "d").toLowerCase();

export function matchAssets(list: readonly Asset[], query: string): Asset[] {
  const words = fold(query).split(/\s+/).filter(Boolean);
  if (words.length === 0) return [...list];
  return list.filter((a) => {
    const hay = fold([a.label_vi, a.label_en, ...a.tags].join(" "));
    return words.every((w) => hay.includes(w));
  });
}
