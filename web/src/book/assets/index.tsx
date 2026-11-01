import { createElement, useEffect, useState, type CSSProperties, type ReactNode } from "react";
import registry from "./registry.json";

// The sticker, shape and frame pack (epic E09, T-151, docs/assets.md). The registry is the list the server also embeds (internal/assets); the SVG files
// are ours, checked by scripts/check-assets.sh, and are turned into React elements here, never injected as HTML. Nothing in the book imports this
// module yet, so the print bundle does not grow; the SVG text of a kind is a separate chunk, fetched when the first asset of that kind is drawn.

export type AssetKind = "sticker" | "shape" | "frame";

export type Asset = {
  id: string;
  kind: AssetKind;
  file: string;
  label_en: string;
  label_vi: string;
  tags: string[];
  /** True when the SVG paints its one tint colour with `currentColor` (CSS `color` or the `tint` prop). */
  tintable: boolean;
  /** The viewBox is `0 0 width height`. */
  width: number;
  height: number;
  licence: "CC0" | "OFL" | "authored";
  licence_file: string;
  source_url: string;
  author: string;
  date_checked: string;
};

export const assets: readonly Asset[] = registry.assets as Asset[];

const byId = new Map(assets.map((a) => [a.id, a]));

export function getAsset(id: string): Asset | undefined {
  return byId.get(id);
}

type Kind = Record<string, string>;
const kinds: Record<AssetKind, () => Promise<{ default: Kind }>> = {
  sticker: () => import("./kind-sticker"),
  shape: () => import("./kind-shape"),
  frame: () => import("./kind-frame"),
};

/** The SVG text of an asset (loads its kind's chunk on first use); undefined for an id that is not in the registry. */
export async function loadSvg(id: string): Promise<string | undefined> {
  const a = byId.get(id);
  if (!a) return undefined;
  return (await kinds[a.kind]()).default[`./${a.file}`];
}

const DRAWN = new Set(["g", "path", "circle", "ellipse", "rect", "line", "polyline", "polygon"]);

function attrName(n: string): string {
  return n.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
}

function convert(el: Element, key: number): ReactNode {
  const props: Record<string, string> = { key: String(key) };
  for (const at of Array.from(el.attributes)) props[attrName(at.name)] = at.value;
  const kids = Array.from(el.children)
    .filter((c) => DRAWN.has(c.localName))
    .map((c, i) => convert(c, i));
  return createElement(el.localName, props, ...kids);
}

/** Turns the text of one pack SVG into the React elements inside its root; null when it is not a well-formed SVG. Unknown elements are dropped. */
export function svgChildren(text: string): ReactNode[] | null {
  const doc = new DOMParser().parseFromString(text, "image/svg+xml");
  const root = doc.documentElement;
  if (root.localName !== "svg" || doc.getElementsByTagName("parsererror").length > 0) return null;
  return Array.from(root.children)
    .filter((c) => DRAWN.has(c.localName))
    .map((c, i) => convert(c, i));
}

export type AssetSvgProps = {
  id: string;
  /** Any CSS colour; only used by a tintable asset (it paints with `currentColor`). */
  tint?: string;
  /** The width: a number is pixels, a string is any CSS length. The height follows the asset's aspect ratio. */
  size?: number | string;
  /** The accessible name; without it the picture is decorative (aria-hidden). */
  label?: string;
  className?: string;
  /** Fill the parent's box (width and height 100%) and fit the picture in it, instead of taking the height from the asset's aspect ratio. */
  fill?: boolean;
};

/** Draws one registered asset inline. Renders nothing for an unknown id or while its kind is loading. */
export function AssetSvg({ id, tint, size = 64, label, className, fill }: AssetSvgProps) {
  const asset = byId.get(id);
  const [kids, setKids] = useState<{ id: string; nodes: ReactNode[] } | null>(null);
  useEffect(() => {
    let live = true;
    void loadSvg(id).then((text) => {
      const nodes = text === undefined ? null : svgChildren(text);
      if (live && nodes) setKids({ id, nodes });
    });
    return () => {
      live = false;
    };
  }, [id]);
  if (!asset || kids?.id !== id) return null;
  const style: CSSProperties = fill
    ? { width: "100%", height: "100%", display: "block" }
    : { width: size, height: "auto", aspectRatio: `${asset.width} / ${asset.height}` };
  if (asset.tintable && tint) style.color = tint;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox={`0 0 ${asset.width} ${asset.height}`}
      style={style}
      className={className}
      data-asset={id}
      {...(label ? { role: "img", "aria-label": label } : { "aria-hidden": true })}
    >
      {kids.nodes}
    </svg>
  );
}
