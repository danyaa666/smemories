// What a new item looks like when it comes from the palette (T-162): size, aspect ratio, place. Pure; the id comes from the caller.
import type { Asset } from "../../assets";
import type { PageBox } from "../../layout/geometry";
import { MAX_BOX, MIN_BOX, type LayoutItem } from "../document";

const clamp = (n: number) => Math.min(MAX_BOX, Math.max(MIN_BOX, Math.round(n)));

/** A box of width `w` (thousandths of the page width) and the given width over height, centred on the page. */
function centred(w: number, aspect: number) {
  const bw = clamp(w);
  return { x: Math.round((1000 - bw) / 2), w: bw, h: clamp(bw / aspect), cy: 500, r: 0 };
}

/** A sticker, shape or frame: 25 % of the page width, the SVG's own aspect ratio, centred. */
export const assetItem = (asset: Asset, id: string): LayoutItem => ({
  id,
  t: "asset",
  asset: asset.id,
  ...centred(250, asset.width / asset.height),
});

/** A design image: its long side is 50 % of the page width, its aspect ratio kept, `contain` so nothing is cut. */
export const imageItem = (
  media: { id: string; width: number; height: number },
  id: string,
): LayoutItem => {
  const aspect = media.width / media.height;
  return {
    id,
    t: "image",
    media: media.id,
    fit: "contain",
    ...centred(aspect >= 1 ? 500 : 500 * aspect, aspect),
  };
};

/** The shift of a copy: 3 % of the page width right and down (the vertical part in thousandths of the page height). */
export const copyShift = (pb: PageBox) => ({ dx: 30, dcy: Math.round((30 * pb.w) / pb.h) });
