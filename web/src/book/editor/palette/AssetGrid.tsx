import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { assets, AssetSvg, type Asset, type AssetKind } from "../../assets";
import { matchAssets } from "./search";

export const COLS = 3; // the grid is three tiles wide (editor.css); the arrow keys step by this

export const assetLabel = (a: Asset, lang: string) =>
  lang.startsWith("vi") ? a.label_vi : a.label_en;

/** Draws the picture only once the tile is near the screen (the SVG text of a kind is a chunk of its own). Without IntersectionObserver it draws at once. */
function LazyAsset({ id }: { id: string }) {
  const ref = useRef<HTMLSpanElement>(null);
  const [near, setNear] = useState(typeof IntersectionObserver === "undefined");
  useEffect(() => {
    const el = ref.current;
    if (near || !el) return;
    const io = new IntersectionObserver((e) => e.some((x) => x.isIntersecting) && setNear(true), {
      rootMargin: "200px",
    });
    io.observe(el);
    return () => io.disconnect();
  }, [near]);
  return (
    <span ref={ref} className="ed-tile-pic">
      {near && <AssetSvg id={id} size={56} />}
    </span>
  );
}

/**
 * The searchable grid of registry assets of some kinds (T-162). One tab stop: arrow keys, Home and End move between the tiles, Enter or Space picks
 * (a tile is a button). Each tile is named by the asset's label in the language of the screen. The search ignores case and diacritics ("nang" finds "nắng").
 */
export function AssetGrid({
  kinds,
  label,
  onPick,
  selectedId,
  disabled,
}: {
  kinds: readonly AssetKind[];
  /** The accessible name of the grid. */
  label: string;
  onPick: (a: Asset) => void;
  /** The asset that is in use (a background), marked pressed. */
  selectedId?: string;
  disabled?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const [q, setQ] = useState("");
  const [cur, setCur] = useState(0);
  const list = useMemo(
    () =>
      matchAssets(
        assets.filter((a) => kinds.includes(a.kind)),
        q,
      ),
    [kinds, q],
  );
  const at = Math.min(cur, Math.max(0, list.length - 1));
  const grid = useRef<HTMLDivElement>(null);
  const move = (to: number) => {
    const i = Math.min(list.length - 1, Math.max(0, to));
    setCur(i);
    grid.current?.querySelectorAll<HTMLElement>("button")[i]?.focus();
  };
  const key = (e: KeyboardEvent) => {
    const d: Record<string, number> = {
      ArrowRight: 1,
      ArrowLeft: -1,
      ArrowDown: COLS,
      ArrowUp: -COLS,
    };
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key in d) move(at + d[e.key]!);
    else if (e.key === "Home") move(0);
    else if (e.key === "End") move(list.length - 1);
    else return;
    e.preventDefault();
  };
  return (
    <div className="ed-assets">
      <input
        type="search"
        className="ed-search"
        aria-label={t("editor.palette.search")}
        placeholder={t("editor.palette.search")}
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      <p className="ed-hint" role="status" aria-live="polite">
        {list.length === 0
          ? t("editor.palette.noMatch")
          : t("editor.palette.count", { count: list.length })}
      </p>
      {/* the keydown handler only moves focus between the buttons inside */}
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
      <div ref={grid} role="group" aria-label={label} className="ed-grid-tiles" onKeyDown={key}>
        {list.map((a, i) => (
          <button
            key={a.id}
            type="button"
            className="ed-tile"
            aria-label={assetLabel(a, i18n.language)}
            title={assetLabel(a, i18n.language)}
            aria-pressed={selectedId === undefined ? undefined : selectedId === a.id}
            tabIndex={i === at ? 0 : -1}
            disabled={disabled}
            onFocus={() => setCur(i)}
            onClick={() => onPick(a)}
          >
            <LazyAsset id={a.id} />
          </button>
        ))}
      </div>
    </div>
  );
}
