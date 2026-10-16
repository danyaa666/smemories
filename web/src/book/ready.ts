import { useEffect, useState } from "react";
import { mediaUrl } from "../api/yearbooks";

const CONCURRENCY = 6;

/** Public ids of every photo the book shows, once each. */
export function photoIds(
  d: {
    yearbook: { cover_media_id: string | null };
    profile: { photo_media_id: string | null };
    notes: { photo_ids: string[] }[];
  },
  shown = { cover: true, notes: true },
): string[] {
  const ids = [
    shown.cover ? d.yearbook.cover_media_id : null,
    d.profile.photo_media_id,
    ...(shown.notes ? d.notes.flatMap((n) => n.photo_ids) : []),
  ];
  return [...new Set(ids.filter((x): x is string => !!x))];
}

/**
 * Fetches every photo at the print size (a few at a time) so that the browser holds them before the print dialog opens;
 * the images on the page are lazy and find them in the cache. `failed` photos print as empty frames, which the screen says.
 */
export function usePhotoPreload(ids: string[]) {
  const key = ids.join(",");
  const [done, setDone] = useState({ key: "", loaded: 0, failed: 0 });
  useEffect(() => {
    let live = true;
    let next = 0;
    let loaded = 0;
    let failed = 0;
    const queue = key ? key.split(",") : [];
    const settle = (ok: boolean) => {
      if (ok) loaded++;
      else failed++;
      if (live) setDone({ key, loaded, failed });
      pump();
    };
    const pump = () => {
      if (!live || next >= queue.length) return;
      const img = new Image();
      img.onload = () => settle(true);
      img.onerror = () => settle(false);
      img.src = mediaUrl(queue[next++] ?? "", "print");
    };
    for (let i = 0; i < Math.min(CONCURRENCY, queue.length); i++) pump();
    return () => {
      live = false;
    };
  }, [key]);
  const current = done.key === key;
  const loaded = current ? done.loaded : 0;
  const failed = current ? done.failed : 0;
  return { total: ids.length, loaded, failed, settled: loaded + failed >= ids.length };
}

/**
 * True once the web fonts have loaded and stayed loaded for three checks in a row (a font only starts to load when
 * text that needs it is laid out, so one "loaded" right after rendering proves nothing). Starts again when `key` changes.
 */
export function useFontsLoaded(key: string): boolean {
  const [state, setState] = useState({ key: "", ok: false });
  useEffect(() => {
    const fonts = document.fonts;
    if (!fonts) {
      const id = setTimeout(() => setState({ key, ok: true }), 0);
      return () => clearTimeout(id);
    }
    let stable = 0;
    const id = setInterval(() => {
      stable = fonts.status === "loaded" ? stable + 1 : 0;
      if (stable >= 3) {
        clearInterval(id);
        setState({ key, ok: true });
      }
    }, 100);
    return () => clearInterval(id);
  }, [key]);
  return state.key === key && state.ok;
}
