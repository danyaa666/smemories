import {
  createContext,
  useContext,
  useLayoutEffect,
  useRef,
  type CSSProperties,
  type ReactNode,
} from "react";

// Physical page sizes in millimetres. The canvas designs are 816 x 1056 CSS px (US Letter at 96 dpi).
export const SIZES = {
  a5: { w: 148, h: 210 },
  a4: { w: 210, h: 297 },
  letter: { w: 215.9, h: 279.4 },
} as const;
export type SizeKey = keyof typeof SIZES;

/** The design is laid out in a box 816 px wide; its height follows the page ratio so that bottom-anchored items stay at the bottom. */
export const DESIGN_W = 816;
export const designHeight = (s: SizeKey) => Math.round((DESIGN_W * SIZES[s].h) / SIZES[s].w);

const SizeCtx = createContext<SizeKey>("letter");
export const SizeProvider = SizeCtx.Provider;
export const useSize = () => useContext(SizeCtx);

/**
 * How the 816 px design is fitted to the paper: CSS zoom (default, real layout scaling) or transform: scale().
 * Measured in ADR 0003: with transform Chromium rasterises every sheet into the PDF (about 2.5x the file size), with zoom it stays vector.
 */
const SCALE_MODE =
  new URLSearchParams(window.location.search).get("scale") === "transform" ? "transform" : "zoom";

/** One printed page: a fixed box in mm that breaks after itself, with the design scaled into it. */
export function Sheet({
  children,
  label,
  style,
  className,
}: {
  children: ReactNode;
  label: string;
  style?: CSSProperties;
  className?: string;
}) {
  const s = useSize();
  const { w, h } = SIZES[s];
  const k = (w * 96) / 25.4 / DESIGN_W; // CSS px of the design -> CSS px on paper
  return (
    <section className="sp-sheet" data-page={label} style={{ width: `${w}mm`, height: `${h}mm` }}>
      <div
        className={`sp-inner ${className ?? ""}`}
        style={{
          width: DESIGN_W,
          height: designHeight(s),
          ...(SCALE_MODE === "transform" ? { transform: `scale(${k})` } : { zoom: k }),
          ...style,
        }}
      >
        {children}
      </div>
    </section>
  );
}

/**
 * Text that shrinks to fit its box (JavaScript, not CSS): the box has a fixed size; if the content overflows,
 * the font size (and a unitless line height with it) is reduced in 0.5 px steps down to minScale. After that the
 * text is clipped and data-fit="overflow" is set so that the editor could warn the owner.
 * Re-run once web fonts have loaded, because fallback fonts have other metrics.
 */
export function FitText({
  children,
  minScale = 0.5,
  className,
  style,
}: {
  children: ReactNode;
  minScale?: number;
  className?: string;
  style?: CSSProperties;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const baseSize = style?.fontSize;
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    let live = true;
    const inline = typeof baseSize === "number" ? `${baseSize}px` : (baseSize ?? ""); // the template's own size is the starting point
    const fit = () => {
      el.style.fontSize = inline;
      el.dataset.fit = "ok";
      const base = parseFloat(getComputedStyle(el).fontSize);
      const over = () =>
        el.scrollHeight > el.clientHeight + 1 || el.scrollWidth > el.clientWidth + 1;
      for (let s = base; over() && s > base * minScale;) {
        s -= 0.5;
        el.style.fontSize = `${s}px`;
        el.dataset.fit = "shrunk";
      }
      if (over()) el.dataset.fit = "overflow";
    };
    fit();
    void document.fonts.ready.then(() => live && fit());
    return () => {
      live = false;
    };
  }, [children, minScale, baseSize]);
  return (
    <div ref={ref} className={`sp-fit ${className ?? ""}`} style={style}>
      {children}
    </div>
  );
}

export function Photo({
  src,
  className,
  style,
  alt = "",
}: {
  src: string | undefined;
  className?: string;
  style?: CSSProperties;
  alt?: string;
}) {
  // loading="eager" + decoding="sync": the print preview must not start before the bitmap is there.
  return src ? (
    <img
      className={`sp-photo ${className ?? ""}`}
      style={style}
      src={src}
      alt={alt}
      loading="eager"
      decoding="sync"
    />
  ) : (
    <div className={`sp-ph ${className ?? ""}`} style={style} />
  );
}

const PATHS = {
  heart:
    "M12 20.5C5 15.5 2.5 12 2.5 8.6 2.5 5.9 4.6 4 7 4c2.1 0 3.9 1.3 5 3 1.1-1.7 2.9-3 5-3 2.4 0 4.5 1.9 4.5 4.6 0 3.4-2.5 6.9-9.5 11.9z",
  sparkle: "M12 3c.8 5 4 8.2 9 9-5 .8-8.2 4-9 9-.8-5-4-8.2-9-9 5-.8 8.2-4 9-9z",
  star: "M12 2.5l2.9 6.1 6.6.8-4.9 4.5 1.3 6.6L12 17.2l-5.9 3.3 1.3-6.6-4.9-4.5 6.6-.8z",
};
export function Icon({
  kind,
  size,
  stroke,
  fill = "none",
  style,
}: {
  kind: keyof typeof PATHS;
  size: number;
  stroke: string;
  fill?: string;
  style?: CSSProperties;
}) {
  return (
    <svg
      style={style}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={fill}
      stroke={stroke}
      strokeWidth={1.8}
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={PATHS[kind]} />
    </svg>
  );
}
