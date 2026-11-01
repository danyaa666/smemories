import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  type CSSProperties,
  type ReactNode,
} from "react";
import type { BookData } from "../../api/book";
import { AssetSvg, getAsset, loadSvg } from "../assets";
import { Photo, useBookEnv } from "../env";
import { FitText } from "../fit";
import { DESIGN_W, designHeight } from "../sizes";
import { boxStyle, pageBox, type Box } from "./geometry";

// The layout layer of the book (epic E09 stage 2, T-154, docs/templates.md "Layout layer"): what the student put on the pages, drawn by the same
// React components for the screen, the print and the class book. The data is `layout` of book-data/get, already validated by the server and re-validated
// on every read; this file still treats it as untrusted: it draws text as React text only and skips what it cannot find.

export type LayoutDoc = NonNullable<BookData["layout"]>;
export type PageContent = LayoutDoc["pages"][string];
type Item = NonNullable<PageContent["items"]>[number];

/** A template element's own place on the A5 reference page (design px, 816 x 1158): where an override that omits a field starts from. */
export type Natural = {
  left: number;
  top: number;
  w: number;
  h: number;
  r?: number;
  /** True when the element sits in the page's flow: moving or hiding it leaves a blank of its size, so nothing else moves. */
  flow?: boolean;
};

/** What a template tells the layout layer: the CSS behind each colour token and font id of its manifest, and its editable elements. */
export type TemplateLayout = {
  colors: Record<string, string>;
  fonts: Record<string, string>;
  natural: Record<string, Natural>;
};

type Ctx = { doc: LayoutDoc; spec: TemplateLayout };
const LayoutContext = createContext<Ctx | null>(null);

/** Gives the pages below it the layout of one book. Without a document, or for a template with no layout block, every page draws as it always did. */
export function LayoutProvider({
  layout,
  spec,
  children,
}: {
  layout: LayoutDoc | null | undefined;
  spec: TemplateLayout | undefined;
  children: ReactNode;
}) {
  const value = useMemo(() => (layout && spec ? { doc: layout, spec } : null), [layout, spec]);
  return <LayoutContext.Provider value={value}>{children}</LayoutContext.Provider>;
}

const useLayout = () => useContext(LayoutContext);
const A5_H = designHeight("A5");

/** The page kind an element id belongs to: the part before the first dot ("cover.photo" is on the cover). */
export const kindOf = (id: string) => id.split(".")[0] ?? "";

/** The natural box of an element in stored units. */
export function naturalBox(n: Natural): Box {
  return {
    x: (n.left * 1000) / DESIGN_W,
    w: (n.w * 1000) / DESIGN_W,
    h: (n.h * 1000) / DESIGN_W,
    cy: ((n.top + n.h / 2) * 1000) / A5_H,
    r: n.r,
  };
}

/**
 * Marks a template element the student may move (`data-layout-id`, pinned to the manifest's `editable` ids by a test). With no layout in the book it adds
 * nothing at all. With a layout, an element it does not move gets a wrapper that has no box of its own (`display: contents`) so the editor can find it. A moved
 * element is drawn in a box of its own, absolutely placed with the geometry rule and rotated; it is the same element with the same data, only moved (it fills
 * the box, see `.mb-ov`). `hidden` leaves it out. An element in the flow leaves its blank behind.
 */
export function Editable({ layoutId: id, children }: { layoutId: string; children: ReactNode }) {
  const ctx = useLayout();
  const { size } = useBookEnv();
  const ov = ctx?.doc.pages[kindOf(id)]?.overrides?.[id];
  const nat = ctx?.spec.natural[id];
  if (!ctx) return <>{children}</>; // no layout: the page is exactly what it always was
  const moves = ov && (ov.hidden || [ov.x, ov.w, ov.h, ov.cy, ov.r].some((v) => v !== undefined));
  if (!moves || !nat)
    return (
      <div data-layout-id={id} style={{ display: "contents" }}>
        {children}
      </div>
    );
  const base = naturalBox(nat);
  const box: Box = {
    x: ov.x ?? base.x,
    w: ov.w ?? base.w,
    h: ov.h ?? base.h,
    cy: ov.cy ?? base.cy,
    r: ov.r ?? base.r,
  };
  return (
    <>
      {nat.flow && <div aria-hidden="true" style={{ flex: "none", width: nat.w, height: nat.h }} />}
      {!ov.hidden && (
        <div data-layout-id={id} className="mb-ov" style={boxStyle(box, pageBox(size))}>
          {children}
        </div>
      )}
    </>
  );
}

/** The page background of a kind: a colour token (CSS value), a registry asset or the owner's design image (`display` variant, 3000 px). */
export function useLayoutBg(kind: string): { color?: string; node?: ReactNode } {
  const ctx = useLayout();
  const bg = ctx?.doc.pages[kind]?.bg;
  if (!ctx || !bg) return {};
  if (bg.color !== undefined)
    return Object.hasOwn(ctx.spec.colors, bg.color) ? { color: ctx.spec.colors[bg.color] } : {};
  if (bg.media !== undefined)
    return {
      node: (
        <Photo
          id={bg.media}
          variant="display"
          quiet
          style={{ position: "absolute", inset: 0, width: "100%", height: "100%" }}
        />
      ),
    };
  if (bg.asset !== undefined && getAsset(bg.asset))
    return {
      node: (
        <div style={{ position: "absolute", inset: 0 }}>
          <AssetSvg id={bg.asset} size="100%" fill />
        </div>
      ),
    };
  return {};
}

/** True when the kind has a background of its own: the template then leaves its paper colour and picture out. */
export const hasLayoutBg = (bg: ReturnType<typeof useLayoutBg>) => !!(bg.color || bg.node);

function TextItem({ item, kind, spec }: { item: Item; kind: string; spec: TemplateLayout }) {
  const font = item.font !== undefined && Object.hasOwn(spec.fonts, item.font) ? item.font : "body";
  const color =
    item.color !== undefined && Object.hasOwn(spec.colors, item.color)
      ? spec.colors[item.color]
      : spec.colors["ink"];
  const style: CSSProperties = {
    width: "100%",
    height: "100%",
    fontSize: item.size ?? 24,
    fontFamily: spec.fonts[font],
    fontWeight: item.bold ? 700 : 400,
    color,
    textAlign: item.align ?? "left",
    lineHeight: 1.25,
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
  };
  return (
    <FitText fitKey={`layout:${kind}:${item.id}`} style={style}>
      {item.text ?? ""}
    </FitText>
  );
}

/** Counts an asset's SVG chunk as loading, so the print does not signal ready before it is drawn. */
function AssetItem({ item, spec }: { item: Item; spec: TemplateLayout }) {
  const { track } = useBookEnv();
  const id = item.asset ?? "";
  useEffect(() => {
    let open = true;
    track(1);
    void loadSvg(id).finally(() => {
      if (open) track(-1);
      open = false;
    });
    return () => {
      if (open) track(-1);
      open = false;
    };
  }, [id, track]);
  const tint =
    item.tint !== undefined && Object.hasOwn(spec.colors, item.tint)
      ? spec.colors[item.tint]
      : undefined;
  return <AssetSvg id={id} size="100%" fill tint={tint} />;
}

function ItemView({ item, kind, spec }: { item: Item; kind: string; spec: TemplateLayout }) {
  const { size } = useBookEnv();
  let body: ReactNode = null;
  if (item.t === "text") body = <TextItem item={item} kind={kind} spec={spec} />;
  else if (item.t === "asset" && item.asset && getAsset(item.asset))
    body = <AssetItem item={item} spec={spec} />;
  else if (item.t === "image" && item.media)
    body = (
      <Photo
        id={item.media}
        variant="print"
        quiet
        style={{
          width: "100%",
          height: "100%",
          objectFit: item.fit ?? "cover",
          borderRadius: item.radius ?? 0,
        }}
      />
    );
  if (body === null) return null; // an unknown kind, or an asset the registry no longer has: skipped without an error
  return (
    <div data-layout-item={item.id} style={boxStyle(item, pageBox(size))}>
      {body}
    </div>
  );
}

/**
 * The items of one page kind, in array order (later on top), over the page. It is the `layer` of `Sheet`; it carries the page's theme and font set so
 * that colour tokens and font ids resolve like the template's own text. The notes kind is drawn on every notes page.
 */
export function LayoutLayer({
  kind,
  theme,
  fonts,
}: {
  kind: string;
  theme?: string;
  fonts?: string;
}) {
  const ctx = useLayout();
  const items = ctx?.doc.pages[kind]?.items;
  if (!ctx || !items?.length) return null;
  return (
    <div className="mb-layer" data-theme={theme || undefined} data-fonts={fonts || undefined}>
      {items.map((it) => (
        <ItemView key={it.id} item={it} kind={kind} spec={ctx.spec} />
      ))}
    </div>
  );
}
