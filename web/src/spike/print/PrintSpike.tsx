// Spike T-054: HTML templates printed through the browser ("Save as PDF"). Throwaway, dev builds only (see App.tsx).
// Route /spike/print?design=memory|classic&size=a5|a4|letter&lang=en|vi
import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import "./fonts/fonts.css";
import "./print.css";
import { ClassicBook } from "./classic";
import { makeBook, type Lang } from "./data";
import { MemoryBook } from "./memory";
import { SIZES, SizeProvider, type SizeKey } from "./sheet";

const pick = <T extends string>(v: string | null, allowed: readonly T[], dflt: T): T =>
  allowed.includes(v as T) ? (v as T) : dflt;

/** Sets <html data-ready> once fonts are loaded, every image is decoded and no async layout is pending (used by the Playwright script). */
function useReady(key: string) {
  const [stats, setStats] = useState<{
    pages: number;
    images: number;
    fit: Record<string, number>;
  } | null>(null);
  useEffect(() => {
    document.documentElement.removeAttribute("data-ready");
    let live = true;
    let stable = 0;
    const timer = setInterval(async () => {
      const imgs = Array.from(document.images);
      const done =
        document.fonts.status === "loaded" &&
        imgs.every((i) => i.complete && i.naturalWidth > 0) &&
        !document.querySelector("[data-pending]");
      stable = done ? stable + 1 : 0;
      if (stable < 3 || !live) return;
      clearInterval(timer);
      await Promise.all(imgs.map((i) => i.decode().catch(() => undefined)));
      const fit: Record<string, number> = {};
      document.querySelectorAll<HTMLElement>("[data-fit]").forEach((e) => {
        fit[e.dataset.fit ?? ""] = (fit[e.dataset.fit ?? ""] ?? 0) + 1;
      });
      setStats({ pages: document.querySelectorAll(".sp-sheet").length, images: imgs.length, fit });
      document.documentElement.dataset.ready = "1";
    }, 100);
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, [key]);
  return stats;
}

export default function PrintSpike() {
  const [params, setParams] = useSearchParams();
  const design = pick(params.get("design"), ["memory", "classic"], "memory");
  const size: SizeKey = pick(params.get("size"), ["a5", "a4", "letter"], "a5");
  const lang: Lang = pick(params.get("lang"), ["en", "vi"], "vi");
  const book = useMemo(() => makeBook(), []);
  const stats = useReady(`${design}-${size}-${lang}`);
  const set = (k: string, v: string) =>
    setParams((p) => {
      p.set(k, v);
      return p;
    });
  const { w, h } = SIZES[size];
  return (
    <SizeProvider value={size}>
      {/* One @page rule per document: every printed page has the same size, margin 0. */}
      <style>{`@page { size: ${w}mm ${h}mm; margin: 0 }`}</style>
      <aside className="sp-ui">
        <h1>Print spike (T-054)</h1>
        <p>
          {(
            [
              ["design", ["memory", "classic"]],
              ["size", ["a5", "a4", "letter"]],
              ["lang", ["en", "vi"]],
            ] as const
          ).map(([k, opts]) => (
            <label key={k}>
              {k}{" "}
              <select value={{ design, size, lang }[k]} onChange={(e) => set(k, e.target.value)}>
                {opts.map((o) => (
                  <option key={o} value={o}>
                    {o}
                  </option>
                ))}
              </select>{" "}
            </label>
          ))}
        </p>
        <button type="button" onClick={() => window.print()}>
          Save as PDF
        </button>
        <ol>
          <li>
            Destination: <b>Save as PDF</b> (Chrome, Edge) or <b>PDF</b> (Safari: PDF menu at the
            bottom left, Firefox: Microsoft Print to PDF or Save to PDF).
          </li>
          <li>
            Paper size: <b>{size.toUpperCase()}</b> ({w} x {h} mm). The page asks for it; if the
            dialog shows something else, choose it.
          </li>
          <li>
            Margins: <b>None</b>. Scale: <b>100 %</b> (not "Fit to page").
          </li>
          <li>
            Turn <b>Headers and footers</b> off and <b>Background graphics</b> on.
          </li>
        </ol>
        {stats ? (
          <p data-testid="stats">
            {stats.pages} pages, {stats.images} images, text fit: {JSON.stringify(stats.fit)}
          </p>
        ) : (
          <p>Loading fonts and photos...</p>
        )}
      </aside>
      <div className="sp-book">
        {design === "memory" ? (
          <MemoryBook book={book} lang={lang} />
        ) : (
          <ClassicBook book={book} lang={lang} />
        )}
      </div>
    </SizeProvider>
  );
}
