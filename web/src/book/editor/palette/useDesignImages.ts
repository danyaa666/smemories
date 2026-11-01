// The list of the book's design images for the editor (T-162): newest first, keyset paging ("load more"), and the few changes the screen makes to it
// without asking the server again (a finished upload goes to the top, a deleted image leaves). Loaded when the editor opens.
import { useCallback, useEffect, useRef, useState } from "react";
import { listDesignImages, type Media } from "../../../api/yearbooks";

export type DesignImages = {
  items: readonly Media[];
  status: "idle" | "loading" | "ready" | "failed";
  /** More pages exist (the last answer had a `next_id`). */
  more: boolean;
  loadingMore: boolean;
  loadMore: () => void;
  reload: () => void;
  prepend: (m: Media) => void;
  drop: (id: string) => void;
};

export function useDesignImages(yearbookId: string): DesignImages {
  const [items, setItems] = useState<readonly Media[]>([]);
  const [next, setNext] = useState("");
  const [status, setStatus] = useState<DesignImages["status"]>("loading");
  const [loadingMore, setLoadingMore] = useState(false);
  const live = useRef(true);
  const gen = useRef(0); // a reload makes the answers of an older request stale
  useEffect(() => {
    live.current = true;
    return () => void (live.current = false);
  }, []);

  const load = useCallback(() => {
    const g = ++gen.current;
    listDesignImages(yearbookId).then(
      (r) => {
        if (!live.current || g !== gen.current) return;
        setItems(r.items);
        setNext(r.next_id);
        setStatus("ready");
      },
      () => live.current && g === gen.current && setStatus("failed"),
    );
  }, [yearbookId]);
  useEffect(load, [load]);
  const reload = useCallback(() => {
    setStatus("loading");
    load();
  }, [load]);

  const loadMore = useCallback(() => {
    if (!next || loadingMore) return;
    setLoadingMore(true);
    const g = gen.current;
    listDesignImages(yearbookId, next).then(
      (r) => {
        if (!live.current || g !== gen.current) return;
        // an upload or a delete between the pages must not repeat an image
        setItems((cur) => [...cur, ...r.items.filter((m) => !cur.some((c) => c.id === m.id))]);
        setNext(r.next_id);
        setLoadingMore(false);
      },
      () => {
        if (live.current) {
          setLoadingMore(false);
          setStatus("failed");
        }
      },
    );
  }, [yearbookId, next, loadingMore]);

  const prepend = useCallback(
    (m: Media) => setItems((cur) => (cur.some((c) => c.id === m.id) ? cur : [m, ...cur])),
    [],
  );
  const drop = useCallback((id: string) => setItems((cur) => cur.filter((m) => m.id !== id)), []);
  return { items, status, more: next !== "", loadingMore, loadMore, reload, prepend, drop };
}
