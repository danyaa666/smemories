import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import {
  getLayoutVersion,
  listLayoutVersions,
  type LayoutVersion,
  type LayoutVersionRow,
} from "../../api/bookLayout";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import {
  absoluteTime,
  historyError,
  MAX_ROWS,
  PreviewCache,
  relativeTime,
  type HistoryError,
} from "./versions";

type List =
  | { status: "loading" }
  | { status: "ready"; rows: LayoutVersionRow[]; at: number }
  | { status: "failed"; error: HistoryError };

/** The text of a failed history call; `seconds` is the Retry-After of a 429. */
export function useErrorText() {
  const { t } = useTranslation();
  return (error: HistoryError, seconds?: number) =>
    t(`editor.history.err.${error}`, { seconds: seconds ?? 5 });
}

/**
 * The version history side panel (T-165): the list is read when the panel opens (it is mounted only then) and again on "Refresh" or when the shell bumps
 * `refreshKey` after a restore; never polled. A row is previewed with `version-get`; the last 5 bodies are kept while the panel is open.
 */
export function HistoryPanel({
  bookId,
  previewId,
  refreshKey,
  onPreview,
  children,
}: {
  bookId: string;
  previewId: string | null;
  refreshKey: number;
  onPreview: (v: LayoutVersion) => void;
  /** The preview bar and the restore error: inside the panel, so that Shift+Tab from the list reaches the two buttons in a few steps. */
  children?: ReactNode;
}) {
  const { t, i18n } = useTranslation();
  const errorText = useErrorText();
  const [list, setList] = useState<List>({ status: "loading" });
  const [nonce, setNonce] = useState(0);
  const [loadingId, setLoadingId] = useState<string | null>(null);
  const [err, setErr] = useState<{ error: HistoryError; seconds?: number } | null>(null);
  const [cursor, setCursor] = useState(0);
  const cache = useRef(new PreviewCache<LayoutVersion>());
  const wanted = useRef<string | null>(null);
  const rows = useRef<(HTMLButtonElement | null)[]>([]);
  const root = useRef<HTMLElement>(null);

  useEffect(() => root.current?.focus(), []);
  useEffect(() => {
    let live = true;
    listLayoutVersions(bookId).then(
      (r) => live && setList({ status: "ready", rows: r.items.slice(0, MAX_ROWS), at: Date.now() }),
      (e: unknown) => live && setList({ status: "failed", error: historyError(e) }),
    );
    return () => void (live = false);
  }, [bookId, nonce, refreshKey]);
  const refresh = useCallback(() => {
    setErr(null);
    setList({ status: "loading" });
    setNonce((n) => n + 1);
  }, []);

  const choose = useCallback(
    (row: LayoutVersionRow) => {
      setErr(null);
      wanted.current = row.id;
      const hit = cache.current.get(row.id);
      if (hit) return onPreview(hit);
      setLoadingId(row.id);
      getLayoutVersion(bookId, row.id).then(
        (v) => {
          cache.current.set(row.id, v);
          if (wanted.current === row.id) {
            setLoadingId(null);
            onPreview(v);
          }
        },
        (e: unknown) => {
          if (wanted.current !== row.id) return;
          setLoadingId(null);
          const error = historyError(e);
          setErr({
            error,
            seconds: (e as { retryAfter?: number } | undefined)?.retryAfter,
          });
          if (error === "gone") setNonce((n) => n + 1); // pruned meanwhile: read the list again, keep the message
        },
      );
    },
    [bookId, onPreview],
  );

  const move = (e: KeyboardEvent, n: number) => {
    const to =
      e.key === "ArrowDown" || e.key === "ArrowRight"
        ? Math.min(cursor + 1, n - 1)
        : e.key === "ArrowUp" || e.key === "ArrowLeft"
          ? Math.max(cursor - 1, 0)
          : e.key === "Home"
            ? 0
            : e.key === "End"
              ? n - 1
              : cursor;
    if (to === cursor && !["Home", "End"].includes(e.key)) return;
    e.preventDefault();
    setCursor(to);
    rows.current[to]?.focus();
  };

  const loc = i18n.language;
  return (
    <aside
      className="ed-panel ed-history"
      aria-label={t("editor.history.title")}
      ref={root}
      tabIndex={-1}
    >
      <h2>{t("editor.history.title")}</h2>
      {children}
      <Button onClick={refresh} disabled={list.status === "loading"}>
        {t("editor.history.refresh")}
      </Button>
      {err && (
        <p role="alert" className="error">
          {errorText(err.error, err.seconds)}
        </p>
      )}
      {list.status === "loading" && <p role="status">{t("editor.history.loading")}</p>}
      {list.status === "failed" && (
        <Banner variant="error" action={<Button onClick={refresh}>{t("auth.retry")}</Button>}>
          {errorText(list.error)}
        </Banner>
      )}
      {list.status === "ready" && list.rows.length === 0 && (
        <p className="ed-hint">{t("editor.history.empty")}</p>
      )}
      {list.status === "ready" && list.rows.length > 0 && (
        <div role="radiogroup" aria-label={t("editor.history.list")} className="ed-history-list">
          {list.rows.map((r, i) => {
            const when = relativeTime(r.created_at, list.at, loc);
            const kind = t(`editor.history.kind.${r.kind}`);
            const items = t("editor.history.items", { count: r.item_count });
            return (
              <button
                key={r.id}
                type="button"
                role="radio"
                aria-checked={r.id === previewId}
                aria-label={`${when}, ${kind}, ${items}`}
                aria-busy={loadingId === r.id || undefined}
                title={absoluteTime(r.created_at, loc)}
                tabIndex={i === cursor ? 0 : -1}
                ref={(el) => void (rows.current[i] = el)}
                className="ed-history-row"
                onFocus={() => setCursor(i)}
                onKeyDown={(e) => move(e, list.rows.length)}
                onClick={() => choose(r)}
              >
                <span>{when}</span>
                <span className="ed-hint">
                  {kind} · {items}
                </span>
              </button>
            );
          })}
        </div>
      )}
    </aside>
  );
}

/** The bar over the stage while a version is previewed: what it is, how many items would be dropped, and the two ways out. */
export function PreviewBanner({
  version,
  dropping,
  busy,
  onRestore,
  onBack,
}: {
  version: LayoutVersion;
  dropping: number;
  busy: boolean;
  onRestore: () => void;
  onBack: () => void;
}) {
  const { t, i18n } = useTranslation();
  return (
    <>
      <Banner
        variant="warning"
        action={
          <>
            <Button variant="primary" busy={busy} onClick={onRestore}>
              {t("editor.history.restore")}
            </Button>
            <Button disabled={busy} onClick={onBack}>
              {t("editor.history.back")}
            </Button>
          </>
        }
      >
        {t("editor.history.previewing", { time: absoluteTime(version.created_at, i18n.language) })}
        {dropping > 0 && <> {t("editor.history.willDrop", { count: dropping })}</>}
      </Banner>
    </>
  );
}
